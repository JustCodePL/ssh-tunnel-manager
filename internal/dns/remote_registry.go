package dns

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	mdns "github.com/miekg/dns"
)

const (
	controlProtocol = "stm-portless-registry-v1"
	controlTimeout  = 2 * time.Second
)

var controlName = "_stm-control." + TLD + "."

// RemoteRegistry allocates Portless addresses through the app instance that
// owns the system-wide DNS listener. Control messages travel directly over
// that listener's TCP socket and never leave the loopback interface.
type RemoteRegistry struct {
	addr  string
	owner string

	mu      sync.Mutex
	entries map[string]Entry
	closed  bool
}

// ConnectRemoteRegistry connects to a compatible SSH Tunnel Manager instance
// already serving Portless DNS on the configured resolver address.
func ConnectRemoteRegistry() (*RemoteRegistry, error) {
	addr := net.JoinHostPort(BindIP, strconv.Itoa(ListenPort))
	return newRemoteRegistry(addr)
}

func newRemoteRegistry(addr string) (*RemoteRegistry, error) {
	owner, err := newRegistryOwnerID()
	if err != nil {
		return nil, err
	}
	r := &RemoteRegistry{
		addr:    addr,
		owner:   owner,
		entries: make(map[string]Entry),
	}
	response, err := r.request("ping")
	if err != nil {
		return nil, fmt.Errorf("checking shared Portless registry: %w", err)
	}
	if len(response) != 2 || response[0] != "ok" || response[1] != controlProtocol {
		return nil, fmt.Errorf("incompatible Portless DNS owner on %s", addr)
	}
	return r, nil
}

func newRegistryOwnerID() (string, error) {
	var suffix [8]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return "", fmt.Errorf("creating Portless registry owner ID: %w", err)
	}
	return fmt.Sprintf("p%d-%s", os.Getpid(), hex.EncodeToString(suffix[:])), nil
}

// Allocate reserves a globally unique loopback address from the DNS owner's
// registry. Repeated calls for the same domain by this app instance are
// idempotent.
func (r *RemoteRegistry) Allocate(domain string, port int) (Entry, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return Entry{}, fmt.Errorf("shared Portless registry is closed")
	}
	domain = normalizeDomain(domain)
	if existing, ok := r.entries[domain]; ok {
		if existing.Port != port {
			return Entry{}, fmt.Errorf("domain %q is already registered on port %d", domain, existing.Port)
		}
		return existing, nil
	}
	response, err := r.request("allocate", r.owner, strconv.Itoa(port), domain)
	if err != nil {
		return Entry{}, err
	}
	if len(response) != 5 || response[0] != "ok" || response[1] != controlProtocol {
		return Entry{}, fmt.Errorf("invalid allocate response from shared Portless registry")
	}
	ip := net.ParseIP(response[3]).To4()
	if ip == nil {
		return Entry{}, fmt.Errorf("shared Portless registry returned invalid IP %q", response[3])
	}
	returnedPort, err := strconv.Atoi(response[4])
	if err != nil || returnedPort != port {
		return Entry{}, fmt.Errorf("shared Portless registry returned invalid port %q", response[4])
	}
	entry := Entry{Domain: response[2], IP: ip, Port: returnedPort, owner: r.owner}
	r.entries[domain] = entry
	return entry, nil
}

// Release returns a domain reservation to the DNS owner's global pool.
func (r *RemoteRegistry) Release(domain string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return
	}
	domain = normalizeDomain(domain)
	if _, ok := r.entries[domain]; !ok {
		return
	}
	if _, err := r.request("release", r.owner, domain); err != nil {
		return
	}
	delete(r.entries, domain)
}

// Block tells the DNS owner not to hand out an address that this process was
// unable to bind. Only addresses reserved by this owner can be blocked.
func (r *RemoteRegistry) Block(ip net.IP) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return
	}
	_, _ = r.request("block", r.owner, ip.String())
}

// Close releases every reservation still owned by this app instance.
func (r *RemoteRegistry) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}
	_, err := r.request("release-owner", r.owner)
	r.closed = true
	r.entries = make(map[string]Entry)
	return err
}

func (r *RemoteRegistry) request(operation string, args ...string) ([]string, error) {
	msg := new(mdns.Msg)
	msg.SetQuestion(controlName, mdns.TypeTXT)
	fields := append([]string{operation, controlProtocol}, args...)
	msg.Extra = []mdns.RR{&mdns.TXT{
		Hdr: mdns.RR_Header{Name: controlName, Rrtype: mdns.TypeTXT, Class: mdns.ClassINET},
		Txt: fields,
	}}
	client := &mdns.Client{Net: "tcp", Timeout: controlTimeout}
	reply, _, err := client.Exchange(msg, r.addr)
	if err != nil {
		return nil, err
	}
	for _, answer := range reply.Answer {
		txt, ok := answer.(*mdns.TXT)
		if !ok || !strings.EqualFold(txt.Hdr.Name, controlName) {
			continue
		}
		if len(txt.Txt) >= 3 && txt.Txt[0] == "error" && txt.Txt[1] == controlProtocol {
			return nil, fmt.Errorf("%s", txt.Txt[2])
		}
		return txt.Txt, nil
	}
	return nil, fmt.Errorf("shared Portless registry returned no control response")
}

func isControlRequest(req *mdns.Msg) bool {
	return len(req.Question) == 1 &&
		req.Question[0].Qtype == mdns.TypeTXT &&
		strings.EqualFold(req.Question[0].Name, controlName)
}

func (s *Server) handleControl(w mdns.ResponseWriter, req *mdns.Msg) {
	response := new(mdns.Msg)
	response.SetReply(req)
	fields, err := controlRequestFields(req)
	if err != nil {
		setControlResponse(response, "error", controlProtocol, err.Error())
		_ = w.WriteMsg(response)
		return
	}

	result, err := s.runControl(fields)
	if err != nil {
		setControlResponse(response, "error", controlProtocol, err.Error())
	} else {
		setControlResponse(response, append([]string{"ok", controlProtocol}, result...)...)
	}
	_ = w.WriteMsg(response)
}

func controlRequestFields(req *mdns.Msg) ([]string, error) {
	for _, extra := range req.Extra {
		txt, ok := extra.(*mdns.TXT)
		if !ok || !strings.EqualFold(txt.Hdr.Name, controlName) {
			continue
		}
		if len(txt.Txt) < 2 || txt.Txt[1] != controlProtocol {
			return nil, fmt.Errorf("unsupported control protocol")
		}
		return txt.Txt, nil
	}
	return nil, fmt.Errorf("missing Portless control command")
}

func (s *Server) runControl(fields []string) ([]string, error) {
	if len(fields) == 0 {
		return nil, fmt.Errorf("empty Portless control request")
	}
	if fields[0] != "ping" {
		if len(fields) < 3 {
			return nil, fmt.Errorf("missing Portless registry owner")
		}
		if err := validateRemoteOwner(fields[2]); err != nil {
			return nil, err
		}
	}
	switch fields[0] {
	case "ping":
		if len(fields) != 2 {
			return nil, fmt.Errorf("invalid ping request")
		}
		return nil, nil
	case "allocate":
		if len(fields) != 5 {
			return nil, fmt.Errorf("invalid allocate request")
		}
		port, err := strconv.Atoi(fields[3])
		if err != nil || port < 1 || port > 65535 {
			return nil, fmt.Errorf("invalid Portless port %q", fields[3])
		}
		entry, err := s.registry.allocateOwned(fields[4], port, fields[2])
		if err != nil {
			return nil, err
		}
		return []string{entry.Domain, entry.IP.String(), strconv.Itoa(entry.Port)}, nil
	case "release":
		if len(fields) != 4 {
			return nil, fmt.Errorf("invalid release request")
		}
		return nil, s.registry.releaseOwned(fields[3], fields[2])
	case "block":
		if len(fields) != 4 {
			return nil, fmt.Errorf("invalid block request")
		}
		ip := net.ParseIP(fields[3]).To4()
		if ip == nil {
			return nil, fmt.Errorf("invalid loopback address %q", fields[3])
		}
		return nil, s.registry.blockOwned(ip, fields[2])
	case "release-owner":
		if len(fields) != 3 {
			return nil, fmt.Errorf("invalid release-owner request")
		}
		s.registry.releaseOwner(fields[2])
		return nil, nil
	default:
		return nil, fmt.Errorf("unknown Portless control operation %q", fields[0])
	}
}

func validateRemoteOwner(owner string) error {
	if len(owner) < 3 || len(owner) > 63 || owner[0] != 'p' || owner == localRegistryOwner {
		return fmt.Errorf("invalid Portless registry owner")
	}
	for _, ch := range owner {
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '-' {
			continue
		}
		return fmt.Errorf("invalid Portless registry owner")
	}
	return nil
}

func setControlResponse(msg *mdns.Msg, fields ...string) {
	msg.Answer = []mdns.RR{&mdns.TXT{
		Hdr: mdns.RR_Header{Name: controlName, Rrtype: mdns.TypeTXT, Class: mdns.ClassINET},
		Txt: fields,
	}}
}
