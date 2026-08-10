//go:build darwin

package dns

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"strings"
)

const (
	resolverPath      = "/etc/resolver/ssh-local"
	loopbackInterface = "lo0"
	ifconfigPath      = "/sbin/ifconfig"
)

func isSystemConfigured() bool {
	return isResolverConfigured() && isLoopbackPoolConfigured()
}

func isResolverConfigured() bool {
	data, err := os.ReadFile(resolverPath)
	if err != nil {
		return false
	}
	content := string(data)
	return strings.Contains(content, "nameserver 127.0.0.1") &&
		strings.Contains(content, fmt.Sprintf("port %d", ListenPort))
}

// macOS, unlike Linux, doesn't treat every address in 127.0.0.0/8 as
// bindable automatically. Each address used by a portless listener must be
// explicitly assigned to lo0. The aliases are lost on reboot, so include them
// in the readiness check even though the resolver file is persistent.
func isLoopbackPoolConfigured() bool {
	configured, err := configuredLoopbackIPs()
	if err != nil {
		return false
	}
	for i := 0; i < loopbackPoolSize; i++ {
		if !configured[loopbackIP(i).String()] {
			return false
		}
	}
	return true
}

func configuredLoopbackIPs() (map[string]bool, error) {
	iface, err := net.InterfaceByName(loopbackInterface)
	if err != nil {
		return nil, err
	}
	addrs, err := iface.Addrs()
	if err != nil {
		return nil, err
	}

	configured := make(map[string]bool, len(addrs))
	for _, addr := range addrs {
		ip, _, err := net.ParseCIDR(addr.String())
		if err == nil {
			configured[ip.String()] = true
		}
	}
	return configured, nil
}

// doSetup assigns the portless loopback pool to lo0 and writes
// /etc/resolver/ssh-local pointing the macOS resolver at 127.0.0.1:5354 for
// the .ssh-local TLD. Must be called from a process that already has root
// (i.e. via osascript "do shell script ... with administrator privileges").
func doSetup() error {
	if err := configureLoopbackPool(); err != nil {
		return err
	}
	if err := os.MkdirAll("/etc/resolver", 0o755); err != nil {
		return fmt.Errorf("creating /etc/resolver: %w", err)
	}
	content := fmt.Sprintf("nameserver 127.0.0.1\nport %d\n", ListenPort)
	if err := os.WriteFile(resolverPath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", resolverPath, err)
	}
	slog.Info("portless: resolver file installed", "path", resolverPath)
	return nil
}

func configureLoopbackPool() error {
	configured, err := configuredLoopbackIPs()
	if err != nil {
		return fmt.Errorf("reading %s addresses: %w", loopbackInterface, err)
	}
	for i := 0; i < loopbackPoolSize; i++ {
		ip := loopbackIP(i).String()
		// /32 avoids changing the existing 127.0.0.1/8 route on lo0. Adding an
		// already-present alias is harmless for our intended one-shot setup,
		// but skipping it makes retries after a partial setup idempotent.
		if configured[ip] {
			continue
		}
		out, err := exec.Command(ifconfigPath, loopbackInterface, "inet", ip+"/32", "add").CombinedOutput()
		if err != nil {
			return fmt.Errorf("adding loopback alias %s: %w (%s)", ip, err, strings.TrimSpace(string(out)))
		}
		configured[ip] = true
	}
	slog.Info("portless: loopback aliases installed",
		"interface", loopbackInterface,
		"first", loopbackIP(0).String(),
		"last", loopbackIP(loopbackPoolSize-1).String())
	return nil
}

func runElevatedSetup(ctx context.Context, exe string) error {
	// AppleScript escaping: the path is wrapped in single quotes inside the
	// "do shell script" string. Escape embedded quotes defensively.
	safeExe := strings.ReplaceAll(exe, `"`, `\"`)
	script := fmt.Sprintf(
		`do shell script "\"%s\" %s" with administrator privileges`,
		safeExe, SetupArg,
	)
	cmd := exec.CommandContext(ctx, "osascript", "-e", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		trimmed := strings.TrimSpace(string(out))
		if strings.Contains(trimmed, "User canceled") || strings.Contains(trimmed, "(-128)") {
			return fmt.Errorf("admin prompt cancelled")
		}
		return fmt.Errorf("osascript: %w (%s)", err, trimmed)
	}
	return nil
}
