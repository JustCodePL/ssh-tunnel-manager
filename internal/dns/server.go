package dns

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sync"
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
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return nil
	}

	addr := fmt.Sprintf("%s:%d", BindIP, ListenPort)
	mux := mdns.NewServeMux()
	mux.HandleFunc(".", s.handle)

	udp := &mdns.Server{Addr: addr, Net: "udp", Handler: mux, ReusePort: false}
	tcp := &mdns.Server{Addr: addr, Net: "tcp", Handler: mux, ReusePort: false}

	udpReady := make(chan error, 1)
	tcpReady := make(chan error, 1)
	udp.NotifyStartedFunc = func() { udpReady <- nil }
	tcp.NotifyStartedFunc = func() { tcpReady <- nil }

	go func() {
		if err := udp.ListenAndServe(); err != nil {
			slog.Error("portless DNS UDP server exited", "error", err)
		}
	}()
	go func() {
		if err := tcp.ListenAndServe(); err != nil {
			slog.Error("portless DNS TCP server exited", "error", err)
		}
	}()

	select {
	case err := <-udpReady:
		if err != nil {
			return fmt.Errorf("binding UDP %s: %w", addr, err)
		}
	case <-time.After(3 * time.Second):
		return fmt.Errorf("DNS server (UDP) did not start in time — %s likely in use; another DNS service (Docker Desktop, WSL2, Pi-hole, VPN) may be blocking it", addr)
	}
	select {
	case err := <-tcpReady:
		if err != nil {
			return fmt.Errorf("binding TCP %s: %w", addr, err)
		}
	case <-time.After(3 * time.Second):
		return fmt.Errorf("DNS server (TCP) did not start in time — %s likely in use; another DNS service (Docker Desktop, WSL2, Pi-hole, VPN) may be blocking it", addr)
	}

	s.udp = udp
	s.tcp = tcp
	s.running = true
	slog.Info("portless DNS server started", "addr", addr)
	return nil
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
