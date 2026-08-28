package dns

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"syscall"
	"time"

	mdns "github.com/miekg/dns"
)

// ListenPort is the UDP/TCP port the embedded DNS server binds on 127.0.0.1.
// Defined per-platform: macOS/Linux use 5354 (5353 is reserved by mDNS) and
// rely on the resolver config explicitly forwarding to that port. Windows uses
// 53 because NRPT cannot specify a non-default port — binding 53 on loopback
// does not require admin on Windows.

// Server is the embedded *.ssh-local DNS server. It answers A queries from
// the registry and returns NXDOMAIN for anything else.
type Server struct {
	registry *Registry

	mu      sync.Mutex
	udp     *mdns.Server
	tcp     *mdns.Server
	running bool
}

// NewServer returns a server backed by the given registry.
func NewServer(reg *Registry) *Server {
	return &Server{registry: reg}
}

// Start binds UDP + TCP listeners on 127.0.0.1:ListenPort. Idempotent — a
// second call while running is a no-op.
func (s *Server) Start() error {
	addr := fmt.Sprintf("%s:%d", BindIP, ListenPort)
	return s.start(addr)
}

// start binds both transports before starting either server. This makes bind
// conflicts fail immediately and prevents a half-started DNS service when only
// one of UDP or TCP can claim the address.
func (s *Server) start(addr string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return nil
	}

	mux := mdns.NewServeMux()
	mux.HandleFunc(".", s.handle)

	udpConn, err := net.ListenPacket("udp", addr)
	if err != nil {
		return fmt.Errorf("binding UDP %s: %w", addr, err)
	}
	tcpListener, err := net.Listen("tcp", addr)
	if err != nil {
		_ = udpConn.Close()
		return fmt.Errorf("binding TCP %s: %w", addr, err)
	}

	udp := &mdns.Server{PacketConn: udpConn, Handler: mux}
	tcp := &mdns.Server{Listener: tcpListener, Handler: mux}
	ready := make(chan struct{}, 2)
	udpErr := make(chan error, 1)
	tcpErr := make(chan error, 1)
	udp.NotifyStartedFunc = func() { ready <- struct{}{} }
	tcp.NotifyStartedFunc = func() { ready <- struct{}{} }

	go func() {
		udpErr <- udp.ActivateAndServe()
	}()
	go func() {
		tcpErr <- tcp.ActivateAndServe()
	}()

	for started := 0; started < 2; started++ {
		select {
		case <-ready:
		case err := <-udpErr:
			_ = udpConn.Close()
			_ = tcpListener.Close()
			return fmt.Errorf("serving UDP %s: %w", addr, err)
		case err := <-tcpErr:
			_ = udpConn.Close()
			_ = tcpListener.Close()
			return fmt.Errorf("serving TCP %s: %w", addr, err)
		case <-time.After(3 * time.Second):
			_ = udpConn.Close()
			_ = tcpListener.Close()
			return fmt.Errorf("DNS server did not start in time on %s", addr)
		}
	}

	s.udp = udp
	s.tcp = tcp
	s.running = true
	go s.logServerExit("UDP", udpErr)
	go s.logServerExit("TCP", tcpErr)
	slog.Info("portless DNS server started", "addr", addr)
	return nil
}

func (s *Server) logServerExit(network string, result <-chan error) {
	if err := <-result; err != nil && s.Running() {
		slog.Error("portless DNS server exited", "network", network, "error", err)
	}
}

// IsAddressInUse reports whether starting the embedded DNS server failed
// because another process already owns its listen address.
func IsAddressInUse(err error) bool {
	return errors.Is(err, syscall.EADDRINUSE)
}

// Stop tears down both listeners. Safe to call multiple times.
func (s *Server) Stop() {
	s.mu.Lock()
	udp, tcp := s.udp, s.tcp
	s.udp, s.tcp, s.running = nil, nil, false
	s.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if udp != nil {
		_ = udp.ShutdownContext(ctx)
	}
	if tcp != nil {
		_ = tcp.ShutdownContext(ctx)
	}
}

// Running reports whether the server is currently bound.
func (s *Server) Running() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

func (s *Server) handle(w mdns.ResponseWriter, req *mdns.Msg) {
	if isControlRequest(req) {
		s.handleControl(w, req)
		return
	}

	msg := new(mdns.Msg)
	msg.SetReply(req)
	msg.Authoritative = true

	if len(req.Question) == 0 {
		msg.Rcode = mdns.RcodeFormatError
		_ = w.WriteMsg(msg)
		return
	}

	q := req.Question[0]
	switch q.Qtype {
	case mdns.TypeA:
		entry, ok := s.registry.LookupFQDN(q.Name)
		if !ok {
			msg.Rcode = mdns.RcodeNameError // NXDOMAIN
			_ = w.WriteMsg(msg)
			return
		}
		rr := &mdns.A{
			Hdr: mdns.RR_Header{
				Name:   q.Name,
				Rrtype: mdns.TypeA,
				Class:  mdns.ClassINET,
				Ttl:    5,
			},
			A: entry.IP.To4(),
		}
		msg.Answer = append(msg.Answer, rr)
	case mdns.TypeAAAA:
		// We never publish IPv6 — return empty answer (NOERROR + no records)
		// so resolvers fall back to A.
	default:
		msg.Rcode = mdns.RcodeNameError
	}
	_ = w.WriteMsg(msg)
}

// ResolverAddr returns the address resolver configs should point at.
func ResolverAddr() *net.UDPAddr {
	return &net.UDPAddr{IP: net.ParseIP(BindIP), Port: ListenPort}
}
