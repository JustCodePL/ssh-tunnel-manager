// Package rdp connects the IronRDP web client to RDP servers. The client runs
// in the webview, which cannot open TCP sockets, so it talks to a loopback
// WebSocket served here. The bridge performs the X.224 exchange and the TLS
// handshake, then relays bytes in both directions.
package rdp

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	tokenLifetime    = 30 * time.Second
	handshakeTimeout = 20 * time.Second
	writeTimeout     = 30 * time.Second
	maxClientMessage = 16 << 20
	relayBufferSize  = 64 << 10
	webSocketPath    = "/rdp/"
)

// ErrCertificateChanged is returned by a Target's CheckCertificate when the
// server presents a certificate other than the one trusted earlier.
var ErrCertificateChanged = errors.New("RDP server certificate changed")

// Target describes where a session connects. The client cannot override it.
type Target struct {
	// Address is reported to the client as the server address.
	Address string
	// Dial opens a connection to the RDP server.
	Dial func(ctx context.Context) (net.Conn, error)
	// CheckCertificate receives the SHA-256 fingerprint of the server's leaf
	// certificate. Authentication relies on this check, because RDP servers
	// usually present self-signed certificates that cannot be chain-verified.
	CheckCertificate func(fingerprint string) error
}

// Session identifies a registered bridge session.
type Session struct {
	ID           string `json:"id"`
	Token        string `json:"token"`
	ProxyAddress string `json:"proxyAddress"`
}

type pendingSession struct {
	id      string
	target  Target
	expires time.Time
}

// Bridge serves the loopback WebSocket used by RDP sessions.
type Bridge struct {
	allowDevOrigins bool

	mu       sync.Mutex
	listener net.Listener
	server   *http.Server
	upgrader websocket.Upgrader
	pending  map[string]pendingSession
	active   map[string]context.CancelFunc
	closed   bool
}

// NewBridge creates a Bridge. allowDevOrigins additionally accepts the loopback
// origins used by `wails dev`.
func NewBridge(allowDevOrigins bool) *Bridge {
	b := &Bridge{
		allowDevOrigins: allowDevOrigins,
		pending:         make(map[string]pendingSession),
		active:          make(map[string]context.CancelFunc),
	}
	b.upgrader = websocket.Upgrader{
		ReadBufferSize:  relayBufferSize,
		WriteBufferSize: relayBufferSize,
		CheckOrigin:     func(r *http.Request) bool { return b.originAllowed(r.Header.Get("Origin")) },
	}
	return b
}

// originAllowed accepts only the webview's own origin, so a web page opened in
// a browser cannot reach the bridge even if it learned the port.
func (b *Bridge) originAllowed(origin string) bool {
	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}
	switch {
	case parsed.Scheme == "wails" && parsed.Host == "wails":
		return true
	case (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Hostname() == "wails.localhost":
		return true
	case b.allowDevOrigins && parsed.Scheme == "http":
		return parsed.Hostname() == "localhost" || parsed.Hostname() == "127.0.0.1"
	}
	return false
}

func randomHex(bytes int) (string, error) {
	buf := make([]byte, bytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generating random value: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// ensureServing starts the listener on first use. The caller holds b.mu.
func (b *Bridge) ensureServing() error {
	if b.closed {
		return errors.New("RDP bridge is closed")
	}
	if b.listener != nil {
		return nil
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("starting RDP bridge: %w", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc(webSocketPath, b.handle)
	b.listener = listener
	b.server = &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	server := b.server
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("RDP bridge stopped", "error", err)
		}
	}()
	return nil
}

// Open registers a session and returns the single-use address for the client.
func (b *Bridge) Open(target Target) (Session, error) {
	if target.Dial == nil || target.CheckCertificate == nil {
		return Session{}, errors.New("RDP target needs Dial and CheckCertificate")
	}
	id, err := randomHex(8)
	if err != nil {
		return Session{}, err
	}
	token, err := randomHex(32)
	if err != nil {
		return Session{}, err
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	if err := b.ensureServing(); err != nil {
		return Session{}, err
	}
	now := time.Now()
	for key, session := range b.pending {
		if now.After(session.expires) {
			delete(b.pending, key)
		}
	}
	b.pending[token] = pendingSession{id: id, target: target, expires: now.Add(tokenLifetime)}
	port := b.listener.Addr().(*net.TCPAddr).Port
	return Session{
		ID:           id,
		Token:        token,
		ProxyAddress: fmt.Sprintf("ws://127.0.0.1:%d%s%s", port, webSocketPath, token),
	}, nil
}

// CloseSession ends a session, whether it is still waiting for the client or
// already running.
func (b *Bridge) CloseSession(id string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for token, session := range b.pending {
		if session.id == id {
			delete(b.pending, token)
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
	b.pending = make(map[string]pendingSession)
	for _, cancel := range b.active {
		cancel()
	}
	if b.server != nil {
		_ = b.server.Close()
	}
}

// claim consumes a token. Each token admits exactly one connection.
func (b *Bridge) claim(token string) (pendingSession, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	session, ok := b.pending[token]
	if !ok {
		return pendingSession{}, false
	}
	delete(b.pending, token)
	return session, time.Now().Before(session.expires)
}

func (b *Bridge) handle(w http.ResponseWriter, r *http.Request) {
	// The origin is checked before the token is spent, so a rejected page
	// cannot invalidate a session that the client is about to open.
	if !b.originAllowed(r.Header.Get("Origin")) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	session, ok := b.claim(strings.TrimPrefix(r.URL.Path, webSocketPath))
	if !ok {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	conn, err := b.upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Warn("RDP bridge upgrade failed", "session", session.id, "error", err)
		return
	}
	defer conn.Close()
	conn.SetReadLimit(maxClientMessage)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	b.active[session.id] = cancel
	b.mu.Unlock()
	defer func() {
		b.mu.Lock()
		delete(b.active, session.id)
		b.mu.Unlock()
	}()

	if err := runSession(ctx, conn, session.target); err != nil && ctx.Err() == nil {
		slog.Warn("RDP session ended with an error", "session", session.id, "error", err)
	}
}

func writeBinary(conn *websocket.Conn, payload []byte) error {
	if err := conn.SetWriteDeadline(time.Now().Add(writeTimeout)); err != nil {
		return err
	}
	return conn.WriteMessage(websocket.BinaryMessage, payload)
}

func sendCleanPathError(conn *websocket.Conn, code int, x224 []byte) {
	pdu, err := encodeCleanPathError(code, x224)
	if err != nil {
		slog.Warn("encoding RDCleanPath error failed", "error", err)
		return
	}
	if err := writeBinary(conn, pdu); err != nil {
		slog.Debug("sending RDCleanPath error failed", "error", err)
	}
}

// connectServer runs the front of the connection: X.224, TLS and the
// certificate check. It returns the TLS connection and the response PDU.
func connectServer(ctx context.Context, request []byte, target Target, conn *websocket.Conn) (*tls.Conn, []byte, error) {
	raw, err := target.Dial(ctx)
	if err != nil {
		sendCleanPathError(conn, errCodeTargetUnreachable, nil)
		return nil, nil, fmt.Errorf("connecting to RDP server: %w", err)
	}
	// SSH channels do not support deadlines, so a timeout closes the conn.
	stop := context.AfterFunc(ctx, func() { raw.Close() })
	defer stop()
	fail := func(err error) (*tls.Conn, []byte, error) {
		raw.Close()
		return nil, nil, err
	}

	if _, err := raw.Write(request); err != nil {
		sendCleanPathError(conn, errCodeTargetUnreachable, nil)
		return fail(fmt.Errorf("sending X.224 request: %w", err))
	}
	response, err := readTPKT(raw)
	if err != nil {
		sendCleanPathError(conn, errCodeGeneral, nil)
		return fail(fmt.Errorf("reading X.224 response: %w", err))
	}
	if !isConnectionConfirm(response) {
		sendCleanPathError(conn, errCodeGeneral, nil)
		return fail(errors.New("server did not confirm the X.224 connection"))
	}
	if isNegotiationFailure(response) {
		sendCleanPathError(conn, errCodeNegotiation, response)
		return fail(errors.New("server rejected the offered security protocols"))
	}

	// #nosec G402 -- RDP servers use self-signed certificates; the fingerprint
	// is pinned by CheckCertificate below, before anything is sent to the server.
	tlsConn := tls.Client(raw, &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS12})
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		sendCleanPathError(conn, errCodeTLSHandshake, nil)
		return fail(fmt.Errorf("TLS handshake with RDP server: %w", err))
	}
	peers := tlsConn.ConnectionState().PeerCertificates
	if len(peers) == 0 {
		sendCleanPathError(conn, errCodeTLSHandshake, nil)
		return fail(errors.New("RDP server sent no certificate"))
	}
	sum := sha256.Sum256(peers[0].Raw)
	if err := target.CheckCertificate(hex.EncodeToString(sum[:])); err != nil {
		code := errCodeTLSHandshake
		if errors.Is(err, ErrCertificateChanged) {
			code = errCodeCertChanged
		}
		sendCleanPathError(conn, code, nil)
		return fail(err)
	}

	chain := make([][]byte, 0, len(peers))
	for _, peer := range peers {
		chain = append(chain, peer.Raw)
	}
	pdu, err := encodeCleanPathResponse(response, chain, target.Address)
	if err != nil {
		sendCleanPathError(conn, errCodeGeneral, nil)
		return fail(fmt.Errorf("encoding RDCleanPath response: %w", err))
	}
	return tlsConn, pdu, nil
}

func runSession(ctx context.Context, conn *websocket.Conn, target Target) error {
	if err := conn.SetReadDeadline(time.Now().Add(handshakeTimeout)); err != nil {
		return err
	}
	_, message, err := conn.ReadMessage()
	if err != nil {
		return fmt.Errorf("reading RDCleanPath request: %w", err)
	}
	request, err := parseCleanPathRequest(message)
	if err != nil {
		sendCleanPathError(conn, errCodeGeneral, nil)
		return err
	}

	handshakeCtx, cancelHandshake := context.WithTimeout(ctx, handshakeTimeout)
	tlsConn, response, err := connectServer(handshakeCtx, request, target, conn)
	cancelHandshake()
	if err != nil {
		return err
	}
	defer tlsConn.Close()

	if err := conn.SetReadDeadline(time.Time{}); err != nil {
		return err
	}
	if err := writeBinary(conn, response); err != nil {
		return fmt.Errorf("sending RDCleanPath response: %w", err)
	}

	stopServer := context.AfterFunc(ctx, func() { tlsConn.Close() })
	defer stopServer()
	stopClient := context.AfterFunc(ctx, func() { conn.Close() })
	defer stopClient()

	results := make(chan error, 2)
	go func() { results <- relayServerToClient(tlsConn, conn) }()
	go func() { results <- relayClientToServer(conn, tlsConn) }()
	err = <-results
	// Closing both ends releases the other relay goroutine.
	tlsConn.Close()
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
				return fmt.Errorf("writing to RDP client: %w", writeErr)
			}
		}
		if err != nil {
			if isExpectedClose(err) {
				return nil
			}
			return fmt.Errorf("reading from RDP server: %w", err)
		}
	}
}

func relayClientToServer(client *websocket.Conn, server net.Conn) error {
	for {
		_, message, err := client.ReadMessage()
		if err != nil {
			if isExpectedClose(err) {
				return nil
			}
			return fmt.Errorf("reading from RDP client: %w", err)
		}
		if _, err := server.Write(message); err != nil {
			return fmt.Errorf("writing to RDP server: %w", err)
		}
	}
}

func isExpectedClose(err error) bool {
	if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway, websocket.CloseNoStatusReceived) {
		return true
	}
	return errors.Is(err, net.ErrClosed) || strings.Contains(err.Error(), "EOF")
}
