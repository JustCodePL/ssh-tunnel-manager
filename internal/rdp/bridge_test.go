package rdp

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/hex"
	"errors"
	"io"
	"math/big"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

var confirmTLS = []byte{0x03, 0x00, 0x00, 0x13, 0x0e, 0xd0, 0, 0, 0, 0, 0, 0x02, 0x00, 0x08, 0x00, 0x01, 0, 0, 0}
var confirmFailure = []byte{0x03, 0x00, 0x00, 0x13, 0x0e, 0xd0, 0, 0, 0, 0, 0, 0x03, 0x00, 0x08, 0x00, 0x05, 0, 0, 0}

const webviewOrigin = "wails://wails"

func testCertificate(t *testing.T) (tls.Certificate, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "rdp"}, NotAfter: time.Now().Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(der)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, hex.EncodeToString(sum[:])
}

// startFakeServer answers X.224 with confirm, upgrades to TLS and echoes data.
func startFakeServer(t *testing.T, confirm []byte, cert tls.Certificate) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				if _, err := readTPKT(conn); err != nil {
					return
				}
				if _, err := conn.Write(confirm); err != nil {
					return
				}
				server := tls.Server(conn, &tls.Config{Certificates: []tls.Certificate{cert}})
				defer server.Close()
				_, _ = io.Copy(server, server)
			}()
		}
	}()
	return listener.Addr().String()
}

func cleanPathRequest(t *testing.T, x224 []byte) []byte {
	t.Helper()
	version, err := encodedVersion()
	if err != nil {
		t.Fatal(err)
	}
	field, err := encodedX224(x224)
	if err != nil {
		t.Fatal(err)
	}
	pdu, err := joinPDU(version, field)
	if err != nil {
		t.Fatal(err)
	}
	return pdu
}

func dialBridge(address, origin string) (*websocket.Conn, *http.Response, error) {
	header := http.Header{}
	header.Set("Origin", origin)
	dialer := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	return dialer.Dial(address, header)
}

// pduFields maps context tags of a PDU to their contents.
func pduFields(t *testing.T, pdu []byte) map[int][]byte {
	t.Helper()
	var outer asn1.RawValue
	if _, err := asn1.Unmarshal(pdu, &outer); err != nil {
		t.Fatal(err)
	}
	fields := make(map[int][]byte)
	rest := outer.Bytes
	for len(rest) > 0 {
		var field asn1.RawValue
		next, err := asn1.Unmarshal(rest, &field)
		if err != nil {
			t.Fatal(err)
		}
		fields[field.Tag] = field.Bytes
		rest = next
	}
	return fields
}

func errorCode(t *testing.T, pdu []byte) int {
	t.Helper()
	body, ok := pduFields(t, pdu)[fieldError]
	if !ok {
		t.Fatalf("PDU has no error field: %x", pdu)
	}
	var seq asn1.RawValue
	if _, err := asn1.Unmarshal(body, &seq); err != nil {
		t.Fatal(err)
	}
	var codeField asn1.RawValue
	if _, err := asn1.Unmarshal(seq.Bytes, &codeField); err != nil {
		t.Fatal(err)
	}
	var code int
	if _, err := asn1.Unmarshal(codeField.Bytes, &code); err != nil {
		t.Fatal(err)
	}
	return code
}

func targetFor(address string, check func(string) error) Target {
	return Target{
		Address:          address,
		Dial:             func(ctx context.Context) (net.Conn, error) { return (&net.Dialer{}).DialContext(ctx, "tcp", address) },
		CheckCertificate: check,
	}
}

func openBridge(t *testing.T, target Target) (*Bridge, Session) {
	t.Helper()
	bridge := NewBridge(false)
	t.Cleanup(bridge.Close)
	session, err := bridge.Open(target)
	if err != nil {
		t.Fatal(err)
	}
	return bridge, session
}

func TestBridgeRelaysAfterHandshake(t *testing.T) {
	cert, fingerprint := testCertificate(t)
	address := startFakeServer(t, confirmTLS, cert)
	var seen string
	_, session := openBridge(t, targetFor(address, func(got string) error { seen = got; return nil }))

	conn, _, err := dialBridge(session.ProxyAddress, webviewOrigin)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := conn.WriteMessage(websocket.BinaryMessage, cleanPathRequest(t, probeRequest)); err != nil {
		t.Fatal(err)
	}
	_, response, err := conn.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	fields := pduFields(t, response)
	var x224 []byte
	if _, err := asn1.Unmarshal(fields[fieldX224], &x224); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(x224, confirmTLS) {
		t.Fatalf("x224 = %x", x224)
	}
	if _, ok := fields[fieldCertChain]; !ok {
		t.Fatal("response has no certificate chain")
	}
	if seen != fingerprint {
		t.Fatalf("fingerprint = %q, want %q", seen, fingerprint)
	}

	payload := []byte("rdp payload")
	if err := conn.WriteMessage(websocket.BinaryMessage, payload); err != nil {
		t.Fatal(err)
	}
	_, echoed, err := conn.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(echoed, payload) {
		t.Fatalf("echo = %q", echoed)
	}
}

func TestBridgeDoesNotConnectToClientChosenDestination(t *testing.T) {
	x224, err := parseCleanPathRequest(cleanPathRequest(t, probeRequest))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(x224, probeRequest) {
		t.Fatalf("x224 = %x", x224)
	}
	cert, _ := testCertificate(t)
	allowed := startFakeServer(t, confirmTLS, cert)
	dialed := ""
	target := targetFor(allowed, func(string) error { return nil })
	dial := target.Dial
	target.Dial = func(ctx context.Context) (net.Conn, error) { dialed = allowed; return dial(ctx) }
	_, session := openBridge(t, target)

	version, _ := encodedVersion()
	destination, _ := asn1.MarshalWithParams("10.99.99.99:3389", "utf8")
	destinationField, _ := explicitField(2, destination)
	field, _ := encodedX224(probeRequest)
	request, _ := joinPDU(version, destinationField, field)

	conn, _, err := dialBridge(session.ProxyAddress, webviewOrigin)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := conn.WriteMessage(websocket.BinaryMessage, request); err != nil {
		t.Fatal(err)
	}
	if _, _, err := conn.ReadMessage(); err != nil {
		t.Fatal(err)
	}
	if dialed != allowed {
		t.Fatalf("dialed %q", dialed)
	}
}

func TestBridgeRejectsForeignOriginWithoutSpendingToken(t *testing.T) {
	cert, _ := testCertificate(t)
	address := startFakeServer(t, confirmTLS, cert)
	_, session := openBridge(t, targetFor(address, func(string) error { return nil }))

	_, response, err := dialBridge(session.ProxyAddress, "https://evil.example")
	if err == nil || response == nil || response.StatusCode != http.StatusForbidden {
		t.Fatalf("foreign origin: err=%v response=%v", err, response)
	}
	conn, _, err := dialBridge(session.ProxyAddress, webviewOrigin)
	if err != nil {
		t.Fatalf("webview origin after rejected page: %v", err)
	}
	conn.Close()
}

func TestBridgeTokenIsSingleUse(t *testing.T) {
	cert, _ := testCertificate(t)
	address := startFakeServer(t, confirmTLS, cert)
	_, session := openBridge(t, targetFor(address, func(string) error { return nil }))

	conn, _, err := dialBridge(session.ProxyAddress, webviewOrigin)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_, response, err := dialBridge(session.ProxyAddress, webviewOrigin)
	if err == nil || response == nil || response.StatusCode != http.StatusForbidden {
		t.Fatalf("second use: err=%v response=%v", err, response)
	}
}

func TestBridgeExpiredTokenIsRejected(t *testing.T) {
	cert, _ := testCertificate(t)
	address := startFakeServer(t, confirmTLS, cert)
	bridge, session := openBridge(t, targetFor(address, func(string) error { return nil }))
	bridge.mu.Lock()
	entry := bridge.pending[session.Token]
	entry.expires = time.Now().Add(-time.Second)
	bridge.pending[session.Token] = entry
	bridge.mu.Unlock()

	_, response, err := dialBridge(session.ProxyAddress, webviewOrigin)
	if err == nil || response == nil || response.StatusCode != http.StatusForbidden {
		t.Fatalf("expired token: err=%v response=%v", err, response)
	}
}

func handshakeError(t *testing.T, session Session) int {
	t.Helper()
	conn, _, err := dialBridge(session.ProxyAddress, webviewOrigin)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := conn.WriteMessage(websocket.BinaryMessage, cleanPathRequest(t, probeRequest)); err != nil {
		t.Fatal(err)
	}
	_, response, err := conn.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	return errorCode(t, response)
}

func TestBridgeRefusesChangedCertificate(t *testing.T) {
	cert, _ := testCertificate(t)
	address := startFakeServer(t, confirmTLS, cert)
	_, session := openBridge(t, targetFor(address, func(string) error { return ErrCertificateChanged }))
	if code := handshakeError(t, session); code != errCodeCertChanged {
		t.Fatalf("code = %d", code)
	}
}

func TestBridgeReportsNegotiationFailure(t *testing.T) {
	cert, _ := testCertificate(t)
	address := startFakeServer(t, confirmFailure, cert)
	_, session := openBridge(t, targetFor(address, func(string) error { return nil }))
	if code := handshakeError(t, session); code != errCodeNegotiation {
		t.Fatalf("code = %d", code)
	}
}

func TestBridgeReportsUnreachableTarget(t *testing.T) {
	_, session := openBridge(t, Target{
		Address:          "unreachable",
		Dial:             func(context.Context) (net.Conn, error) { return nil, errors.New("refused") },
		CheckCertificate: func(string) error { return nil },
	})
	if code := handshakeError(t, session); code != errCodeTargetUnreachable {
		t.Fatalf("code = %d", code)
	}
}

func TestCloseSessionEndsRunningSession(t *testing.T) {
	cert, _ := testCertificate(t)
	address := startFakeServer(t, confirmTLS, cert)
	bridge, session := openBridge(t, targetFor(address, func(string) error { return nil }))
	conn, _, err := dialBridge(session.ProxyAddress, webviewOrigin)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := conn.WriteMessage(websocket.BinaryMessage, cleanPathRequest(t, probeRequest)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := conn.ReadMessage(); err != nil {
		t.Fatal(err)
	}
	bridge.CloseSession(session.ID)
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, _, err := conn.ReadMessage(); err == nil {
		t.Fatal("expected the session to end")
	}
}

func TestOriginAllowed(t *testing.T) {
	production, development := NewBridge(false), NewBridge(true)
	cases := []struct {
		origin          string
		wantProduction  bool
		wantDevelopment bool
	}{
		{"wails://wails", true, true},
		{"http://wails.localhost", true, true},
		{"https://wails.localhost:34115", true, true},
		{"http://localhost:34115", false, true},
		{"http://127.0.0.1:5173", false, true},
		{"https://evil.example", false, false},
		{"http://wails.localhost.evil.example", false, false},
		{"wails://other", false, false},
		{"", false, false},
	}
	for _, c := range cases {
		if got := production.originAllowed(c.origin); got != c.wantProduction {
			t.Errorf("production %q = %v", c.origin, got)
		}
		if got := development.originAllowed(c.origin); got != c.wantDevelopment {
			t.Errorf("development %q = %v", c.origin, got)
		}
	}
}

func probeAnswering(t *testing.T, answer []byte) (bool, error) {
	t.Helper()
	client, server := net.Pipe()
	go func() {
		defer server.Close()
		buffer := make([]byte, len(probeRequest))
		if _, err := io.ReadFull(server, buffer); err != nil {
			return
		}
		_, _ = server.Write(answer)
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return Probe(ctx, client)
}

func TestProbe(t *testing.T) {
	cases := []struct {
		name   string
		answer []byte
		want   bool
	}{
		{"confirm", confirmTLS, true},
		{"negotiation failure is still RDP", confirmFailure, true},
		{"ssh banner", []byte("SSH-2.0-OpenSSH_9.6\r\n"), false},
		{"tpkt that is not a confirm", []byte{3, 0, 0, 7, 2, 0xF0, 0x80}, false},
		{"closed without answer", nil, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := probeAnswering(t, c.answer)
			if err != nil {
				t.Fatal(err)
			}
			if got != c.want {
				t.Fatalf("got %v", got)
			}
		})
	}
}

func TestProbeHonoursContext(t *testing.T) {
	client, server := net.Pipe()
	defer server.Close()
	go func() { _, _ = io.Copy(io.Discard, server) }()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := Probe(ctx, client); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v", err)
	}
}
