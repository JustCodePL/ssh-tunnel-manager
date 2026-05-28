//go:build windows

package dns

const (
	ListenPort = 53
	// BindIP is a dedicated loopback address — using 127.0.0.1 collides with
	// any Windows process that already binds DNS port 53 (Docker Desktop,
	// WSL2, Pi-hole, some VPN clients). 127.0.0.53 mirrors the convention
	// used by systemd-resolved on Linux.
	BindIP = "127.0.0.53"
)
