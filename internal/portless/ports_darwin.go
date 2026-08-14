//go:build darwin

// Package portless contains platform-specific details of Portless listeners.
package portless

const (
	// PrivilegedPortStart is the first TCP port the macOS app can bind without
	// elevated privileges on the supported systems.
	PrivilegedPortStart = 1024

	// RedirectOffset keeps the unprivileged listener range disjoint from the
	// public privileged range: public 80 listens on 10080, 443 on 10443, etc.
	RedirectOffset = 10000
)

// RequiresPrivilegedPortRedirect reports whether macOS PF must translate the
// public port before traffic reaches the unprivileged app process.
func RequiresPrivilegedPortRedirect(publicPort int) bool {
	return publicPort > 0 && publicPort < PrivilegedPortStart
}

// ListenerPort returns the actual port the unprivileged app binds. PF keeps
// the public *.ssh-local port unchanged from the caller's perspective.
func ListenerPort(publicPort int) int {
	if RequiresPrivilegedPortRedirect(publicPort) {
		return publicPort + RedirectOffset
	}
	return publicPort
}
