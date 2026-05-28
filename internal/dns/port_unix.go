//go:build !windows

package dns

const (
	ListenPort = 5354
	BindIP     = "127.0.0.1"
)
