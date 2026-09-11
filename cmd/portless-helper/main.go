//go:build darwin

// portless-helper is the only executable launchd runs as root. Keep this
// entrypoint deliberately small: it can converge or remove only the
// machine-wide Portless network prerequisites.
package main

import (
	"log/slog"
	"os"

	"ssh-tunnel-manager/internal/dns"
)

func main() {
	var err error
	switch {
	case len(os.Args) == 1:
		err = dns.RunSetup(dns.SetupRequirements{PrivilegedPortRedirect: true})
	case len(os.Args) == 2 && os.Args[1] == dns.CleanupArg:
		err = dns.RunCleanup()
	default:
		slog.Error("unsupported Portless helper arguments")
		os.Exit(2)
	}
	if err != nil {
		slog.Error("Portless helper failed", "error", err)
		os.Exit(1)
	}
}
