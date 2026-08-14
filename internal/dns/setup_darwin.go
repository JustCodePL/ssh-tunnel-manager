//go:build darwin

package dns

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"ssh-tunnel-manager/internal/portless"
)

const (
	resolverPath      = "/etc/resolver/ssh-local"
	loopbackInterface = "lo0"
	ifconfigPath      = "/sbin/ifconfig"
	pfctlPath         = "/sbin/pfctl"
	pfConfigPath      = "/etc/pf.conf"
	pfAnchor          = "com.apple/ssh-tunnel-manager"
	pfMarkerPath      = "/var/run/ssh-tunnel-manager-portless-pf-v1"
)

var appleRDRAnchorPattern = regexp.MustCompile(`(?m)^\s*rdr-anchor\s+"com\.apple/\*"`)

func isSystemConfigured() bool {
	return isResolverConfigured() && isLoopbackPoolConfigured()
}

func isPrivilegedPortRedirectConfigured() bool {
	_, err := os.Stat(pfMarkerPath)
	return err == nil
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

// doSetup assigns the Portless loopback pool to lo0, writes the resolver, and
// optionally installs the privileged-port PF redirect. It must be called from
// a process that already has root (via osascript administrator privileges).
func doSetup(requirements SetupRequirements) error {
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
	if requirements.PrivilegedPortRedirect {
		if err := configurePrivilegedPortRedirect(); err != nil {
			return err
		}
	}
	return nil
}

func privilegedPortRedirectRule() string {
	return fmt.Sprintf(
		"rdr pass on %s inet proto tcp from 127.0.0.0/8 to 127.0.1.0/24 port 1:%d -> 127.0.1.0/24 port %d:* bitmask\n",
		loopbackInterface,
		portless.PrivilegedPortStart-1,
		portless.ListenerPort(1),
	)
}

func configurePrivilegedPortRedirect() error {
	alreadyEnabledByUs := isPrivilegedPortRedirectConfigured()
	if !alreadyEnabledByUs {
		// Verify the per-boot marker location before acquiring a PF enable
		// reference. This avoids leaving an untracked reference on ordinary
		// directory/permission failures.
		if err := os.MkdirAll(filepath.Dir(pfMarkerPath), 0o755); err != nil {
			return fmt.Errorf("creating PF marker directory: %w", err)
		}
	}

	mainRules, err := os.ReadFile(pfConfigPath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", pfConfigPath, err)
	}
	if !appleRDRAnchorPattern.Match(mainRules) {
		return fmt.Errorf("%s does not expose the required rdr-anchor \"com.apple/*\"; refusing to modify the main PF ruleset", pfConfigPath)
	}

	rulesFile, err := os.CreateTemp("", "ssh-tunnel-manager-pf-*.conf")
	if err != nil {
		return fmt.Errorf("creating temporary PF rules file: %w", err)
	}
	rulesPath := rulesFile.Name()
	defer os.Remove(rulesPath)
	if err := rulesFile.Chmod(0o600); err != nil {
		rulesFile.Close()
		return fmt.Errorf("securing temporary PF rules file: %w", err)
	}
	if _, err := rulesFile.WriteString(privilegedPortRedirectRule()); err != nil {
		rulesFile.Close()
		return fmt.Errorf("writing temporary PF rules file: %w", err)
	}
	if err := rulesFile.Close(); err != nil {
		return fmt.Errorf("closing temporary PF rules file: %w", err)
	}

	if err := runPFCTL("-n", "-a", pfAnchor, "-f", rulesPath); err != nil {
		return fmt.Errorf("validating Portless PF redirect: %w", err)
	}
	if err := runPFCTL("-q", "-a", pfAnchor, "-f", rulesPath); err != nil {
		return fmt.Errorf("loading Portless PF redirect: %w", err)
	}
	if !alreadyEnabledByUs {
		enableOut, err := exec.Command(pfctlPath, "-E").CombinedOutput()
		if err != nil {
			return fmt.Errorf("enabling PF: %w (%s)", err, strings.TrimSpace(string(enableOut)))
		}

		// Keep pfctl's enable-reference token in the per-boot marker so a
		// future uninstall/cleanup flow can release exactly our reference.
		marker := fmt.Sprintf(
			"anchor=%s\nrule=%spfctl_enable=%s\n",
			pfAnchor,
			privilegedPortRedirectRule(),
			strings.TrimSpace(string(enableOut)),
		)
		if err := os.WriteFile(pfMarkerPath, []byte(marker), 0o644); err != nil {
			return fmt.Errorf("writing PF marker: %w", err)
		}
	}
	slog.Info("portless: privileged-port PF redirect installed",
		"anchor", pfAnchor,
		"newPFEnableReference", !alreadyEnabledByUs,
		"publicPorts", fmt.Sprintf("1-%d", portless.PrivilegedPortStart-1),
		"listenerPorts", fmt.Sprintf("%d-%d", portless.ListenerPort(1), portless.ListenerPort(portless.PrivilegedPortStart-1)))
	return nil
}

func runPFCTL(args ...string) error {
	out, err := exec.Command(pfctlPath, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("pfctl %s: %w (%s)", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
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

func runElevatedSetup(ctx context.Context, exe string, args []string) error {
	script, err := elevatedSetupScript(exe, args)
	if err != nil {
		return err
	}
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

func elevatedSetupScript(exe string, args []string) (string, error) {
	if strings.ContainsAny(exe, "\x00\r\n") {
		return "", fmt.Errorf("executable path contains unsupported control characters")
	}
	// `quoted form of` asks AppleScript to produce shell-safe single quoting.
	// Escape only the AppleScript string literal itself here; the shell never
	// receives the executable path as interpolated command text.
	appleScriptExe := strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(exe)
	return fmt.Sprintf(
		`do shell script ((quoted form of "%s") & " %s") with administrator privileges`,
		appleScriptExe,
		strings.Join(args, " "),
	), nil
}
