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
// HostHeaderOn opts in to HTTP-aware mode for a portless forward: the request
// Host header is rewritten before being sent upstream. It is OFF by default —
// the rewrite caused unexpected breakage for plain TCP / non-HTTP services, so
// it only happens when the user explicitly enables it on a forward.
//
// When HostHeaderOn is set, HostHeader (if non-empty) is the literal value sent
// upstream. If HostHeader is empty and RemoteHost looks like an FQDN
// (e.g. "dev.mix-dev.com"), RemoteHost is used automatically so a service
// behind a host-routing reverse proxy (Traefik, nginx, Portainer) "just works".
//
// When HostHeaderOn is false (the default), the forward is raw TCP: the
// browser's Host header (e.g. db.ssh-local) passes through unchanged.
type PortForward struct {
	LocalPort    int    `json:"localPort"`
	RemoteHost   string `json:"remoteHost"`
	RemotePort   int    `json:"remotePort"`
	Description  string `json:"description"`
	Portless     bool   `json:"portless,omitempty"`
	Domain       string `json:"domain,omitempty"`
	ExposePort   int    `json:"exposePort,omitempty"`
	HostHeader   string `json:"hostHeader,omitempty"`
	HostHeaderOn bool   `json:"hostHeaderOn,omitempty"`
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
