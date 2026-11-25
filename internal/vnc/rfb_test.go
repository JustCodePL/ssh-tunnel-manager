package vnc

import (
	"bytes"
	"crypto/aes"
	"crypto/md5"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"math/big"
	"net"
	"testing"
	"time"
)

// oakleyGroup2 is the 1024-bit MODP prime from RFC 2409, the size macOS uses.
const oakleyGroup2 = "FFFFFFFFFFFFFFFFC90FDAA22168C234C4C6628B80DC1CD1" +
	"29024E088A67CC74020BBEA63B139B22514A08798E3404DD" +
	"EF9519B3CD3A431B302B0A6DF25F14374FE1356D6D51C245" +
	"E485B576625E7EC6F44C42E9A637ED6B0BFF5CB6F406B7ED" +
	"EE386BFB5A899FA5AE9F24117C4B1FE649286651ECE65381" +
	"FFFFFFFFFFFFFFFF"

// fakeServer describes how a test RFB server behaves.
type fakeServer struct {
	version  string // e.g. "RFB 003.008\n"
	types    []byte // offered security types; for 3.3 the first is chosen
	refuse   string // a reason to refuse the connection with no types
	password string // for VNC authentication
	username string // with password, for Apple Remote Desktop
	reject   string // a reason to fail the security result
	echo     bool   // echo everything after ServerInit
	// session, when set, takes over after the ClientInit byte instead of
	// the "INIT" answer.
	session func(conn net.Conn) error
}

// serve runs the server side of the handshake on conn and returns the security
// type the client chose, or an error. After a successful handshake it answers
// one ClientInit byte with "INIT".
func (s fakeServer) serve(conn net.Conn) (byte, error) {
	defer conn.Close()
	if _, err := io.WriteString(conn, s.version); err != nil {
		return 0, err
	}
	reply := make([]byte, versionLength)
	if _, err := io.ReadFull(conn, reply); err != nil {
		return 0, err
	}
	v38 := string(reply) == "RFB 003.008\n"
	v33 := string(reply) == "RFB 003.003\n"

	var chosen byte
	switch {
	case v33:
		value := uint32(0)
		if s.refuse == "" {
			value = uint32(s.types[0])
		}
		if err := binary.Write(conn, binary.BigEndian, value); err != nil {
			return 0, err
		}
		if s.refuse != "" {
			writeReason(conn, s.refuse)
			return 0, errors.New("refused")
		}
		chosen = s.types[0]
	case s.refuse != "":
		_, _ = conn.Write([]byte{0})
		writeReason(conn, s.refuse)
		return 0, errors.New("refused")
	default:
		if _, err := conn.Write(append([]byte{byte(len(s.types))}, s.types...)); err != nil {
			return 0, err
		}
		choice := make([]byte, 1)
		if _, err := io.ReadFull(conn, choice); err != nil {
			return 0, err
		}
		chosen = choice[0]
	}

	ok := true
	switch chosen {
	case securityNone:
		if !v38 {
			return chosen, s.finish(conn)
		}
	case securityVNCAuth:
		challenge := make([]byte, vncChallengeLength)
		_, _ = rand.Read(challenge)
		if _, err := conn.Write(challenge); err != nil {
			return 0, err
		}
		response := make([]byte, vncChallengeLength)
		if _, err := io.ReadFull(conn, response); err != nil {
			return 0, err
		}
		expected, _ := ChallengeResponse(s.password, challenge)
		ok = bytes.Equal(response, expected)
	case securityARD:
		username, password, err := serveARD(conn)
		if err != nil {
			return 0, err
		}
		ok = username == s.username && password == s.password
	default:
		return 0, errors.New("unexpected security type")
	}
	if s.reject != "" {
		ok = false
	}
	if !ok {
		_ = binary.Write(conn, binary.BigEndian, uint32(1))
		if v38 {
			writeReason(conn, s.reject)
		}
		return chosen, errors.New("authentication failed")
	}
	if err := binary.Write(conn, binary.BigEndian, uint32(0)); err != nil {
		return 0, err
	}
	return chosen, s.finish(conn)
}

func (s fakeServer) finish(conn net.Conn) error {
	init := make([]byte, 1)
	if _, err := io.ReadFull(conn, init); err != nil {
		return err
	}
	if s.session != nil {
		return s.session(conn)
	}
	if _, err := io.WriteString(conn, "INIT"); err != nil {
		return err
	}
	if s.echo {
		_, _ = io.Copy(conn, conn)
	}
	return nil
}

func writeReason(w io.Writer, reason string) {
	_ = binary.Write(w, binary.BigEndian, uint32(len(reason)))
	_, _ = io.WriteString(w, reason)
}

// serveARD plays the server's part of Apple Remote Desktop authentication and
// returns the credentials the client sent.
func serveARD(conn net.Conn) (string, string, error) {
	prime, _ := new(big.Int).SetString(oakleyGroup2, 16)
	const keyLength = 128
	private, _ := rand.Int(rand.Reader, new(big.Int).Sub(prime, big.NewInt(3)))
	private.Add(private, big.NewInt(2))
	public := new(big.Int).Exp(big.NewInt(2), private, prime)

	message := binary.BigEndian.AppendUint16(nil, 2)
	message = binary.BigEndian.AppendUint16(message, keyLength)
	message = append(message, prime.FillBytes(make([]byte, keyLength))...)
	message = append(message, public.FillBytes(make([]byte, keyLength))...)
	if _, err := conn.Write(message); err != nil {
		return "", "", err
	}
	response := make([]byte, ardCredentialBytes+keyLength)
	if _, err := io.ReadFull(conn, response); err != nil {
		return "", "", err
	}
	clientKey := new(big.Int).SetBytes(response[ardCredentialBytes:])
	shared := new(big.Int).Exp(clientKey, private, prime).FillBytes(make([]byte, keyLength))
	key := md5.Sum(shared)
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", "", err
	}
	plain := make([]byte, ardCredentialBytes)
	for i := 0; i < ardCredentialBytes; i += aes.BlockSize {
		block.Decrypt(plain[i:], response[i:])
	}
	field := func(b []byte) string {
		if end := bytes.IndexByte(b, 0); end >= 0 {
			return string(b[:end])
		}
		return string(b)
	}
	return field(plain[:ardFieldBytes]), field(plain[ardFieldBytes:]), nil
}

type serverResult struct {
	chosen byte
	err    error
}

// attempt is the outcome of one handshake against a fake server.
type attempt struct {
	version       Version
	authenticated bool
	apple         bool
	err           error
	server        serverResult
	conn          net.Conn
}

// handshake runs Handshake against a fake server over a pipe.
func handshake(t *testing.T, server fakeServer, login Credentials) attempt {
	t.Helper()
	client, peer := net.Pipe()
	t.Cleanup(func() { client.Close() })
	done := make(chan serverResult, 1)
	go func() {
		chosen, err := server.serve(peer)
		done <- serverResult{chosen, err}
	}()
	handshake, err := Handshake(client, login)
	result := attempt{version: handshake.Version, authenticated: handshake.Authenticated, apple: handshake.Apple, err: err, conn: client}
	if err != nil {
		client.Close()
		select {
		case result.server = <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("server did not finish")
		}
	}
	return result
}

// finishInit sends ClientInit and expects the server's reply.
func finishInit(t *testing.T, conn net.Conn) {
	t.Helper()
	if _, err := conn.Write([]byte{1}); err != nil {
		t.Fatal(err)
	}
	reply := make([]byte, 4)
	if _, err := io.ReadFull(conn, reply); err != nil || string(reply) != "INIT" {
		t.Fatalf("after handshake got %q, %v", reply, err)
	}
}

func TestHandshakeNone(t *testing.T) {
	for _, version := range []string{"RFB 003.008\n", "RFB 003.007\n", "RFB 003.003\n"} {
		a := handshake(t, fakeServer{version: version, types: []byte{securityNone}}, Credentials{})
		v, err, conn := a.version, a.err, a.conn
		if err != nil {
			t.Fatalf("%q: %v", version, err)
		}
		if a.authenticated {
			t.Errorf("%q: no authentication reported as authenticated", version)
		}
		if want := Version(version[10] - '0'); v != want {
			t.Errorf("%q: version %d, want %d", version, v, want)
		}
		finishInit(t, conn)
	}
}

func TestHandshakeVNCPassword(t *testing.T) {
	for _, version := range []string{"RFB 003.008\n", "RFB 003.007\n", "RFB 003.003\n"} {
		server := fakeServer{version: version, types: []byte{securityVNCAuth}, password: "secret12"}
		a := handshake(t, server, Credentials{Password: "secret12"})
		err, conn := a.err, a.conn
		if err != nil {
			t.Fatalf("%q: %v", version, err)
		}
		if !a.authenticated {
			t.Errorf("%q: password check not reported", version)
		}
		finishInit(t, conn)
	}
}

func TestHandshakeWrongPassword(t *testing.T) {
	server := fakeServer{version: "RFB 003.008\n", types: []byte{securityVNCAuth}, password: "secret12", reject: "Authentication failed"}
	err := handshake(t, server, Credentials{Password: "wrong"}).err
	var auth *AuthError
	if !errors.As(err, &auth) || auth.Reason != "Authentication failed" {
		t.Fatalf("err = %v", err)
	}

	server = fakeServer{version: "RFB 003.007\n", types: []byte{securityVNCAuth}, password: "secret12"}
	err = handshake(t, server, Credentials{Password: "wrong"}).err
	if !errors.As(err, &auth) || auth.Reason != "" {
		t.Fatalf("3.7 err = %v", err)
	}
}

func TestHandshakeAppleRemoteDesktop(t *testing.T) {
	// macOS answers with its own version and offers ARD alongside others.
	server := fakeServer{version: "RFB 003.889\n", types: []byte{securityARD, 33, 36, 35}, username: "artur", password: "zażółć-hasło"}
	a := handshake(t, server, Credentials{Username: "artur", Password: "zażółć-hasło"})
	v, err, conn := a.version, a.err, a.conn
	if err != nil {
		t.Fatal(err)
	}
	if v != Version38 {
		t.Errorf("version %d", v)
	}
	if !a.apple {
		t.Error("macOS Screen Sharing not recognised")
	}
	finishInit(t, conn)

	failed := handshake(t, server, Credentials{Username: "artur", Password: "wrong"})
	err = failed.err
	var auth *AuthError
	if !errors.As(err, &auth) {
		t.Fatalf("err = %v", err)
	}
	if failed.server.chosen != securityARD {
		t.Errorf("chose %d", failed.server.chosen)
	}
}

func TestHandshakePrefersARDWithUserName(t *testing.T) {
	server := fakeServer{version: "RFB 003.008\n", types: []byte{securityVNCAuth, securityARD}, username: "u", password: "p"}
	if a := handshake(t, server, Credentials{Username: "u", Password: "p"}); a.err != nil {
		t.Fatal(a.err)
	} else {
		finishInit(t, a.conn)
	}
	// Without a user name the VNC password is used.
	if a := handshake(t, server, Credentials{Password: "p"}); a.err != nil {
		t.Fatal(a.err)
	} else {
		finishInit(t, a.conn)
	}
}

func TestHandshakeNeedsCredentials(t *testing.T) {
	var required *CredentialsRequiredError
	err := handshake(t, fakeServer{version: "RFB 003.008\n", types: []byte{securityARD}}, Credentials{Password: "p"}).err
	if !errors.As(err, &required) || !required.Username {
		t.Fatalf("ARD only: %v", err)
	}
	err = handshake(t, fakeServer{version: "RFB 003.008\n", types: []byte{securityVNCAuth, securityARD}}, Credentials{}).err
	if !errors.As(err, &required) || required.Username {
		t.Fatalf("VNC and ARD: %v", err)
	}
	err = handshake(t, fakeServer{version: "RFB 003.003\n", types: []byte{securityVNCAuth}}, Credentials{}).err
	if !errors.As(err, &required) {
		t.Fatalf("3.3: %v", err)
	}
}

func TestHandshakeUnsupportedAndRefused(t *testing.T) {
	var unsupported *UnsupportedSecurityError
	err := handshake(t, fakeServer{version: "RFB 003.008\n", types: []byte{19, 18}}, Credentials{Username: "u", Password: "p"}).err
	if !errors.As(err, &unsupported) || !bytes.Equal(unsupported.Types, []byte{19, 18}) {
		t.Fatalf("err = %v", err)
	}
	if got := err.Error(); got != "the VNC server offers no supported security type (offered: VeNCrypt (19), TLS (18))" {
		t.Errorf("message %q", got)
	}

	var refused *RefusedError
	for _, version := range []string{"RFB 003.008\n", "RFB 003.003\n"} {
		err = handshake(t, fakeServer{version: version, refuse: "Too many security failures"}, Credentials{}).err
		if !errors.As(err, &refused) || refused.Reason != "Too many security failures" {
			t.Fatalf("%q: %v", version, err)
		}
	}
}

func TestHandshakeNotVNC(t *testing.T) {
	client, peer := net.Pipe()
	defer client.Close()
	go func() {
		_, _ = io.WriteString(peer, "SSH-2.0-OpenSSH_9.6\r\n")
		peer.Close()
	}()
	if _, err := Handshake(client, Credentials{}); !errors.Is(err, ErrNotVNC) {
		t.Fatalf("err = %v", err)
	}

	client2, peer2 := net.Pipe()
	defer client2.Close()
	go peer2.Close()
	if _, err := Handshake(client2, Credentials{}); !errors.Is(err, ErrNotVNC) {
		t.Fatalf("closed: err = %v", err)
	}
}

func TestParseVersion(t *testing.T) {
	cases := map[string]Version{
		"RFB 003.003\n": Version33,
		"RFB 003.005\n": Version33,
		"RFB 003.006\n": Version33,
		"RFB 003.007\n": Version37,
		"RFB 003.008\n": Version38,
		"RFB 003.889\n": Version38,
		"RFB 004.001\n": Version38,
		"RFB 005.000\n": Version38,
	}
	for banner, want := range cases {
		if got, err := parseVersion([]byte(banner)); err != nil || got != want {
			t.Errorf("%q = %d, %v", banner, got, err)
		}
	}
	for _, banner := range []string{"RFB 003.008 ", "HTTP/1.1 200", "RFB 0x3.008\n"} {
		if _, err := parseVersion([]byte(banner)); !errors.Is(err, ErrNotVNC) {
			t.Errorf("%q: %v", banner, err)
		}
	}
}

// The expected values come from noVNC's DES implementation, which the web
// client uses for VNC authentication.
func TestVNCAuthResponseMatchesNoVNC(t *testing.T) {
	challenge := make([]byte, 16)
	for i := range challenge {
		challenge[i] = byte(i * 17)
	}
	cases := map[string]string{
		"secret12": "817402416b165d7ade9c46b5faa42370",
		"pw":       "4d7a74da2480830e543b44ad841a6778",
		// Only the first eight characters count.
		"secret12-and-more": "817402416b165d7ade9c46b5faa42370",
	}
	for password, want := range cases {
		got, err := ChallengeResponse(password, challenge)
		if err != nil {
			t.Fatal(err)
		}
		if hex.EncodeToString(got) != want {
			t.Errorf("%q: %x, want %s", password, got, want)
		}
	}
}

func TestARDRejectsWeakServerKey(t *testing.T) {
	prime, _ := new(big.Int).SetString(oakleyGroup2, 16)
	for _, key := range []*big.Int{big.NewInt(1), new(big.Int).Sub(prime, big.NewInt(1))} {
		message := binary.BigEndian.AppendUint16(nil, 2)
		message = binary.BigEndian.AppendUint16(message, 128)
		message = append(message, prime.FillBytes(make([]byte, 128))...)
		message = append(message, key.FillBytes(make([]byte, 128))...)
		if _, err := readARDParams(bytes.NewReader(message)); err == nil {
			t.Errorf("key %v accepted", key)
		}
	}
	if _, err := readARDParams(bytes.NewReader([]byte{0, 2, 0, 0})); err == nil {
		t.Error("zero key length accepted")
	}
}

func TestARDCredentialsTruncateLongFields(t *testing.T) {
	long := string(bytes.Repeat([]byte("a"), 100))
	block, err := ardCredentials(Credentials{Username: long, Password: "p"})
	if err != nil {
		t.Fatal(err)
	}
	if block[63] != 0 || !bytes.Equal(block[:63], bytes.Repeat([]byte("a"), 63)) {
		t.Error("user name not truncated to 63 bytes")
	}
	if block[64] != 'p' || block[65] != 0 {
		t.Error("password field misplaced")
	}
}
