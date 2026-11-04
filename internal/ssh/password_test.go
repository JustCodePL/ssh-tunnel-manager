package ssh

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	gossh "golang.org/x/crypto/ssh"

	"ssh-tunnel-manager/internal/config"
	"ssh-tunnel-manager/internal/e2efixture"
)

// newPasswordServer starts the fake server without a key or agent available to
// the client, so a connection can only succeed with the saved password.
func newPasswordServer(t *testing.T) (*e2efixture.Server, config.TunnelConfig) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SSH_AUTH_SOCK", "")
	setup, err := e2efixture.PrepareHome(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	server, err := e2efixture.New(setup.FilesDir, setup.Signer.PublicKey())
	if err != nil {
		t.Fatal(err)
	}
	return server, config.TunnelConfig{ID: "demo", Name: "demo", Host: "127.0.0.1", Port: server.Port, User: "someone-else"}
}

func staticLogin(login *PasswordLogin) LoginFunc {
	return func(string) (*PasswordLogin, error) { return login, nil }
}

func dial(t *testing.T, cfg config.TunnelConfig, getLogin LoginFunc) error {
	t.Helper()
	clientConfig, err := buildClientConfig(cfg, nil, getLogin)
	if err != nil {
		return err
	}
	client, err := gossh.Dial("tcp", fmt.Sprintf("%s:%d", cfg.Host, cfg.Port), clientConfig)
	if err != nil {
		return err
	}
	return client.Close()
}

func TestSavedPasswordLogsInWithTheLoginsUser(t *testing.T) {
	_, cfg := newPasswordServer(t)
	var seen []string
	err := dial(t, cfg, staticLogin(&PasswordLogin{
		Username: e2efixture.PasswordUser,
		Password: e2efixture.Password,
		CheckHostKey: func(address, fingerprint string) error {
			seen = append(seen, fingerprint)
			if address != fmt.Sprintf("127.0.0.1:%d", cfg.Port) {
				t.Errorf("address = %q", address)
			}
			return nil
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if len(seen) != 1 || !strings.HasPrefix(seen[0], "SHA256:") {
		t.Errorf("host key check saw %v", seen)
	}
}

func TestWrongSavedPasswordIsRejected(t *testing.T) {
	_, cfg := newPasswordServer(t)
	err := dial(t, cfg, staticLogin(&PasswordLogin{
		Username:     e2efixture.PasswordUser,
		Password:     "wrong",
		CheckHostKey: func(string, string) error { return nil },
	}))
	if err == nil || !strings.Contains(err.Error(), "unable to authenticate") {
		t.Fatalf("err = %v", err)
	}
}

// The check runs before authentication, so a refused server never receives the
// password.
func TestChangedHostKeyStopsTheLogin(t *testing.T) {
	_, cfg := newPasswordServer(t)
	refused := errors.New("host key changed")
	err := dial(t, cfg, staticLogin(&PasswordLogin{
		Username:     e2efixture.PasswordUser,
		Password:     e2efixture.Password,
		CheckHostKey: func(string, string) error { return refused },
	}))
	if !errors.Is(err, refused) {
		t.Fatalf("err = %v", err)
	}
}

func TestALoginWithoutAHostKeyCheckIsRefused(t *testing.T) {
	_, cfg := newPasswordServer(t)
	err := dial(t, cfg, staticLogin(&PasswordLogin{Username: e2efixture.PasswordUser, Password: e2efixture.Password}))
	if err == nil || !strings.Contains(err.Error(), "host key check") {
		t.Fatalf("err = %v", err)
	}
}

func TestLoginFailureStopsTheConnection(t *testing.T) {
	_, cfg := newPasswordServer(t)
	failed := errors.New("keychain locked")
	err := dial(t, cfg, func(string) (*PasswordLogin, error) { return nil, failed })
	if !errors.Is(err, failed) {
		t.Fatalf("err = %v", err)
	}
}

func TestUnusableKeyOnlyMattersWithoutALogin(t *testing.T) {
	_, cfg := newPasswordServer(t)
	cfg.KeyPath = "/does/not/exist"
	if err := dial(t, cfg, staticLogin(nil)); err == nil || !strings.Contains(err.Error(), "reading key") {
		t.Fatalf("without a login: %v", err)
	}
	err := dial(t, cfg, staticLogin(&PasswordLogin{
		Username:     e2efixture.PasswordUser,
		Password:     e2efixture.Password,
		CheckHostKey: func(string, string) error { return nil },
	}))
	if err != nil {
		t.Fatalf("with a login: %v", err)
	}
}

func TestPasswordChallengeOnlyAnswersOneHiddenQuestion(t *testing.T) {
	cases := []struct {
		name      string
		questions []string
		echos     []bool
		want      []string
		fails     bool
	}{
		{name: "no question"},
		{name: "password prompt", questions: []string{"Password: "}, echos: []bool{false}, want: []string{"p"}},
		{name: "visible prompt", questions: []string{"Username: "}, echos: []bool{true}, fails: true},
		{name: "second factor too", questions: []string{"Password: ", "Code: "}, echos: []bool{false, false}, fails: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := answerPasswordChallenge("p", c.questions, c.echos)
			if (err != nil) != c.fails {
				t.Fatalf("err = %v", err)
			}
			if strings.Join(got, ",") != strings.Join(c.want, ",") {
				t.Errorf("answers = %v", got)
			}
		})
	}
}

func TestManagerConnectsWithASavedPassword(t *testing.T) {
	_, cfg := newPasswordServer(t)
	connected := make(chan struct{}, 1)
	m := NewManager(func(event StatusEvent) {
		if event.Status == config.StatusConnected {
			select {
			case connected <- struct{}{}:
			default:
			}
		}
	}, nil)
	m.WithLoginProvider(staticLogin(&PasswordLogin{
		Username:     e2efixture.PasswordUser,
		Password:     e2efixture.Password,
		CheckHostKey: func(string, string) error { return nil },
	}))
	if err := m.Connect(cfg); err != nil {
		t.Fatal(err)
	}
	defer m.DisconnectAll()
	select {
	case <-connected:
	case <-time.After(10 * time.Second):
		t.Fatal("the tunnel did not connect")
	}
}

func TestARejectedPasswordIsNotRetried(t *testing.T) {
	for _, message := range []string{
		"ssh: handshake failed: ssh: unable to authenticate, attempted methods [none password], no supported methods remain",
		"ssh: handshake failed: ssh: unexpected message type 51 (expected 60)",
	} {
		if !isAuthError(errors.New(message)) {
			t.Errorf("not an authentication error: %s", message)
		}
	}
	if !isHostKeyRefusal(errors.New("connecting: the host key changed for a:22")) {
		t.Error("a host key refusal was not recognized")
	}
	if isHostKeyRefusal(errors.New("connection refused")) {
		t.Error("an ordinary failure counted as a host key refusal")
	}
}
