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
type PortForward struct {
	LocalPort   int    `json:"localPort"`
	RemoteHost  string `json:"remoteHost"`
	RemotePort  int    `json:"remotePort"`
	Description string `json:"description"`
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
