//go:build !darwin

// Package portless contains platform-specific details of Portless listeners.
package portless

// RequiresPrivilegedPortRedirect is false off macOS. Linux authorizes a
// direct low-port bind with CAP_NET_BIND_SERVICE; Windows binds directly.
func RequiresPrivilegedPortRedirect(int) bool { return false }

// ListenerPort is unchanged off macOS.
func ListenerPort(publicPort int) int { return publicPort }
