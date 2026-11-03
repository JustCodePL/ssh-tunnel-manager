package ssh

import (
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"ssh-tunnel-manager/internal/config"
)

// recorder collects the events and logs a Manager emits.
type recorder struct {
	mu     sync.Mutex
	events []StatusEvent
	logs   []string
}

func (r *recorder) emit(e StatusEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, e)
}

func (r *recorder) log(_, _, msg string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.logs = append(r.logs, msg)
}

func (r *recorder) failedAttempts() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, l := range r.logs {
		if strings.HasPrefix(l, "Connection failed: ") {
			n++
		}
	}
	return n
}

func (r *recorder) errorEvents() []StatusEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []StatusEvent
	for _, e := range r.events {
		if e.Status == config.StatusError {
			out = append(out, e)
		}
	}
	return out
}

func waitForEnd(t *testing.T, m *Manager, id string) {
	t.Helper()
	m.mu.Lock()
	done := m.tunnels[id].done
	m.mu.Unlock()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("tunnel kept retrying past the attempt limit")
	}
}

func TestConnectStopsAfterMaxAttempts(t *testing.T) {
	// A forward whose local port is taken makes every attempt fail, without
	// any network access, with an error that is neither auth nor DNS.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	cfg := config.TunnelConfig{
		ID:   "nas",
		Name: "nas",
		Host: "127.0.0.1",
		PortForwards: []config.PortForward{
			{LocalPort: ln.Addr().(*net.TCPAddr).Port, RemoteHost: "127.0.0.1", RemotePort: 80},
		},
	}

	rec := &recorder{}
	m := NewManager(rec.emit, nil)
	m.WithLogEmitter(rec.log)

	if err := m.Connect(cfg); err != nil {
		t.Fatal(err)
	}
	waitForEnd(t, m, cfg.ID)

	if got := rec.failedAttempts(); got != maxConnectAttempts {
		t.Errorf("failed attempts = %d, want %d", got, maxConnectAttempts)
	}
	errs := rec.errorEvents()
	if len(errs) != 1 {
		t.Fatalf("error events = %d, want exactly 1 (only the final one): %+v", len(errs), errs)
	}
	if !strings.Contains(errs[0].Error, "after 3 attempts") {
		t.Errorf("final error = %q, want it to mention the attempt count", errs[0].Error)
	}
	if last := rec.events[len(rec.events)-1]; last.Status != config.StatusError {
		t.Errorf("last status = %q, want error", last.Status)
	}
	if s := m.GetStatuses()[cfg.ID]; s.Status != config.StatusError || s.Error != errs[0].Error {
		t.Errorf("GetStatuses = %+v, want the final error", s)
	}

	// Connecting again by hand starts a fresh round of attempts.
	if err := m.Connect(cfg); err != nil {
		t.Fatal(err)
	}
	waitForEnd(t, m, cfg.ID)
	if got := rec.failedAttempts(); got != 2*maxConnectAttempts {
		t.Errorf("failed attempts after a manual retry = %d, want %d", got, 2*maxConnectAttempts)
	}
}
