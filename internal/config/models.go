package config

import "time"

// LogEntry holds a single log line for a tunnel connection.
type LogEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Level     string    `json:"level"` // "info", "warn", "error"
	Message   string    `json:"message"`
}

// TunnelStatus represents the current state of a tunnel.
type TunnelStatus string

const (
	StatusDisconnected TunnelStatus = "disconnected"
	StatusConnecting   TunnelStatus = "connecting"
	StatusConnected    TunnelStatus = "connected"
	StatusReconnecting TunnelStatus = "reconnecting"
	StatusError        TunnelStatus = "error"
)

// PortForward defines a single local-to-remote port mapping.
//
// LocalPort is the bind port for the standard 127.0.0.1 listener and is also
// what plain `ssh` writes to disk respects. It is preserved across toggles of
// Portless so the user does not lose their setting when switching modes.
//
// When Portless is true, the runtime instead binds on a dynamically allocated
// 127.0.1.x loopback address at ExposePort (or RemotePort if ExposePort is 0).
// Domain is published under the .ssh-local TLD by the embedded DNS server.
//
// HostHeader, when set, switches the portless forward to HTTP-aware mode and
// rewrites the request Host header before sending upstream. Empty + RemoteHost
// that looks like an FQDN auto-uses RemoteHost as the header (so a Portainer
// behind a host-routing reverse proxy "just works"). Empty + non-FQDN remote
// keeps raw TCP — the browser's Host header (e.g. db.ssh-local) is sent
// through unchanged.
//
// HostHeaderOff opts out of the rewrite entirely (raw TCP), even when the
// FQDN auto-rule would otherwise kick in. UI default is the override-on
// state, so this flag only appears in ~/.ssh/config when the user has
// explicitly disabled it.
type PortForward struct {
	LocalPort     int    `json:"localPort"`
	RemoteHost    string `json:"remoteHost"`
	RemotePort    int    `json:"remotePort"`
	Description   string `json:"description"`
	Portless      bool   `json:"portless,omitempty"`
	Domain        string `json:"domain,omitempty"`
	ExposePort    int    `json:"exposePort,omitempty"`
	HostHeader    string `json:"hostHeader,omitempty"`
	HostHeaderOff bool   `json:"hostHeaderOff,omitempty"`
}

// TunnelConfig holds the persistent configuration for a single SSH tunnel.
type TunnelConfig struct {
	ID              string        `json:"id"`
	Name            string        `json:"name"`
	Host            string        `json:"host"`
	Port            int           `json:"port"`
	User            string        `json:"user"`
	KeyPath         string        `json:"keyPath"`
	PortForwards    []PortForward `json:"portForwards"`
	ProxyCommand    string        `json:"proxyCommand,omitempty"`
	ProxyJump       string        `json:"proxyJump,omitempty"`
	Color           string        `json:"color,omitempty"`
	Group           string        `json:"group"`
	AutoConnect     bool          `json:"autoConnect"`
	Pinned          bool          `json:"pinned"`
	SourceFile      string        `json:"sourceFile,omitempty"`
	SourceFileLabel string        `json:"sourceFileLabel,omitempty"`
}

// ConfigFileInfo describes an SSH config file that can be edited.
type ConfigFileInfo struct {
	Path  string `json:"path"`
	Label string `json:"label"`
}
