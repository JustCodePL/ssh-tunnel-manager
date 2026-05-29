//go:build linux

package netcap

import (
	"os"
	"strings"
	"testing"
)

func TestIsPrivilegedPort(t *testing.T) {
	start := UnprivilegedPortStart()
	if start <= 0 {
		t.Fatalf("UnprivilegedPortStart() = %d, want > 0", start)
	}

	cases := []struct {
		port int
		want bool
	}{
		{0, false},        // 0 is "any port", never privileged
		{-1, false},       // nonsense, not privileged
		{80, true},        // classic privileged port
		{start - 1, true}, // just below the threshold
		{start, false},    // at the threshold is unprivileged
		{8080, false},     // high port
	}
	for _, c := range cases {
		if got := IsPrivilegedPort(c.port); got != c.want {
			t.Errorf("IsPrivilegedPort(%d) = %v, want %v (start=%d)", c.port, got, c.want, start)
		}
	}
}

func TestSetcapCommand(t *testing.T) {
	cmd := SetcapCommand("/home/user/.local/bin/ssh-tunnel-manager")
	for _, want := range []string{"setcap", CapName, "/home/user/.local/bin/ssh-tunnel-manager"} {
		if !strings.Contains(cmd, want) {
			t.Errorf("SetcapCommand() = %q, missing %q", cmd, want)
		}
	}
}

func TestHasBindServiceCapability_NoCaps(t *testing.T) {
	// A freshly written temp file has no file capabilities.
	f, err := os.CreateTemp(t.TempDir(), "stm-cap-*")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	has, err := HasBindServiceCapability(f.Name())
	if err != nil {
		t.Fatalf("HasBindServiceCapability() error = %v", err)
	}
	if has {
		t.Error("HasBindServiceCapability() = true for a file with no caps, want false")
	}
}
