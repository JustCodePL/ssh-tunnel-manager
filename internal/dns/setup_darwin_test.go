//go:build darwin

package dns

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrivilegedPortRedirectRule(t *testing.T) {
	const want = "rdr pass on lo0 inet proto tcp from 127.0.0.0/8 to 127.0.1.0/24 port 1:1023 -> 127.0.1.0/24 port 10001:* bitmask\n"
	if got := privilegedPortRedirectRule(); got != want {
		t.Fatalf("privilegedPortRedirectRule() = %q, want %q", got, want)
	}
}

func TestPrivilegedPortRedirectRuleParsesWithPFCTL(t *testing.T) {
	if _, err := os.Stat(pfctlPath); err != nil {
		t.Skipf("pfctl unavailable: %v", err)
	}
	rulesPath := filepath.Join(t.TempDir(), "portless.conf")
	if err := os.WriteFile(rulesPath, []byte(privilegedPortRedirectRule()), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(pfctlPath, "-n", "-f", rulesPath).CombinedOutput()
	if err != nil {
		t.Fatalf("pfctl rejected redirect rule: %v\n%s", err, out)
	}
}

func TestAppleRDRAnchorPattern(t *testing.T) {
	valid := `
scrub-anchor "com.apple/*"
rdr-anchor "com.apple/*"
anchor "com.apple/*"
`
	if !appleRDRAnchorPattern.MatchString(valid) {
		t.Fatal("default macOS rdr-anchor was not detected")
	}
	for _, invalid := range []string{
		`rdr-anchor "third-party/*"`,
		`# rdr-anchor "com.apple/*"`,
		`anchor "com.apple/*"`,
	} {
		if appleRDRAnchorPattern.MatchString(invalid) {
			t.Fatalf("unexpected anchor match for %q", strings.TrimSpace(invalid))
		}
	}
}

func TestElevatedSetupScriptUsesAppleScriptShellQuoting(t *testing.T) {
	exe := `/Applications/SSH $Tunnel "Beta".app/Contents/MacOS/ssh-tunnel-manager`
	script, err := elevatedSetupScript(exe, []string{SetupArg, PrivilegedRedirectArg})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(script, `quoted form of`) {
		t.Fatalf("script does not use AppleScript shell quoting: %s", script)
	}
	if !strings.Contains(script, `SSH $Tunnel \"Beta\".app`) {
		t.Fatalf("script does not escape the AppleScript path literal: %s", script)
	}
	if !strings.Contains(script, SetupArg+" "+PrivilegedRedirectArg) {
		t.Fatalf("script does not include setup flags: %s", script)
	}
}

func TestElevatedSetupScriptRejectsNewline(t *testing.T) {
	if _, err := elevatedSetupScript("/tmp/bad\npath", []string{SetupArg}); err == nil {
		t.Fatal("expected executable path with newline to be rejected")
	}
}

func TestElevatedSetupScriptCompiles(t *testing.T) {
	osacompile, err := exec.LookPath("osacompile")
	if err != nil {
		t.Skipf("osacompile unavailable: %v", err)
	}
	script, err := elevatedSetupScript(
		`/Applications/SSH Tunnel Manager.app/Contents/MacOS/ssh-tunnel-manager`,
		[]string{SetupArg, PrivilegedRedirectArg},
	)
	if err != nil {
		t.Fatal(err)
	}
	outputPath := filepath.Join(t.TempDir(), "setup.scpt")
	out, err := exec.Command(osacompile, "-o", outputPath, "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("osacompile rejected elevated setup script: %v\n%s", err, out)
	}
}

func TestPFEnableTokenPattern(t *testing.T) {
	marker := []byte("anchor=com.apple/ssh-tunnel-manager\nrule=rdr pass ...\npfctl_enable=pf enabled\nToken : 123456789\n")
	match := pfEnableTokenPattern.FindSubmatch(marker)
	if len(match) != 2 || string(match[1]) != "123456789" {
		t.Fatalf("pfEnableTokenPattern match = %q, want token 123456789", match)
	}
}

func TestExecutableDigest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "helper")
	if err := os.WriteFile(path, []byte("signed helper bytes"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := executableDigest(path)
	if err != nil {
		t.Fatal(err)
	}
	const want = "842a832965515063d4d08ce0285d12b879472088b7e9ea6f1f60000a23720968"
	if got != want {
		t.Fatalf("executableDigest() = %q, want %q", got, want)
	}
}

func TestPortlessLaunchDaemonPlist(t *testing.T) {
	plistBuddy := "/usr/libexec/PlistBuddy"
	if _, err := os.Stat(plistBuddy); err != nil {
		t.Skipf("PlistBuddy unavailable: %v", err)
	}
	plist := filepath.Join("..", "..", "build", "darwin", portlessServicePlistName)
	checks := map[string]string{
		"Print :Label":                         "pl.justcode.ssh-tunnel-manager.portless",
		"Print :AssociatedBundleIdentifiers:0": "com.wails.ssh-tunnel-manager",
		"Print :BundleProgram":                 "Contents/Library/HelperTools/ssh-tunnel-manager-portless",
		"Print :ProgramArguments:0":            portlessServiceHelper,
		"Print :RunAtLoad":                     "true",
	}
	for command, want := range checks {
		out, err := exec.Command(plistBuddy, "-c", command, plist).CombinedOutput()
		if err != nil {
			t.Fatalf("PlistBuddy %q failed: %v\n%s", command, err, out)
		}
		if got := strings.TrimSpace(string(out)); got != want {
			t.Errorf("PlistBuddy %q = %q, want %q", command, got, want)
		}
	}
}
