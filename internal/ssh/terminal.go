package ssh

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"

	"ssh-tunnel-manager/internal/config"
)

// TerminalSession represents an interactive SSH shell session.
type TerminalSession struct {
	SessionID string
	client    *ssh.Client
	session   *ssh.Session
	stdin     io.WriteCloser

	mu     sync.Mutex
	closed bool
}

// Write sends data (user keystrokes) to the remote shell.
func (ts *TerminalSession) Write(data string) error {
	ts.mu.Lock()
	if ts.closed {
		ts.mu.Unlock()
		return fmt.Errorf("session %s is closed", ts.SessionID)
	}
	ts.mu.Unlock()
	_, err := ts.stdin.Write([]byte(data))
	return err
}

// Resize changes the PTY window size.
func (ts *TerminalSession) Resize(cols, rows int) error {
	ts.mu.Lock()
	if ts.closed {
		ts.mu.Unlock()
		return fmt.Errorf("session %s is closed", ts.SessionID)
	}
	ts.mu.Unlock()
	return ts.session.WindowChange(rows, cols)
}

// Close tears down the session and SSH connection.
func (ts *TerminalSession) Close() {
	ts.mu.Lock()
	if ts.closed {
		ts.mu.Unlock()
		return
	}
	ts.closed = true
	ts.mu.Unlock()

	ts.session.Close()
	ts.client.Close()
}

// TerminalManager manages interactive SSH terminal sessions.
type TerminalManager struct {
	mu            sync.Mutex
	sessions      map[string]*TerminalSession
	nextID        int
	getPassphrase PassphraseFunc
}

// NewTerminalManager creates a new TerminalManager.
func NewTerminalManager(getPassphrase PassphraseFunc) *TerminalManager {
	return &TerminalManager{
		sessions:      make(map[string]*TerminalSession),
		getPassphrase: getPassphrase,
	}
}

// OpenSession establishes a new SSH connection and opens an interactive PTY shell.
// emitFn is called with output data; closedFn is called when the session ends.
func (tm *TerminalManager) OpenSession(cfg config.TunnelConfig, emitFn func(sessionID, data string), closedFn func(sessionID string)) (string, error) {
	return tm.openSession(cfg, "", emitFn, closedFn)
}

// OpenCommandSession is like OpenSession but runs command in the PTY instead of
// a login shell — used for full-screen TUI tools such as htop.
func (tm *TerminalManager) OpenCommandSession(cfg config.TunnelConfig, command string, emitFn func(sessionID, data string), closedFn func(sessionID string)) (string, error) {
	return tm.openSession(cfg, command, emitFn, closedFn)
}

// openSession opens an interactive PTY session. If command is empty it starts a
// login shell; otherwise it runs command in the PTY.
func (tm *TerminalManager) openSession(cfg config.TunnelConfig, command string, emitFn func(sessionID, data string), closedFn func(sessionID string)) (string, error) {
	// Build SSH client config (reuse same logic as Tunnel)
	sshConfig, err := buildClientConfig(cfg, tm.getPassphrase)
	if err != nil {
		return "", fmt.Errorf("building SSH config for terminal: %w", err)
	}

	// Dial SSH connection (own connection, separate from tunnel)
	tun := &Tunnel{Config: cfg, GetPassphrase: tm.getPassphrase}
	client, err := tun.dial(context.Background(), sshConfig)
	if err != nil {
		return "", fmt.Errorf("connecting terminal to %s: %w", cfg.Host, err)
	}

	session, err := client.NewSession()
	if err != nil {
		client.Close()
		return "", fmt.Errorf("opening SSH session: %w", err)
	}

	// Request PTY
	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}
	if err := session.RequestPty("xterm-256color", 24, 80, modes); err != nil {
		session.Close()
		client.Close()
		return "", fmt.Errorf("requesting PTY: %w", err)
	}

	stdinPipe, err := session.StdinPipe()
	if err != nil {
		session.Close()
		client.Close()
		return "", fmt.Errorf("creating stdin pipe: %w", err)
	}

	stdoutPipe, err := session.StdoutPipe()
	if err != nil {
		session.Close()
		client.Close()
		return "", fmt.Errorf("creating stdout pipe: %w", err)
	}

	stderrPipe, err := session.StderrPipe()
	if err != nil {
		session.Close()
		client.Close()
		return "", fmt.Errorf("creating stderr pipe: %w", err)
	}

	if command == "" {
		if err := session.Shell(); err != nil {
			session.Close()
			client.Close()
			return "", fmt.Errorf("starting shell: %w", err)
		}
	} else {
		if err := session.Start(command); err != nil {
			session.Close()
			client.Close()
			return "", fmt.Errorf("starting command %q: %w", command, err)
		}
	}

	tm.mu.Lock()
	tm.nextID++
	sessionID := fmt.Sprintf("term-%d", tm.nextID)
	ts := &TerminalSession{
		SessionID: sessionID,
		client:    client,
		session:   session,
		stdin:     stdinPipe,
	}
	tm.sessions[sessionID] = ts
	tm.mu.Unlock()

	slog.Info("terminal session opened", "sessionId", sessionID, "host", cfg.Host)

	// Read stdout in background
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := stdoutPipe.Read(buf)
			if n > 0 {
				emitFn(sessionID, string(buf[:n]))
			}
			if err != nil {
				break
			}
		}
		tm.cleanupSession(sessionID)
		closedFn(sessionID)
	}()

	// Read stderr in background
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := stderrPipe.Read(buf)
			if n > 0 {
				emitFn(sessionID, string(buf[:n]))
			}
			if err != nil {
				return
			}
		}
	}()

	return sessionID, nil
}

// GetSession returns a session by ID.
func (tm *TerminalManager) GetSession(sessionID string) (*TerminalSession, bool) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	ts, ok := tm.sessions[sessionID]
	return ts, ok
}

// CloseSession closes and removes a terminal session.
func (tm *TerminalManager) CloseSession(sessionID string) {
	tm.mu.Lock()
	ts, ok := tm.sessions[sessionID]
	if ok {
		delete(tm.sessions, sessionID)
	}
	tm.mu.Unlock()
	if ok {
		ts.Close()
		slog.Info("terminal session closed", "sessionId", sessionID)
	}
}

// CloseAll closes all terminal sessions.
func (tm *TerminalManager) CloseAll() {
	tm.mu.Lock()
	sessions := make([]*TerminalSession, 0, len(tm.sessions))
	for _, ts := range tm.sessions {
		sessions = append(sessions, ts)
	}
	tm.sessions = make(map[string]*TerminalSession)
	tm.mu.Unlock()

	for _, ts := range sessions {
		ts.Close()
	}
}

func (tm *TerminalManager) cleanupSession(sessionID string) {
	tm.mu.Lock()
	ts, ok := tm.sessions[sessionID]
	if ok {
		delete(tm.sessions, sessionID)
	}
	tm.mu.Unlock()
	if ok {
		ts.Close()
	}
}

// buildClientConfig creates an ssh.ClientConfig from a TunnelConfig.
// This is the shared helper used by both Tunnel and TerminalManager.
//
// Authentication methods are tried in order:
//  1. Explicit key from KeyPath (if set)
//  2. SSH agent (ssh-agent on Unix, OpenSSH agent on Windows)
//  3. Default key files (~/.ssh/id_ed25519, id_rsa, etc.) when KeyPath is empty
func buildClientConfig(cfg config.TunnelConfig, getPassphrase PassphraseFunc) (*ssh.ClientConfig, error) {
	var authMethods []ssh.AuthMethod

	// 1. Explicit key file
	if cfg.KeyPath != "" {
		signer, err := loadSigner(cfg.KeyPath, getPassphrase)
		if err != nil {
			return nil, err
		}
		authMethods = append(authMethods, ssh.PublicKeys(signer))
	}

	// 2. SSH agent
	if agentAuth, err := sshAgentAuth(); err == nil {
		authMethods = append(authMethods, agentAuth)
	} else {
		slog.Debug("SSH agent not available", "error", err)
	}

	// 3. Default key files (only when no explicit key was given)
	if cfg.KeyPath == "" {
		home, _ := os.UserHomeDir()
		if home != "" {
			defaults := []string{
				filepath.Join(home, ".ssh", "id_ed25519"),
				filepath.Join(home, ".ssh", "id_ecdsa"),
				filepath.Join(home, ".ssh", "id_rsa"),
			}
			for _, p := range defaults {
				signer, err := loadSigner(p, getPassphrase)
				if err != nil {
					continue // skip missing or unreadable keys
				}
				authMethods = append(authMethods, ssh.PublicKeys(signer))
			}
		}
	}

	if len(authMethods) == 0 {
		return nil, fmt.Errorf("no authentication methods available for %s (no key, no agent)", cfg.Host)
	}

	return &ssh.ClientConfig{
		User:            cfg.User,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}, nil
}

// loadSigner reads a private key file and returns an ssh.Signer.
// If the key is encrypted, it calls getPassphrase for the passphrase.
func loadSigner(keyPath string, getPassphrase PassphraseFunc) (ssh.Signer, error) {
	keyData, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("reading key %s: %w", keyPath, err)
	}

	signer, err := ssh.ParsePrivateKey(keyData)
	if err != nil {
		if _, ok := err.(*ssh.PassphraseMissingError); ok {
			if getPassphrase == nil {
				return nil, fmt.Errorf("key %s is encrypted and no passphrase provider available", keyPath)
			}
			passphrase, pErr := getPassphrase(keyPath)
			if pErr != nil {
				return nil, fmt.Errorf("getting passphrase for %s: %w", keyPath, pErr)
			}
			signer, err = ssh.ParsePrivateKeyWithPassphrase(keyData, []byte(passphrase))
			if err != nil {
				return nil, fmt.Errorf("decrypting key %s: %w", keyPath, err)
			}
		} else {
			return nil, fmt.Errorf("parsing key %s: %w", keyPath, err)
		}
	}
	return signer, nil
}

// sshAgentAuth connects to the running SSH agent and returns an AuthMethod.
func sshAgentAuth() (ssh.AuthMethod, error) {
	conn, err := dialAgent()
	if err != nil {
		return nil, err
	}
	return ssh.PublicKeysCallback(agent.NewClient(conn).Signers), nil
}

// dialAgent connects to the platform-specific SSH agent socket.
func dialAgent() (net.Conn, error) {
	if runtime.GOOS == "windows" {
		return net.Dial("pipe", `\\.\pipe\openssh-ssh-agent`)
	}
	sock := os.Getenv("SSH_AUTH_SOCK")
	if sock == "" && runtime.GOOS == "darwin" {
		// macOS GUI apps may not inherit SSH_AUTH_SOCK from the shell.
		// Try the launchd-managed agent socket.
		sock = findMacOSAgentSocket()
	}
	if sock == "" {
		return nil, fmt.Errorf("SSH_AUTH_SOCK not set")
	}
	return net.Dial("unix", sock)
}
