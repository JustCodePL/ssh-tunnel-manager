package ssh

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/pem"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"

	"ssh-tunnel-manager/internal/config"
	"ssh-tunnel-manager/internal/e2efixture"
)

// fakeAgent serves an in-process agent over pipes and counts the connections
// the client has not closed yet.
type fakeAgent struct {
	agent agent.Agent
	dials atomic.Int32
	open  sync.WaitGroup
}

func (f *fakeAgent) dial() (net.Conn, error) {
	client, server := net.Pipe()
	f.dials.Add(1)
	f.open.Add(1)
	go func() {
		defer f.open.Done()
		_ = agent.ServeAgent(f.agent, server)
		_ = server.Close()
	}()
	return client, nil
}

// waitClosed fails the test unless every agent connection is closed soon.
func (f *fakeAgent) waitClosed(t *testing.T) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		f.open.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("an agent connection was left open")
	}
}

// keyServer starts the fake server accepting only the public key of private,
// with an empty HOME so no key file can log in.
func keyServer(t *testing.T, private any) config.TunnelConfig {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	setup, err := e2efixture.PrepareHome(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	signer, err := gossh.NewSignerFromKey(private)
	if err != nil {
		t.Fatal(err)
	}
	server, err := e2efixture.New(setup.FilesDir, signer.PublicKey())
	if err != nil {
		t.Fatal(err)
	}
	return config.TunnelConfig{ID: "demo", Name: "demo", Host: "127.0.0.1", Port: server.Port, User: "demo"}
}

func ed25519FixtureKey(t *testing.T) any {
	t.Helper()
	setup, err := e2efixture.PrepareHome(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(setup.KeyPath)
	if err != nil {
		t.Fatal(err)
	}
	key, err := gossh.ParseRawPrivateKey(data)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

// TestMain points SSH_AUTH_SOCK at a socket that does not exist, so a test that
// forgets to replace the system agent dialer still never reaches the user's
// own agent. An empty value would not do: on macOS it falls back to launchd.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "stm-no-agent")
	if err != nil {
		panic(err)
	}
	_ = os.Setenv("SSH_AUTH_SOCK", filepath.Join(dir, "agent.sock"))
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

func unreachableAgent() (net.Conn, error) { return nil, errors.New("no agent here") }

// The agent's keys log in, and no connection to the agent outlives the login:
// leaked connections once exhausted the launchd agent's file descriptors, after
// which it accepted connections but never answered.
func TestAgentLoginClosesEveryAgentConnection(t *testing.T) {
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	keys := map[string]any{"ed25519": ed25519FixtureKey(t), "rsa": rsaKey}
	for name, key := range keys {
		t.Run(name, func(t *testing.T) {
			cfg := keyServer(t, key)
			keyring := agent.NewKeyring()
			if err := keyring.Add(agent.AddedKey{PrivateKey: key}); err != nil {
				t.Fatal(err)
			}
			fake := &fakeAgent{agent: keyring}
			for range 3 {
				if err := dialWithAgent(t, cfg, nil, trustAnyHostKey, fake.dial); err != nil {
					t.Fatalf("login with the agent's key: %v", err)
				}
			}
			fake.waitClosed(t)
			if fake.dials.Load() == 0 {
				t.Fatal("the agent was never asked")
			}
		})
	}
}

// An agent that cannot be reached is skipped: the saved password still logs
// in, and so does a default key file, which shares the one publickey method.
func TestUnreachableAgentLeavesTheOtherLogins(t *testing.T) {
	t.Run("saved password", func(t *testing.T) {
		_, cfg := newPasswordServer(t)
		err := dialWithAgent(t, cfg, staticLogin(&PasswordLogin{
			Username: e2efixture.PasswordUser,
			Password: e2efixture.Password,
		}), trustAnyHostKey, unreachableAgent)
		if err != nil {
			t.Fatal(err)
		}
	})
	t.Run("default key file", func(t *testing.T) {
		home := t.TempDir()
		setup, err := e2efixture.PrepareHome(home)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(setup.KeyPath, filepath.Join(home, ".ssh", "id_ed25519")); err != nil {
			t.Fatal(err)
		}
		t.Setenv("HOME", home)
		server, err := e2efixture.New(setup.FilesDir, setup.Signer.PublicKey())
		if err != nil {
			t.Fatal(err)
		}
		cfg := config.TunnelConfig{ID: "demo", Name: "demo", Host: "127.0.0.1", Port: server.Port, User: "demo"}
		if err := dialWithAgent(t, cfg, nil, trustAnyHostKey, unreachableAgent); err != nil {
			t.Fatal(err)
		}
	})
}

// Without a key, an agent key or a password the configuration fails at once
// with a clear reason instead of at the server.
func TestNoKeyAndNoAgentFailsEarly(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	empty := &fakeAgent{agent: agent.NewKeyring()}
	for name, dial := range map[string]AgentDialer{"unreachable": unreachableAgent, "empty": empty.dial} {
		t.Run(name, func(t *testing.T) {
			_, err := buildClientConfig(config.TunnelConfig{Name: "demo", Host: "example"}, nil, nil, trustAnyHostKey, dial)
			if err == nil || !strings.Contains(err.Error(), "no authentication methods available") {
				t.Fatalf("err = %v", err)
			}
		})
	}
	empty.waitClosed(t)
}

// Keys come from one source, as before the agent change, so a saved password
// is not crowded out by the server's MaxAuthTries: an explicit key file leaves
// the agent unasked, and agent keys leave the default key files unread.
func TestKeysComeFromOneSource(t *testing.T) {
	home := t.TempDir()
	setup, err := e2efixture.PrepareHome(home)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(setup.KeyPath, filepath.Join(home, ".ssh", "id_ed25519")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	keyring := agent.NewKeyring()
	if err := keyring.Add(agent.AddedKey{PrivateKey: ed25519FixtureKey(t)}); err != nil {
		t.Fatal(err)
	}
	fake := &fakeAgent{agent: keyring}

	explicit := config.TunnelConfig{Name: "demo", Host: "example", KeyPath: filepath.Join(home, ".ssh", "id_ed25519")}
	if _, err := buildClientConfig(explicit, nil, nil, trustAnyHostKey, fake.dial); err != nil {
		t.Fatal(err)
	}
	if n := fake.dials.Load(); n != 0 {
		t.Fatalf("the agent was asked %d times next to an explicit key", n)
	}

	unreadable := func(string) (string, error) {
		t.Error("a default key file was read although the agent has keys")
		return "", errors.New("unused")
	}
	// An encrypted default key would ask for its passphrase if it were read.
	block, err := gossh.MarshalPrivateKeyWithPassphrase(ed25519FixtureKey(t), "", []byte("secret"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".ssh", "id_ecdsa"), pem.EncodeToMemory(block), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := buildClientConfig(config.TunnelConfig{Name: "demo", Host: "example"}, unreadable, nil, trustAnyHostKey, fake.dial)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Auth) != 1 || fake.dials.Load() != 1 {
		t.Fatalf("auth methods = %d, agent dials = %d", len(cfg.Auth), fake.dials.Load())
	}
	fake.waitClosed(t)
}

// An agent that accepts the connection but never answers gives up after the
// timeout and its connection is closed.
func TestAgentThatNeverAnswersTimesOut(t *testing.T) {
	peerClosed := make(chan struct{})
	dial := func() (net.Conn, error) {
		client, server := net.Pipe()
		go func() {
			_, _ = io.Copy(io.Discard, server)
			close(peerClosed)
		}()
		return client, nil
	}
	start := time.Now()
	if _, err := agentSigners(dial, 100*time.Millisecond, time.Second); err == nil {
		t.Fatal("listing keys of a silent agent succeeded")
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("gave up after %s", elapsed)
	}
	select {
	case <-peerClosed:
	case <-time.After(5 * time.Second):
		t.Fatal("the connection to the silent agent was left open")
	}
}

// hangingSigner lists its keys but never answers a signature request.
type hangingSigner struct {
	agent.ExtendedAgent
	release chan struct{}
}

func (h *hangingSigner) Sign(gossh.PublicKey, []byte) (*gossh.Signature, error) {
	<-h.release
	return nil, errors.New("released")
}

func (h *hangingSigner) SignWithFlags(gossh.PublicKey, []byte, agent.SignatureFlags) (*gossh.Signature, error) {
	<-h.release
	return nil, errors.New("released")
}

func TestAgentSignatureThatNeverComesTimesOut(t *testing.T) {
	keyring := agent.NewKeyring().(agent.ExtendedAgent)
	if err := keyring.Add(agent.AddedKey{PrivateKey: ed25519FixtureKey(t)}); err != nil {
		t.Fatal(err)
	}
	hanging := &hangingSigner{ExtendedAgent: keyring, release: make(chan struct{})}
	fake := &fakeAgent{agent: hanging}
	t.Cleanup(func() {
		close(hanging.release)
		fake.waitClosed(t)
	})

	signers, err := agentSigners(fake.dial, time.Second, 100*time.Millisecond)
	if err != nil || len(signers) != 1 {
		t.Fatalf("signers = %d, err = %v", len(signers), err)
	}
	start := time.Now()
	if _, err := signers[0].Sign(rand.Reader, []byte("data")); err == nil {
		t.Fatal("a signature that never came succeeded")
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("gave up after %s", elapsed)
	}
}
