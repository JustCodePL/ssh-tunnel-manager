package e2efixture

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"time"
)

// Direct are servers reached without SSH: a VNC server like the one behind
// the forwards, and an RDP server that answers X.224 and TLS, enough for the
// app to see its certificate.
type Direct struct {
	VNCPort        int    `json:"vnc"`
	RDPPort        int    `json:"rdp"`
	RDPFingerprint string `json:"rdpFingerprint"`
}

// StartDirect starts the servers on loopback.
func StartDirect() (*Direct, error) {
	vncListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("listening for VNC: %w", err)
	}
	go acceptEach(vncListener, func(conn net.Conn) { serveRFB(conn) })

	cert, fingerprint, err := selfSignedCertificate()
	if err != nil {
		return nil, err
	}
	rdpListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("listening for RDP: %w", err)
	}
	go acceptEach(rdpListener, func(conn net.Conn) { serveRDPTLS(conn, cert) })

	return &Direct{
		VNCPort:        vncListener.Addr().(*net.TCPAddr).Port,
		RDPPort:        rdpListener.Addr().(*net.TCPAddr).Port,
		RDPFingerprint: fingerprint,
	}, nil
}

func acceptEach(listener net.Listener, serve func(net.Conn)) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		go func() {
			defer conn.Close()
			serve(conn)
		}()
	}
}

// confirmTLS is an X.224 connection confirm that selects TLS security.
var confirmTLS = []byte{0x03, 0x00, 0x00, 0x13, 0x0e, 0xd0, 0, 0, 0, 0, 0, 0x02, 0x00, 0x08, 0x00, 0x01, 0, 0, 0}

// serveRDPTLS answers the X.224 request, completes TLS and then waits: the
// fixture is not an RDP server, but the app checks the certificate first.
func serveRDPTLS(conn net.Conn, cert tls.Certificate) {
	header := make([]byte, 4)
	if _, err := io.ReadFull(conn, header); err != nil {
		return
	}
	if length := int(binary.BigEndian.Uint16(header[2:4])); length > 4 {
		if _, err := io.ReadFull(conn, make([]byte, length-4)); err != nil {
			return
		}
	}
	if _, err := conn.Write(confirmTLS); err != nil {
		return
	}
	server := tls.Server(conn, &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12})
	defer server.Close()
	_, _ = io.Copy(io.Discard, server)
}

func selfSignedCertificate() (tls.Certificate, string, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, "", fmt.Errorf("generating key: %w", err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "fixture-pc"}, NotAfter: time.Now().Add(24 * time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, "", fmt.Errorf("creating certificate: %w", err)
	}
	sum := sha256.Sum256(der)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, hex.EncodeToString(sum[:]), nil
}

// Handler serves the ports and fingerprint as JSON.
func (d *Direct) Handler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(d)
}
