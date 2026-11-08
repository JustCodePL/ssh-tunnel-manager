package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

func newHostKey(t *testing.T) gossh.PublicKey {
	t.Helper()
	public, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	key, err := gossh.NewPublicKey(public)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func writeKnownHost(t *testing.T, address string, key gossh.PublicKey) {
	t.Helper()
	line := knownhosts.Line([]string{knownhosts.Normalize(address)}, key) + "\n"
	if err := os.MkdirAll(filepath.Dir(knownHostsPath()), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(knownHostsPath(), []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}
}

// answerHostKey waits for the question and answers it.
func answerHostKey(t *testing.T, app *App, accepted bool) *HostKeyPrompt {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if prompt := app.GetPendingHostKey(); prompt != nil {
			app.AnswerHostKey(prompt.ID, accepted)
			return prompt
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("no host key question")
	return nil
}

func checkAsync(app *App, tunnelID, address string, key gossh.PublicKey) <-chan error {
	result := make(chan error, 1)
	go func() { result <- app.checkSSHHostKey(tunnelID, address, key) }()
	return result
}

func TestKnownHostsKeyConnectsWithoutAQuestion(t *testing.T) {
	app := newTestApp(t, demoTunnel("t", ""))
	key := newHostKey(t)
	writeKnownHost(t, "example.com:22", key)
	if err := app.checkSSHHostKey("t", "example.com:22", key); err != nil {
		t.Fatal(err)
	}
	if got := app.GetSSHHostKeys("t"); len(got) != 0 {
		t.Errorf("pinned = %+v", got)
	}
}

func TestKnownHostsMismatchStopsTheConnection(t *testing.T) {
	app := newTestApp(t, demoTunnel("t", ""))
	writeKnownHost(t, "example.com:22", newHostKey(t))
	err := app.checkSSHHostKey("t", "example.com:22", newHostKey(t))
	if err == nil || !strings.Contains(err.Error(), "host key changed") || !strings.Contains(err.Error(), "known_hosts") {
		t.Fatalf("err = %v", err)
	}
}

func TestUnknownServerIsPinnedOnlyAfterTheUserTrustsIt(t *testing.T) {
	app := newTestApp(t, demoTunnel("t", ""))
	key := newHostKey(t)

	result := checkAsync(app, "t", "example.com:2222", key)
	prompt := answerHostKey(t, app, true)
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	if prompt.TunnelName != "t" || prompt.Address != "example.com:2222" || prompt.KeyType != "ssh-ed25519" || prompt.Fingerprint != gossh.FingerprintSHA256(key) {
		t.Errorf("prompt = %+v", prompt)
	}
	if app.GetPendingHostKey() != nil {
		t.Error("the question is still waiting")
	}

	// The accepted key connects with no further question.
	if err := app.checkSSHHostKey("t", "example.com:2222", key); err != nil {
		t.Fatal(err)
	}
	// Another key at the same address is refused, whatever the user might say.
	err := app.checkSSHHostKey("t", "example.com:2222", newHostKey(t))
	if err == nil || !strings.Contains(err.Error(), "host key changed") {
		t.Fatalf("changed key: %v", err)
	}
}

func TestDecliningAServerPinsNothing(t *testing.T) {
	app := newTestApp(t, demoTunnel("t", ""))
	result := checkAsync(app, "t", "example.com:22", newHostKey(t))
	answerHostKey(t, app, false)
	if err := <-result; err == nil || !strings.Contains(err.Error(), "was not trusted") {
		t.Fatalf("err = %v", err)
	}
	if got := app.GetSSHHostKeys("t"); len(got) != 0 {
		t.Errorf("pinned = %+v", got)
	}
}

func TestStaleHostKeyAnswerIsIgnored(t *testing.T) {
	app := newTestApp(t, demoTunnel("t", ""))
	result := checkAsync(app, "t", "example.com:22", newHostKey(t))
	deadline := time.Now().Add(5 * time.Second)
	for app.GetPendingHostKey() == nil && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	prompt := app.GetPendingHostKey()
	if prompt == nil {
		t.Fatal("no host key question")
	}
	app.AnswerHostKey(prompt.ID+1, true)
	select {
	case err := <-result:
		t.Fatalf("a stale answer decided the question: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	app.AnswerHostKey(prompt.ID, false)
	if err := <-result; err == nil {
		t.Fatal("expected a refusal")
	}
}

func TestShutdownRefusesWaitingAndLaterQuestions(t *testing.T) {
	app := newTestApp(t, demoTunnel("t", ""))
	result := checkAsync(app, "t", "example.com:22", newHostKey(t))
	deadline := time.Now().Add(5 * time.Second)
	for app.GetPendingHostKey() == nil && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	app.closeHostKeyPrompts()
	if err := <-result; err == nil {
		t.Fatal("expected a refusal")
	}
	if err := app.checkSSHHostKey("t", "other.example.com:22", newHostKey(t)); err == nil {
		t.Fatal("a question was asked after shutdown")
	}
}
