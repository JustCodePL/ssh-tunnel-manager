// Package dns implements the portless mode: an embedded DNS server that
// resolves *.ssh-local names to dynamically allocated loopback addresses
// (127.0.1.x), plus the per-platform admin setup needed for the OS resolver
// to forward those queries to us.
package dns

import (
	"fmt"
	"net"
	"strings"
	"sync"
)

// TLD is the suffix attached to all portless domain names. Chosen instead of
// .local to avoid colliding with mDNS / Bonjour.
const TLD = "ssh-local"

// LoopbackBase is the first usable IP in the loopback pool. The pool covers
// 127.0.1.1 through 127.0.1.254 — 254 concurrent portless forwards.
var loopbackBase = net.IPv4(127, 0, 1, 1)

// Entry describes a single live portless forward registered with the DNS
// server. Domain is the user-facing name (without the .ssh-local suffix).
type Entry struct {
	Domain string
	IP     net.IP
	Port   int
}

// Registry tracks portless domain → (ip, port) bindings backing the embedded
// DNS server. It allocates loopback IPs from a fixed pool on demand and
// recycles them on release.
type Registry struct {
	mu        sync.RWMutex
	byDomain  map[string]Entry
	allocated map[string]bool // ip.String() → true
	blocked   map[string]bool // IPs that failed to bind this session
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{
		byDomain:  make(map[string]Entry),
		allocated: make(map[string]bool),
		blocked:   make(map[string]bool),
	}
}

// Block marks an IP as unusable for the remainder of this session. Useful
// when binding a listener on the IP fails (something outside our process is
// holding it). Allocate will skip blocked IPs.
func (r *Registry) Block(ip net.IP) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.blocked[ip.String()] = true
}

// Allocate binds a domain to a free loopback IP and the given port. If the
// domain is already registered, the existing entry is returned unchanged so
// repeated calls during reconnects are idempotent. Returns an error if the
// pool is exhausted.
func (r *Registry) Allocate(domain string, port int) (Entry, error) {
	domain = normalizeDomain(domain)
	if domain == "" {
		return Entry{}, fmt.Errorf("empty domain")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if existing, ok := r.byDomain[domain]; ok {
		return existing, nil
	}

	ip := r.findFreeIPLocked()
	if ip == nil {
		return Entry{}, fmt.Errorf("loopback IP pool exhausted (>254 portless forwards)")
	}

	entry := Entry{Domain: domain, IP: ip, Port: port}
	r.byDomain[domain] = entry
	r.allocated[ip.String()] = true
	return entry, nil
}

// Release removes the binding for a domain. No-op if the domain isn't
// registered.
func (r *Registry) Release(domain string) {
	domain = normalizeDomain(domain)
	r.mu.Lock()
	defer r.mu.Unlock()
	if entry, ok := r.byDomain[domain]; ok {
		delete(r.allocated, entry.IP.String())
		delete(r.byDomain, domain)
	}
}

// Lookup returns the entry for a domain (without TLD), or false if absent.
func (r *Registry) Lookup(domain string) (Entry, bool) {
	domain = normalizeDomain(domain)
	r.mu.RLock()
	defer r.mu.RUnlock()
	entry, ok := r.byDomain[domain]
	return entry, ok
}

// LookupFQDN resolves a fully-qualified name like "db.foo.ssh-local." into
// an entry. Returns false if the name isn't under our TLD or isn't registered.
func (r *Registry) LookupFQDN(fqdn string) (Entry, bool) {
	name := strings.TrimSuffix(strings.ToLower(fqdn), ".")
	suffix := "." + TLD
	if !strings.HasSuffix(name, suffix) {
		return Entry{}, false
	}
	domain := strings.TrimSuffix(name, suffix)
	return r.Lookup(domain)
}

// All returns a snapshot of every active entry.
func (r *Registry) All() []Entry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Entry, 0, len(r.byDomain))
	for _, e := range r.byDomain {
		out = append(out, e)
	}
	return out
}

func (r *Registry) findFreeIPLocked() net.IP {
	// Walk 127.0.1.1 .. 127.0.1.254
	base := loopbackBase.To4()
	for i := 0; i < 254; i++ {
		ip := net.IPv4(base[0], base[1], base[2], base[3]+byte(i)).To4()
		key := ip.String()
		if r.allocated[key] || r.blocked[key] {
			continue
		}
		return ip
	}
	return nil
}

func normalizeDomain(d string) string {
	return strings.ToLower(strings.TrimSpace(d))
}
