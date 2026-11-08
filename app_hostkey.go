package main

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	gossh "golang.org/x/crypto/ssh"

	"ssh-tunnel-manager/internal/ssh"
)

// hostKeyAnswerTimeout limits how long a connection waits for the user. An SSH
// server usually drops a client that has not logged in after two minutes.
const hostKeyAnswerTimeout = 90 * time.Second

// HostKeyPrompt asks the user whether to trust the key of an unknown server.
type HostKeyPrompt struct {
	ID          uint64 `json:"id"`
	TunnelName  string `json:"tunnelName"`
	Address     string `json:"address"`
	KeyType     string `json:"keyType"`
	Fingerprint string `json:"fingerprint"`
}

type pendingHostKey struct {
	prompt HostKeyPrompt
	answer chan bool
}

// checkSSHHostKey decides whether a server may be talked to. A key pinned for
// this tunnel is compared first, then ~/.ssh/known_hosts. A server known to
// neither is the user's decision, including during auto-connect and reconnect.
func (a *App) checkSSHHostKey(tunnelID, address string, key gossh.PublicKey) error {
	fingerprint := gossh.FingerprintSHA256(key)
	if pinned, err := a.comparePinnedHostKey(tunnelID, address, fingerprint); pinned || err != nil {
		return err
	}

	switch ssh.CheckKnownHosts(knownHostsPath(), address, key) {
	case ssh.KnownHostsMatch:
		return nil
	case ssh.KnownHostsMismatch:
		return fmt.Errorf("the host key changed for %s: %s does not match ~/.ssh/known_hosts, so the connection was stopped. If this change is expected, update known_hosts", address, fingerprint)
	case ssh.KnownHostsRevoked:
		return fmt.Errorf("the host key of %s is revoked in ~/.ssh/known_hosts, so it was not trusted", address)
	}

	// One question at a time. While this connection waited its turn, another one
	// to the same server may have got an answer.
	a.hostKeyPromptMu.Lock()
	defer a.hostKeyPromptMu.Unlock()
	if pinned, err := a.comparePinnedHostKey(tunnelID, address, fingerprint); pinned || err != nil {
		return err
	}
	if !a.askHostKey(tunnelID, address, key.Type(), fingerprint) {
		return fmt.Errorf("the host key of %s (%s) was not trusted, so the connection was stopped", address, fingerprint)
	}
	return a.pinSSHHostKey(tunnelID, address, fingerprint)
}

// comparePinnedHostKey reports whether the key is the one pinned for this
// tunnel and address. A different pinned key is an error.
func (a *App) comparePinnedHostKey(tunnelID, address, fingerprint string) (bool, error) {
	a.credentialsMu.Lock()
	defer a.credentialsMu.Unlock()
	pinned := a.prefs.Get().SSHHostKeys[tunnelID][address]
	switch pinned {
	case "":
		return false, nil
	case fingerprint:
		return true, nil
	}
	return false, fmt.Errorf("the host key changed for %s (expected %s, got %s), so the connection was stopped. If this change is expected, forget the old key in the tunnel's settings", address, pinned, fingerprint)
}

func (a *App) pinSSHHostKey(tunnelID, address, fingerprint string) error {
	a.credentialsMu.Lock()
	defer a.credentialsMu.Unlock()
	slog.Info("pinning SSH host key accepted by the user", "tunnel", tunnelID, "address", address, "sha256", fingerprint)
	p := a.prefs.Get()
	keys := make(map[string]string, len(p.SSHHostKeys[tunnelID])+1)
	for key, value := range p.SSHHostKeys[tunnelID] {
		keys[key] = value
	}
	keys[address] = fingerprint
	p.SSHHostKeys = withSSHHostKeys(p.SSHHostKeys, tunnelID, keys)
	return a.prefs.Set(p)
}

// askHostKey shows the question in the main window and waits for the answer.
// No answer means no.
func (a *App) askHostKey(tunnelID, address, keyType, fingerprint string) bool {
	name := tunnelID
	if cfg, ok := a.store.GetTunnel(tunnelID); ok {
		name = cfg.Name
	}
	pending := &pendingHostKey{answer: make(chan bool, 1)}

	a.hostKeyMu.Lock()
	if a.hostKeyClosed {
		a.hostKeyMu.Unlock()
		return false
	}
	a.hostKeyNextID++
	pending.prompt = HostKeyPrompt{ID: a.hostKeyNextID, TunnelName: name, Address: address, KeyType: keyType, Fingerprint: fingerprint}
	a.hostKeyPending = pending
	a.hostKeyMu.Unlock()

	defer func() {
		a.hostKeyMu.Lock()
		a.hostKeyPending = nil
		a.hostKeyMu.Unlock()
		a.emit("hostkey:resolved", pending.prompt.ID)
	}()

	a.emit("hostkey:request", pending.prompt)
	a.showWindow()

	timeout := time.NewTimer(hostKeyAnswerTimeout)
	defer timeout.Stop()
	select {
	case accepted := <-pending.answer:
		return accepted
	case <-timeout.C:
		slog.Warn("no answer about an unknown SSH host key", "tunnel", tunnelID, "address", address)
		return false
	}
}

// GetPendingHostKey returns the question waiting for an answer, or nil. The main
// window asks when it loads, because an auto-connect can start before it does.
func (a *App) GetPendingHostKey() *HostKeyPrompt {
	a.hostKeyMu.Lock()
	defer a.hostKeyMu.Unlock()
	if a.hostKeyPending == nil {
		return nil
	}
	prompt := a.hostKeyPending.prompt
	return &prompt
}

// AnswerHostKey records the user's decision about a host key question. An
// answer to a question that is no longer waiting is ignored.
func (a *App) AnswerHostKey(id uint64, accepted bool) {
	a.hostKeyMu.Lock()
	defer a.hostKeyMu.Unlock()
	if a.hostKeyPending != nil && a.hostKeyPending.prompt.ID == id {
		select {
		case a.hostKeyPending.answer <- accepted:
		default:
		}
	}
}

// closeHostKeyPrompts refuses the waiting question and any later one, so
// shutting down does not wait for an answer.
func (a *App) closeHostKeyPrompts() {
	a.hostKeyMu.Lock()
	defer a.hostKeyMu.Unlock()
	a.hostKeyClosed = true
	if a.hostKeyPending != nil {
		select {
		case a.hostKeyPending.answer <- false:
		default:
		}
	}
}

func knownHostsPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".ssh", "known_hosts")
}
