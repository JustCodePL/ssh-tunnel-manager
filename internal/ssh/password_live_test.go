package ssh

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"ssh-tunnel-manager/internal/config"
)

// TestSavedPasswordAgainstARealServer logs in to a real server with a password,
// without a key or agent. It runs only when asked:
//
//	STM_SSH_LIVE_HOST=host STM_SSH_LIVE_PORT=22 STM_SSH_LIVE_USER_FILE=path STM_SSH_LIVE_PASSWORD_FILE=path go test ./internal/ssh -run RealServer -v
func TestSavedPasswordAgainstARealServer(t *testing.T) {
	host := os.Getenv("STM_SSH_LIVE_HOST")
	if host == "" {
		t.Skip("set STM_SSH_LIVE_HOST to run")
	}
	read := func(name string) string {
		data, err := os.ReadFile(os.Getenv(name))
		if err != nil {
			t.Fatalf("reading %s", name)
		}
		return strings.TrimRight(string(data), "\r\n")
	}
	port, err := strconv.Atoi(os.Getenv("STM_SSH_LIVE_PORT"))
	if err != nil {
		t.Fatal("STM_SSH_LIVE_PORT must be a number")
	}
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SSH_AUTH_SOCK", "")
	cfg := config.TunnelConfig{ID: "live", Name: "live", Host: host, Port: port}
	err = dial(t, cfg, func(string) (*PasswordLogin, error) {
		return &PasswordLogin{
			Username: read("STM_SSH_LIVE_USER_FILE"),
			Password: read("STM_SSH_LIVE_PASSWORD_FILE"),
			CheckHostKey: func(address, fingerprint string) error {
				fmt.Println("host key", address, fingerprint)
				return nil
			},
		}, nil
	})
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}
}
