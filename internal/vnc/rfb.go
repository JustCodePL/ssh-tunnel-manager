// Package vnc connects the noVNC web client to VNC servers. The client runs in
// the webview, which cannot open TCP sockets, so it talks to a loopback
// WebSocket served here. The app authenticates to the server itself, so the
// password never reaches the webview, and then offers the client a session
// that needs no authentication.
package vnc

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
)

// DefaultPort is the standard VNC port, used by macOS Screen Sharing.
const DefaultPort = 5900

// RFB security types.
const (
	securityInvalid = 0
	securityNone    = 1
	securityVNCAuth = 2
	securityARD     = 30
)

const (
	versionLength = 12
	// maxReasonLength bounds a failure reason the server sends, so a broken
	// server cannot make the app allocate without limit.
	maxReasonLength = 4096
)

// Version is an RFB protocol version the app speaks: 3.3, 3.7 or 3.8.
type Version int

// Supported protocol versions.
const (
	Version33 Version = 3
	Version37 Version = 7
	Version38 Version = 8
)

func (v Version) banner() []byte {
	return []byte(fmt.Sprintf("RFB 003.%03d\n", int(v)))
}

// Credentials is a login for a VNC server. Username is needed only by Apple
// Remote Desktop authentication, which macOS Screen Sharing uses with the
// Mac's account name and password.
type Credentials struct {
	Username string
	Password string
}

// ErrNotVNC means the peer does not speak the RFB protocol.
var ErrNotVNC = errors.New("the server is not a VNC server")

// CredentialsRequiredError means the server needs a login the app did not have.
type CredentialsRequiredError struct {
	// Username is true when the server accepts only Apple Remote Desktop
	// authentication, which needs a user name as well as a password.
	Username bool
}

// Error describes what the server needs.
func (e *CredentialsRequiredError) Error() string {
	if e.Username {
		return "the VNC server needs a user name and password"
	}
	return "the VNC server needs a password"
}

// AuthError means the server rejected the login.
type AuthError struct {
	// Reason is the server's explanation, when the protocol carries one.
	Reason string
}

// Error includes the server's reason when it gave one.
func (e *AuthError) Error() string {
	if e.Reason == "" {
		return "the VNC server rejected the login"
	}
	return "the VNC server rejected the login: " + e.Reason
}

// RefusedError means the server refused the connection before authentication,
// for example after too many failed attempts.
type RefusedError struct {
	Reason string
}

// Error includes the server's reason when it gave one.
func (e *RefusedError) Error() string {
	if e.Reason == "" {
		return "the VNC server refused the connection"
	}
	return "the VNC server refused the connection: " + e.Reason
}

// UnsupportedSecurityError means the server offers no security type the app
// can use.
type UnsupportedSecurityError struct {
	Types []byte
}

// Error lists what the server offered.
func (e *UnsupportedSecurityError) Error() string {
	names := make([]string, 0, len(e.Types))
	for _, t := range e.Types {
		names = append(names, securityName(t))
	}
	return "the VNC server offers no supported security type (offered: " + strings.Join(names, ", ") + ")"
}

func securityName(t byte) string {
	switch t {
	case securityNone:
		return "none"
	case securityVNCAuth:
		return "VNC password"
	case 5, 6:
		return "RA2 (" + strconv.Itoa(int(t)) + ")"
	case 16:
		return "Tight (16)"
	case 18:
		return "TLS (18)"
	case 19:
		return "VeNCrypt (19)"
	case securityARD:
		return "Apple Remote Desktop"
	}
	return strconv.Itoa(int(t))
}

// parseVersion maps the server's protocol version to the one the app answers
// with, as other clients do: 3.3 for anything older than 3.7, and 3.8 for
// anything newer, including Apple's 3.889 and RealVNC's 4.x and 5.x.
func parseVersion(banner []byte) (Version, error) {
	if len(banner) != versionLength || !bytes.HasPrefix(banner, []byte("RFB ")) || banner[7] != '.' || banner[11] != '\n' {
		return 0, ErrNotVNC
	}
	major, err := strconv.Atoi(string(banner[4:7]))
	if err != nil {
		return 0, ErrNotVNC
	}
	minor, err := strconv.Atoi(string(banner[8:11]))
	if err != nil {
		return 0, ErrNotVNC
	}
	switch {
	case major < 3:
		return 0, fmt.Errorf("unsupported RFB version %d.%d", major, minor)
	case major > 3 || minor >= 8:
		return Version38, nil
	case minor == 7:
		return Version37, nil
	}
	return Version33, nil
}

// HandshakeResult describes a finished handshake.
type HandshakeResult struct {
	Version Version
	// Authenticated is true when the server checked the login. A server that
	// needs none accepts any, so a login it never checked must not be saved.
	Authenticated bool
}

// Handshake runs the client side of the RFB handshake on conn up to and
// including the security result, so the next message is the client's
// ClientInit.
func Handshake(conn io.ReadWriter, login Credentials) (HandshakeResult, error) {
	version, security, err := negotiate(conn, login)
	if err != nil {
		return HandshakeResult{}, err
	}
	return HandshakeResult{Version: version, Authenticated: security != securityNone}, nil
}

func negotiate(conn io.ReadWriter, login Credentials) (Version, byte, error) {
	banner := make([]byte, versionLength)
	if _, err := io.ReadFull(conn, banner); err != nil {
		return 0, 0, fmt.Errorf("reading the server's protocol version: %w", notVNCOnEOF(err))
	}
	version, err := parseVersion(banner)
	if err != nil {
		return 0, 0, err
	}
	if _, err := conn.Write(version.banner()); err != nil {
		return 0, 0, fmt.Errorf("sending the protocol version: %w", err)
	}

	var security byte
	if version == Version33 {
		security, err = readServerChosenSecurity(conn, login)
	} else {
		security, err = chooseSecurity(conn, login)
	}
	if err != nil {
		return 0, 0, err
	}

	switch security {
	case securityNone:
		// Only 3.8 sends a security result after no authentication.
		if version != Version38 {
			return version, security, nil
		}
	case securityVNCAuth:
		if err := authenticateVNC(conn, login.Password); err != nil {
			return 0, 0, err
		}
	case securityARD:
		if err := authenticateARD(conn, login); err != nil {
			return 0, 0, err
		}
	}
	return version, security, readSecurityResult(conn, version)
}

func notVNCOnEOF(err error) error {
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return ErrNotVNC
	}
	return err
}

// readServerChosenSecurity handles RFB 3.3, where the server picks the type.
func readServerChosenSecurity(conn io.ReadWriter, login Credentials) (byte, error) {
	var chosen uint32
	if err := binary.Read(conn, binary.BigEndian, &chosen); err != nil {
		return 0, fmt.Errorf("reading the security type: %w", err)
	}
	switch chosen {
	case securityInvalid:
		reason, err := readReason(conn)
		if err != nil {
			return 0, err
		}
		return 0, &RefusedError{Reason: reason}
	case securityNone:
		return securityNone, nil
	case securityVNCAuth:
		if login.Password == "" {
			return 0, &CredentialsRequiredError{}
		}
		return securityVNCAuth, nil
	}
	if chosen > 255 {
		return 0, &UnsupportedSecurityError{}
	}
	return 0, &UnsupportedSecurityError{Types: []byte{byte(chosen)}}
}

// chooseSecurity reads the types a 3.7 or 3.8 server offers and picks one: no
// authentication when the server allows it, Apple Remote Desktop when the login
// has a user name, and the VNC password otherwise.
func chooseSecurity(conn io.ReadWriter, login Credentials) (byte, error) {
	var count [1]byte
	if _, err := io.ReadFull(conn, count[:]); err != nil {
		return 0, fmt.Errorf("reading the security types: %w", err)
	}
	if count[0] == 0 {
		reason, err := readReason(conn)
		if err != nil {
			return 0, err
		}
		return 0, &RefusedError{Reason: reason}
	}
	offered := make([]byte, count[0])
	if _, err := io.ReadFull(conn, offered); err != nil {
		return 0, fmt.Errorf("reading the security types: %w", err)
	}

	var choice byte
	switch {
	case slices.Contains(offered, securityNone):
		choice = securityNone
	case slices.Contains(offered, securityARD) && login.Username != "" && login.Password != "":
		choice = securityARD
	case slices.Contains(offered, securityVNCAuth) && login.Password != "":
		choice = securityVNCAuth
	case slices.Contains(offered, securityVNCAuth):
		return 0, &CredentialsRequiredError{}
	case slices.Contains(offered, securityARD):
		return 0, &CredentialsRequiredError{Username: true}
	default:
		return 0, &UnsupportedSecurityError{Types: offered}
	}
	if _, err := conn.Write([]byte{choice}); err != nil {
		return 0, fmt.Errorf("sending the security type: %w", err)
	}
	return choice, nil
}

func readReason(r io.Reader) (string, error) {
	var length uint32
	if err := binary.Read(r, binary.BigEndian, &length); err != nil {
		return "", fmt.Errorf("reading the failure reason: %w", err)
	}
	reason := make([]byte, min(length, maxReasonLength))
	if _, err := io.ReadFull(r, reason); err != nil {
		return "", fmt.Errorf("reading the failure reason: %w", err)
	}
	return strings.TrimSpace(string(reason)), nil
}

// readSecurityResult reads the outcome of authentication. Only 3.8 servers
// explain a failure.
func readSecurityResult(r io.Reader, version Version) error {
	var result uint32
	if err := binary.Read(r, binary.BigEndian, &result); err != nil {
		return fmt.Errorf("reading the security result: %w", err)
	}
	if result == 0 {
		return nil
	}
	if version != Version38 {
		return &AuthError{}
	}
	reason, err := readReason(r)
	if err != nil {
		// The server said no; a missing reason does not change that.
		return &AuthError{}
	}
	return &AuthError{Reason: reason}
}
