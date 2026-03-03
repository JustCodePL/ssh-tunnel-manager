package ssh

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"sync"
	"time"

	"ssh-tunnel-manager/internal/config"
)

const (
	initialBackoff = 1 * time.Second
	maxBackoff     = 60 * time.Second
	backoffFactor  = 2.0
)

// StatusEvent is emitted when a tunnel's status changes.
type StatusEvent struct {
	TunnelID string              `json:"tunnelId"`
	Status   config.TunnelStatus `json:"status"`
	Error    string              `json:"error,omitempty"`
}

// EventEmitter is called whenever a tunnel status changes.
// The App layer wires this to Wails runtime.EventsEmit.
type EventEmitter func(event StatusEvent)

// Manager tracks running tunnels, one goroutine per tunnel.
type Manager struct {
	mu            sync.Mutex
	tunnels       map[string]*runningTunnel
	emitter       EventEmitter
	getPassphrase PassphraseFunc
	logEmitter    func(tunnelID, level, msg string)
}

// WithLogEmitter sets a callback that receives per-tunnel log entries.
func (m *Manager) WithLogEmitter(fn func(tunnelID, level, msg string)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.logEmitter = fn
}

// isAuthError returns true if the error looks like an SSH authentication failure.
func isAuthError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unable to authenticate") ||
		strings.Contains(msg, "no supported methods remain") ||
		strings.Contains(msg, "permission denied") ||
		strings.Contains(msg, "publickey") ||
		strings.Contains(msg, "authentication failed") ||
		strings.Contains(msg, "handshake failed")
}

type runningTunnel struct {
	cancel context.CancelFunc
	status config.TunnelStatus
	errMsg string
}

// NewManager creates a Manager with the given event callback.
func NewManager(emitter EventEmitter, getPassphrase PassphraseFunc) *Manager {
	return &Manager{
		tunnels:       make(map[string]*runningTunnel),
		emitter:       emitter,
		getPassphrase: getPassphrase,
	}
}

// Connect starts a tunnel in a new goroutine. Returns immediately.
// If the tunnel drops unexpectedly, it will auto-reconnect with
// exponential backoff until explicitly disconnected.
func (m *Manager) Connect(cfg config.TunnelConfig) error {
	m.mu.Lock()
	if rt, exists := m.tunnels[cfg.ID]; exists && rt.status == config.StatusConnected {
		m.mu.Unlock()
		return fmt.Errorf("tunnel %q is already connected", cfg.Name)
	}

	ctx, cancel := context.WithCancel(context.Background())
	rt := &runningTunnel{cancel: cancel, status: config.StatusConnecting}
	m.tunnels[cfg.ID] = rt
	m.mu.Unlock()

	m.emit(StatusEvent{TunnelID: cfg.ID, Status: config.StatusConnecting})

	go m.runWithReconnect(ctx, cancel, cfg, rt)

	return nil
}

func (m *Manager) runWithReconnect(ctx context.Context, cancel context.CancelFunc, cfg config.TunnelConfig, rt *runningTunnel) {
	backoff := initialBackoff
	firstAttempt := true
	attempt := 0

	for {
		if ctx.Err() != nil {
			return
		}

		if !firstAttempt {
			m.mu.Lock()
			rt.status = config.StatusReconnecting
			m.mu.Unlock()
			reconnMsg := fmt.Sprintf("reconnecting in %s", backoff.Truncate(time.Second))
			m.emitLog(cfg.ID, "warn", fmt.Sprintf("Reconnecting in %s (attempt %d)", backoff.Truncate(time.Second), attempt))
			m.emit(StatusEvent{
				TunnelID: cfg.ID,
				Status:   config.StatusReconnecting,
				Error:    reconnMsg,
			})

			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}

			if ctx.Err() != nil {
				return
			}

			m.mu.Lock()
			rt.status = config.StatusConnecting
			m.mu.Unlock()
			m.emit(StatusEvent{TunnelID: cfg.ID, Status: config.StatusConnecting})
		}
		firstAttempt = false
		attempt++

		tunnelID := cfg.ID
		m.mu.Lock()
		logEmitter := m.logEmitter
		m.mu.Unlock()

		tun := &Tunnel{
			Config:        cfg,
			GetPassphrase: m.getPassphrase,
			OnConnected: func() {
				m.mu.Lock()
				rt.status = config.StatusConnected
				rt.errMsg = ""
				m.mu.Unlock()
				m.emit(StatusEvent{TunnelID: cfg.ID, Status: config.StatusConnected})
				backoff = initialBackoff // reset backoff on successful connection
			},
		}
		if logEmitter != nil {
			tun.LogFunc = func(level, msg string) {
				logEmitter(tunnelID, level, msg)
			}
		}
		err := tun.Connect(ctx)

		if ctx.Err() != nil {
			// Intentional disconnect
			m.mu.Lock()
			rt.status = config.StatusDisconnected
			rt.errMsg = ""
			m.mu.Unlock()
			m.emit(StatusEvent{TunnelID: cfg.ID, Status: config.StatusDisconnected})
			return
		}

		if err != nil {
			errMsg := err.Error()
			slog.Error("tunnel connection failed", "tunnel", cfg.Name, "error", err, "backoff", backoff)

			// Don't retry on authentication errors
			if isAuthError(err) {
				authMsg := "Authentication failed — check your SSH key. Not retrying."
				m.emitLog(cfg.ID, "error", authMsg)
				m.mu.Lock()
				rt.status = config.StatusError
				rt.errMsg = authMsg
				m.mu.Unlock()
				m.emit(StatusEvent{TunnelID: cfg.ID, Status: config.StatusError, Error: authMsg})
				cancel()
				return
			}

			m.emitLog(cfg.ID, "error", fmt.Sprintf("Connection failed: %s", errMsg))
			m.mu.Lock()
			rt.status = config.StatusError
			rt.errMsg = errMsg
			m.mu.Unlock()
			m.emit(StatusEvent{TunnelID: cfg.ID, Status: config.StatusError, Error: errMsg})

			// Increase backoff for next attempt
			backoff = time.Duration(math.Min(
				float64(backoff)*backoffFactor,
				float64(maxBackoff),
			))
			continue // retry
		}

		// Connection ended without error (e.g. server closed) — also reconnect
		slog.Info("tunnel connection ended, will reconnect", "tunnel", cfg.Name)
		backoff = initialBackoff
	}
}

// Disconnect stops a running tunnel. Cancels the context so the reconnect
// loop exits.
func (m *Manager) Disconnect(tunnelID string) error {
	m.mu.Lock()
	rt, exists := m.tunnels[tunnelID]
	if !exists {
		m.mu.Unlock()
		return fmt.Errorf("tunnel %q not found in manager", tunnelID)
	}
	rt.cancel()
	rt.status = config.StatusDisconnected
	rt.errMsg = ""
	m.mu.Unlock()

	m.emit(StatusEvent{TunnelID: tunnelID, Status: config.StatusDisconnected})
	return nil
}

// DisconnectAll stops all running tunnels.
func (m *Manager) DisconnectAll() {
	m.mu.Lock()
	for _, rt := range m.tunnels {
		rt.cancel()
		rt.status = config.StatusDisconnected
		rt.errMsg = ""
	}
	m.mu.Unlock()
}

// GetStatuses returns the current status of all tracked tunnels.
func (m *Manager) GetStatuses() map[string]StatusEvent {
	m.mu.Lock()
	defer m.mu.Unlock()

	out := make(map[string]StatusEvent, len(m.tunnels))
	for id, rt := range m.tunnels {
		out[id] = StatusEvent{
			TunnelID: id,
			Status:   rt.status,
			Error:    rt.errMsg,
		}
	}
	return out
}

func (m *Manager) emit(event StatusEvent) {
	if m.emitter != nil {
		m.emitter(event)
	}
}

func (m *Manager) emitLog(tunnelID, level, msg string) {
	m.mu.Lock()
	fn := m.logEmitter
	m.mu.Unlock()
	if fn != nil {
		fn(tunnelID, level, msg)
	}
}
