package vnc

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

const webviewOrigin = "wails://localhost"

// noAuthTarget is a server that needs no login, answers ClientInit with
// "INIT" and then echoes. wrap, when set, sees the client end of the
// connection the bridge gets.
func noAuthTarget(wrap func(net.Conn) net.Conn) Target {
	return Target{Dial: func(context.Context) (net.Conn, error) {
		client, server := net.Pipe()
		go func() {
			_, _ = fakeServer{version: "RFB 003.008\n", types: []byte{securityNone}, echo: true}.serve(server)
		}()
		if wrap != nil {
			return wrap(client), nil
		}
		return client, nil
	}}
}

func open(t *testing.T, bridge *Bridge, target Target) Session {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	session, err := bridge.Open(ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	return session
}

func dial(t *testing.T, address, origin string) (*websocket.Conn, *http.Response, error) {
	t.Helper()
	header := http.Header{}
	if origin != "" {
		header.Set("Origin", origin)
	}
	dialer := websocket.Dialer{HandshakeTimeout: 2 * time.Second, Subprotocols: []string{"binary"}}
	return dialer.Dial(address, header)
}

// readExactly reads n bytes from the client side, across messages.
func readExactly(t *testing.T, conn *websocket.Conn, n int) []byte {
	t.Helper()
	reader := &messageReader{conn: conn}
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buffer := make([]byte, n)
	if _, err := io.ReadFull(reader, buffer); err != nil {
		t.Fatalf("reading %d bytes: %v", n, err)
	}
	return buffer
}

func send(t *testing.T, conn *websocket.Conn, payload []byte) {
	t.Helper()
	if err := conn.WriteMessage(websocket.BinaryMessage, payload); err != nil {
		t.Fatal(err)
	}
}

func TestBridgeOffersNoAuthenticationAndRelays(t *testing.T) {
	bridge := NewBridge(false)
	defer bridge.Close()
	session := open(t, bridge, noAuthTarget(nil))
	conn, response, err := dial(t, session.ProxyAddress, webviewOrigin)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if got := response.Header.Get("Sec-WebSocket-Protocol"); got != "binary" {
		t.Errorf("subprotocol %q", got)
	}

	if got := string(readExactly(t, conn, versionLength)); got != "RFB 003.008\n" {
		t.Fatalf("banner %q", got)
	}
	// The client may split its messages anywhere.
	send(t, conn, []byte("RFB 003"))
	send(t, conn, []byte(".008\n"))
	if got := readExactly(t, conn, 2); got[0] != 1 || got[1] != securityNone {
		t.Fatalf("security types %v", got)
	}
	// Choice and ClientInit in one message.
	send(t, conn, []byte{securityNone, 1})
	if got := binary.BigEndian.Uint32(readExactly(t, conn, 4)); got != 0 {
		t.Fatalf("security result %d", got)
	}
	if got := string(readExactly(t, conn, 4)); got != "INIT" {
		t.Fatalf("server init %q", got)
	}
	send(t, conn, []byte("hello"))
	if got := string(readExactly(t, conn, 5)); got != "hello" {
		t.Fatalf("echo %q", got)
	}
}

func TestBridgeOlderClients(t *testing.T) {
	for _, version := range []string{"RFB 003.007\n", "RFB 003.003\n"} {
		bridge := NewBridge(false)
		session := open(t, bridge, noAuthTarget(nil))
		conn, _, err := dial(t, session.ProxyAddress, webviewOrigin)
		if err != nil {
			t.Fatal(err)
		}
		readExactly(t, conn, versionLength)
		send(t, conn, []byte(version))
		if version == "RFB 003.003\n" {
			if got := binary.BigEndian.Uint32(readExactly(t, conn, 4)); got != securityNone {
				t.Fatalf("3.3 security %d", got)
			}
		} else {
			readExactly(t, conn, 2)
			send(t, conn, []byte{securityNone})
		}
		send(t, conn, []byte{1})
		if got := string(readExactly(t, conn, 4)); got != "INIT" {
			t.Fatalf("%q: server init %q", version, got)
		}
		conn.Close()
		bridge.Close()
	}
}

func TestBridgeRejectsForeignOriginWithoutSpendingToken(t *testing.T) {
	bridge := NewBridge(false)
	defer bridge.Close()
	session := open(t, bridge, noAuthTarget(nil))
	for _, origin := range []string{"https://evil.example", "http://127.0.0.1:5173", ""} {
		if _, response, err := dial(t, session.ProxyAddress, origin); err == nil || response == nil || response.StatusCode != http.StatusForbidden {
			t.Fatalf("origin %q was not rejected: %v", origin, err)
		}
	}
	conn, _, err := dial(t, session.ProxyAddress, webviewOrigin)
	if err != nil {
		t.Fatalf("token was spent by a rejected origin: %v", err)
	}
	conn.Close()
}

func TestBridgeTokenIsSingleUse(t *testing.T) {
	bridge := NewBridge(false)
	defer bridge.Close()
	session := open(t, bridge, noAuthTarget(nil))
	conn, _, err := dial(t, session.ProxyAddress, webviewOrigin)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, response, err := dial(t, session.ProxyAddress, webviewOrigin); err == nil || response.StatusCode != http.StatusForbidden {
		t.Fatalf("second use accepted: %v", err)
	}
	unknown := strings.TrimSuffix(session.ProxyAddress, session.Token) + strings.Repeat("0", 64)
	if _, response, err := dial(t, unknown, webviewOrigin); err == nil || response.StatusCode != http.StatusForbidden {
		t.Fatalf("unknown token accepted: %v", err)
	}
}

// closedConn reports when its Close is called.
type closedConn struct {
	net.Conn
	closed chan struct{}
}

func (c *closedConn) Close() error {
	select {
	case <-c.closed:
	default:
		close(c.closed)
	}
	return c.Conn.Close()
}

func TestBridgeClosesServerOfUnusedSession(t *testing.T) {
	bridge := NewBridge(false)
	defer bridge.Close()
	closed := make(chan struct{})
	session := open(t, bridge, noAuthTarget(func(c net.Conn) net.Conn { return &closedConn{Conn: c, closed: closed} }))
	bridge.CloseSession(session.ID)
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("server connection was not closed")
	}
	if _, response, err := dial(t, session.ProxyAddress, webviewOrigin); err == nil || response.StatusCode != http.StatusForbidden {
		t.Fatalf("closed session accepted: %v", err)
	}
}

func TestBridgeExpiresUnclaimedSession(t *testing.T) {
	bridge := NewBridge(false)
	defer bridge.Close()
	closed := make(chan struct{})
	session := open(t, bridge, noAuthTarget(func(c net.Conn) net.Conn { return &closedConn{Conn: c, closed: closed} }))
	// Fire the expiry now instead of waiting for the token lifetime.
	bridge.mu.Lock()
	bridge.pending[session.Token].timer.Reset(time.Millisecond)
	bridge.mu.Unlock()
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("server connection of an expired session was not closed")
	}
}

func TestBridgeCloseSessionEndsRunningSession(t *testing.T) {
	bridge := NewBridge(false)
	defer bridge.Close()
	session := open(t, bridge, noAuthTarget(nil))
	conn, _, err := dial(t, session.ProxyAddress, webviewOrigin)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	readExactly(t, conn, versionLength)
	deadline := time.Now().Add(time.Second)
	for {
		bridge.mu.Lock()
		_, running := bridge.active[session.ID]
		bridge.mu.Unlock()
		if running || time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	bridge.CloseSession(session.ID)
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, _, err := conn.ReadMessage(); err == nil {
		t.Fatal("session still open")
	}
}

func TestBridgeOpenReportsLoginProblemsWithoutASession(t *testing.T) {
	bridge := NewBridge(false)
	defer bridge.Close()
	target := Target{
		Dial: func(context.Context) (net.Conn, error) {
			client, server := net.Pipe()
			go func() {
				_, _ = fakeServer{version: "RFB 003.889\n", types: []byte{securityARD}, username: "u", password: "p"}.serve(server)
			}()
			return client, nil
		},
		Credentials: Credentials{Username: "u", Password: "wrong"},
	}
	ctx := context.Background()
	var rejected *AuthError
	if _, err := bridge.Open(ctx, target); !errors.As(err, &rejected) {
		t.Fatalf("wrong password: %v", err)
	}
	target.Credentials = Credentials{}
	var required *CredentialsRequiredError
	if _, err := bridge.Open(ctx, target); !errors.As(err, &required) || !required.Username {
		t.Fatalf("no login: %v", err)
	}
	dialErr := errors.New("refused")
	if _, err := bridge.Open(ctx, Target{Dial: func(context.Context) (net.Conn, error) { return nil, dialErr }}); !errors.Is(err, dialErr) {
		t.Fatalf("dial: %v", err)
	}
	bridge.mu.Lock()
	defer bridge.mu.Unlock()
	if len(bridge.pending) != 0 {
		t.Fatalf("failed attempts left sessions: %d", len(bridge.pending))
	}
}

func TestBridgeOpenGivesUpOnASilentServer(t *testing.T) {
	bridge := NewBridge(false)
	defer bridge.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := bridge.Open(ctx, Target{Dial: func(context.Context) (net.Conn, error) {
		client, _ := net.Pipe()
		return client, nil
	}})
	if !errors.Is(err, ErrHandshakeTimeout) {
		t.Fatalf("err = %v", err)
	}
}

func TestProbe(t *testing.T) {
	answer := func(greeting string) (bool, error) {
		client, server := net.Pipe()
		go func() {
			defer server.Close()
			_, _ = io.WriteString(server, greeting)
		}()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		return Probe(ctx, client)
	}
	cases := map[string]bool{
		"RFB 003.889\n":           true,
		"RFB 003.008\n":           true,
		"SSH-2.0-OpenSSH_9.6\r\n": false,
		"":                        false,
	}
	for greeting, want := range cases {
		got, err := answer(greeting)
		if err != nil || got != want {
			t.Errorf("%q = %v, %v", greeting, got, err)
		}
	}

	// A server that says nothing is an error once the context ends, not "no VNC".
	client, server := net.Pipe()
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := Probe(ctx, client); err == nil {
		t.Error("silent server: no error")
	}
}
