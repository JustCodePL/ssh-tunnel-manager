package vnc

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"ssh-tunnel-manager/internal/webview"
)

const (
	tokenLifetime    = 30 * time.Second
	handshakeTimeout = 20 * time.Second
	writeTimeout     = 30 * time.Second
	maxClientMessage = 16 << 20
	relayBufferSize  = 64 << 10
	webSocketPath    = "/vnc/"
)

// Target describes where a session connects. The client cannot override it.
type Target struct {
	// Dial opens a connection to the VNC server, for example through an SSH
	// tunnel.
	Dial func(ctx context.Context) (net.Conn, error)
	// Credentials is the login the app authenticates with. The client never
	// sees it.
	Credentials Credentials
}

// ErrHandshakeTimeout means the server did not finish the handshake in time.
var ErrHandshakeTimeout = errors.New("the VNC server did not finish the handshake in time")

// Session identifies a registered bridge session.
type Session struct {
	ID           string `json:"id"`
	Token        string `json:"token"`
	ProxyAddress string `json:"proxyAddress"`
	// Authenticated is true when the server checked the login.
	Authenticated bool `json:"authenticated"`
}

type pendingSession struct {
	id     string
	server net.Conn
	timer  *time.Timer
}

// Bridge serves the loopback WebSocket used by VNC sessions. It connects to
// the server and authenticates before the client connects, so each session
// carries a server connection that is already past authentication.
type Bridge struct {
	allowDevOrigins bool

	mu       sync.Mutex
	listener net.Listener
	server   *http.Server
	upgrader websocket.Upgrader
	pending  map[string]*pendingSession
	active   map[string]context.CancelFunc
	closed   bool
}

// NewBridge creates a Bridge. allowDevOrigins additionally accepts the loopback
// origins used by `wails dev` and the browser build.
func NewBridge(allowDevOrigins bool) *Bridge {
	b := &Bridge{
		allowDevOrigins: allowDevOrigins,
		pending:         make(map[string]*pendingSession),
		active:          make(map[string]context.CancelFunc),
	}
	b.upgrader = websocket.Upgrader{
		ReadBufferSize:  relayBufferSize,
		WriteBufferSize: relayBufferSize,
		// noVNC asks for the "binary" subprotocol.
		Subprotocols: []string{"binary"},
		CheckOrigin:  func(r *http.Request) bool { return b.originAllowed(r.Header.Get("Origin")) },
	}
	return b
}

func (b *Bridge) originAllowed(origin string) bool {
	return webview.OriginAllowed(origin, b.allowDevOrigins)
}

func randomHex(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generating random value: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// ensureServing starts the listener on first use. The caller holds b.mu.
func (b *Bridge) ensureServing() error {
	if b.closed {
		return errors.New("VNC bridge is closed")
	}
	if b.listener != nil {
		return nil
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("starting VNC bridge: %w", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc(webSocketPath, b.handle)
	b.listener = listener
	b.server = &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	server := b.server
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("VNC bridge stopped", "error", err)
		}
	}()
	return nil
}

// Open connects to the target's server and authenticates with its credentials,
// then registers a session for the client and returns its single-use address.
// A rejected or missing login comes back as *AuthError or
// *CredentialsRequiredError, so the caller can ask for another. ctx bounds the
// connection and the handshake, not the session.
func (b *Bridge) Open(ctx context.Context, target Target) (Session, error) {
	if target.Dial == nil {
		return Session{}, errors.New("VNC target needs Dial")
	}
	server, err := target.Dial(ctx)
	if err != nil {
		return Session{}, err
	}
	// SSH channels do not support deadlines, so ctx closes the conn.
	stop := context.AfterFunc(ctx, func() { server.Close() })
	result, err := Handshake(server, target.Credentials)
	if !stop() {
		server.Close()
		return Session{}, ErrHandshakeTimeout
	}
	if err != nil {
		server.Close()
		return Session{}, err
	}
	session, err := b.register(server)
	session.Authenticated = result.Authenticated
	return session, err
}

// register keeps an authenticated server connection, whose next message is
// the client's ClientInit, until the client claims it. The bridge owns server
// from then on: it is closed when the session ends, or when the client does
// not connect in time.
func (b *Bridge) register(server net.Conn) (Session, error) {
	fail := func(err error) (Session, error) {
		server.Close()
		return Session{}, err
	}
	id, err := randomHex(8)
	if err != nil {
		return fail(err)
	}
	token, err := randomHex(32)
	if err != nil {
		return fail(err)
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	if err := b.ensureServing(); err != nil {
		return fail(err)
	}
	b.pending[token] = &pendingSession{
		id:     id,
		server: server,
		timer:  time.AfterFunc(tokenLifetime, func() { b.expire(token) }),
	}
	port := b.listener.Addr().(*net.TCPAddr).Port
	return Session{
		ID:           id,
		Token:        token,
		ProxyAddress: fmt.Sprintf("ws://127.0.0.1:%d%s%s", port, webSocketPath, token),
	}, nil
}

// expire drops a session the client never claimed.
func (b *Bridge) expire(token string) {
	b.mu.Lock()
	session, ok := b.pending[token]
	delete(b.pending, token)
	b.mu.Unlock()
	if ok {
		session.server.Close()
	}
}

// CloseSession ends a session, whether it is still waiting for the client or
// already running.
func (b *Bridge) CloseSession(id string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for token, session := range b.pending {
		if session.id == id {
			delete(b.pending, token)
			session.timer.Stop()
			session.server.Close()
		}
	}
	if cancel, ok := b.active[id]; ok {
		cancel()
	}
}

// Close ends all sessions and stops the listener.
func (b *Bridge) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = true
	for _, session := range b.pending {
		session.timer.Stop()
		session.server.Close()
	}
	b.pending = make(map[string]*pendingSession)
	for _, cancel := range b.active {
		cancel()
	}
	if b.server != nil {
		_ = b.server.Close()
	}
}

// claim consumes a token. Each token admits exactly one connection. The
// session becomes active in the same step, with cancel ending it, so
// CloseSession finds it at every moment.
func (b *Bridge) claim(token string, cancel context.CancelFunc) (*pendingSession, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	session, ok := b.pending[token]
	if !ok || b.closed {
		return nil, false
	}
	delete(b.pending, token)
	if !session.timer.Stop() {
		// The token expired as it was claimed. The timer's expire call finds
		// nothing to close now, so close the connection here.
		session.server.Close()
		return nil, false
	}
	b.active[session.id] = cancel
	return session, true
}

func (b *Bridge) handle(w http.ResponseWriter, r *http.Request) {
	// The origin is checked before the token is spent, so a rejected page
	// cannot invalidate a session that the client is about to open.
	if !b.originAllowed(r.Header.Get("Origin")) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	session, ok := b.claim(strings.TrimPrefix(r.URL.Path, webSocketPath), cancel)
	if !ok {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	defer session.server.Close()
	defer func() {
		b.mu.Lock()
		delete(b.active, session.id)
		b.mu.Unlock()
	}()
	conn, err := b.upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Warn("VNC bridge upgrade failed", "session", session.id, "error", err)
		return
	}
	defer conn.Close()
	conn.SetReadLimit(maxClientMessage)

	if err := runSession(ctx, conn, session.server); err != nil && ctx.Err() == nil {
		slog.Warn("VNC session ended with an error", "session", session.id, "error", err)
	}
}

// messageReader reads a WebSocket's messages as one byte stream, because RFB
// messages need not line up with WebSocket messages.
type messageReader struct {
	conn    *websocket.Conn
	current io.Reader
}

func (r *messageReader) Read(p []byte) (int, error) {
	for {
		if r.current == nil {
			_, next, err := r.conn.NextReader()
			if err != nil {
				return 0, err
			}
			r.current = next
		}
		n, err := r.current.Read(p)
		if errors.Is(err, io.EOF) {
			r.current = nil
			if n > 0 {
				return n, nil
			}
			continue
		}
		return n, err
	}
}

func writeBinary(conn *websocket.Conn, payload []byte) error {
	if err := conn.SetWriteDeadline(time.Now().Add(writeTimeout)); err != nil {
		return err
	}
	return conn.WriteMessage(websocket.BinaryMessage, payload)
}

// greetClient plays the server's part of the handshake towards the web client:
// the app has authenticated already, so the client is offered no
// authentication. The client's next message is its ClientInit.
func greetClient(conn *websocket.Conn, reader io.Reader) error {
	if err := writeBinary(conn, Version38.banner()); err != nil {
		return fmt.Errorf("sending the protocol version: %w", err)
	}
	banner := make([]byte, versionLength)
	if _, err := io.ReadFull(reader, banner); err != nil {
		return fmt.Errorf("reading the client's protocol version: %w", err)
	}
	var version Version
	switch {
	case bytes.Equal(banner, Version38.banner()):
		version = Version38
	case bytes.Equal(banner, Version37.banner()):
		version = Version37
	case bytes.Equal(banner, Version33.banner()):
		version = Version33
	default:
		return fmt.Errorf("unexpected client protocol version %q", banner)
	}
	if version == Version33 {
		return writeBinary(conn, binary.BigEndian.AppendUint32(nil, securityNone))
	}
	if err := writeBinary(conn, []byte{1, securityNone}); err != nil {
		return fmt.Errorf("sending the security types: %w", err)
	}
	var choice [1]byte
	if _, err := io.ReadFull(reader, choice[:]); err != nil {
		return fmt.Errorf("reading the client's security type: %w", err)
	}
	if choice[0] != securityNone {
		return fmt.Errorf("client chose security type %d", choice[0])
	}
	if version == Version38 {
		return writeBinary(conn, binary.BigEndian.AppendUint32(nil, 0))
	}
	return nil
}

func runSession(ctx context.Context, conn *websocket.Conn, server net.Conn) error {
	stopServer := context.AfterFunc(ctx, func() { server.Close() })
	defer stopServer()
	stopClient := context.AfterFunc(ctx, func() { conn.Close() })
	defer stopClient()

	reader := &messageReader{conn: conn}
	if err := conn.SetReadDeadline(time.Now().Add(handshakeTimeout)); err != nil {
		return err
	}
	if err := greetClient(conn, reader); err != nil {
		return err
	}
	if err := conn.SetReadDeadline(time.Time{}); err != nil {
		return err
	}

	results := make(chan error, 2)
	go func() { results <- relayServerToClient(server, conn) }()
	go func() { results <- relayClientToServer(reader, server) }()
	err := <-results
	// Closing both ends releases the other relay goroutine.
	server.Close()
	conn.Close()
	<-results
	return err
}

func relayServerToClient(server net.Conn, client *websocket.Conn) error {
	buffer := make([]byte, relayBufferSize)
	for {
		n, err := server.Read(buffer)
		if n > 0 {
			if writeErr := writeBinary(client, buffer[:n]); writeErr != nil {
				return fmt.Errorf("writing to VNC client: %w", writeErr)
			}
		}
		if err != nil {
			if isExpectedClose(err) {
				return nil
			}
			return fmt.Errorf("reading from VNC server: %w", err)
		}
	}
}

func relayClientToServer(client io.Reader, server net.Conn) error {
	if _, err := io.Copy(server, client); err != nil && !isExpectedClose(err) {
		return fmt.Errorf("relaying VNC client data: %w", err)
	}
	return nil
}

func isExpectedClose(err error) bool {
	if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway, websocket.CloseNoStatusReceived) {
		return true
	}
	return errors.Is(err, net.ErrClosed) || errors.Is(err, io.EOF) || strings.Contains(err.Error(), "EOF")
}
