package ssh

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"

	"ssh-tunnel-manager/internal/config"
)

const (
	keepAliveInterval = 30 * time.Second
	keepAliveTimeout  = 10 * time.Second
)

// PassphraseFunc is called when an encrypted key needs a passphrase.
// keyPath is the path to the encrypted key file.
// Returns the passphrase or an error if the user cancelled.
type PassphraseFunc func(keyPath string) (string, error)

// Tunnel manages a single SSH connection and its port forwards.
type Tunnel struct {
	Config        config.TunnelConfig
	OnConnected   func() // called after SSH dial succeeds, before blocking
	GetPassphrase PassphraseFunc

	mu     sync.Mutex
	client *ssh.Client
}

// CheckPortConflicts tests whether all local ports for this tunnel are
// available. Returns a descriptive error if any port is already in use.
func (t *Tunnel) CheckPortConflicts() error {
	for _, pf := range t.Config.PortForwards {
		addr := fmt.Sprintf("127.0.0.1:%d", pf.LocalPort)
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			return fmt.Errorf("port %d is already in use", pf.LocalPort)
		}
		ln.Close()
	}
	return nil
}

// Connect establishes the SSH connection, starts port forwarding listeners,
// and runs keep-alive until ctx is cancelled. It blocks until shutdown.
func (t *Tunnel) Connect(ctx context.Context) error {
	if err := t.CheckPortConflicts(); err != nil {
		return fmt.Errorf("port conflict for %s: %w", t.Config.Name, err)
	}

	sshConfig, err := t.buildSSHConfig()
	if err != nil {
		return fmt.Errorf("building SSH config for %s: %w", t.Config.Name, err)
	}

	client, err := t.dial(ctx, sshConfig)
	if err != nil {
		return err
	}
	t.mu.Lock()
	t.client = client
	t.mu.Unlock()

	if t.OnConnected != nil {
		t.OnConnected()
	}

	defer func() {
		t.mu.Lock()
		t.client = nil
		t.mu.Unlock()
		client.Close()
		slog.Info("SSH connection closed", "tunnel", t.Config.Name)
	}()

	var wg sync.WaitGroup

	// Start port forward listeners
	for _, pf := range t.Config.PortForwards {
		wg.Add(1)
		go func(pf config.PortForward) {
			defer wg.Done()
			t.forwardPort(ctx, client, pf)
		}(pf)
	}

	// Keep-alive loop
	wg.Add(1)
	go func() {
		defer wg.Done()
		t.keepAlive(ctx, client)
	}()

	// Wait for context cancellation or connection failure
	<-ctx.Done()
	wg.Wait()
	return ctx.Err()
}

func (t *Tunnel) buildSSHConfig() (*ssh.ClientConfig, error) {
	keyData, err := os.ReadFile(t.Config.KeyPath)
	if err != nil {
		return nil, fmt.Errorf("reading key %s: %w", t.Config.KeyPath, err)
	}

	signer, err := ssh.ParsePrivateKey(keyData)
	if err != nil {
		if _, ok := err.(*ssh.PassphraseMissingError); ok {
			signer, err = t.parseEncryptedKey(keyData)
			if err != nil {
				return nil, err
			}
		} else {
			return nil, fmt.Errorf("parsing key %s: %w", t.Config.KeyPath, err)
		}
	}

	return &ssh.ClientConfig{
		User: t.Config.User,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}, nil
}

func (t *Tunnel) parseEncryptedKey(keyData []byte) (ssh.Signer, error) {
	if t.GetPassphrase == nil {
		return nil, fmt.Errorf("key %s is encrypted and no passphrase provider available", t.Config.KeyPath)
	}
	passphrase, err := t.GetPassphrase(t.Config.KeyPath)
	if err != nil {
		return nil, fmt.Errorf("getting passphrase for %s: %w", t.Config.KeyPath, err)
	}
	signer, err := ssh.ParsePrivateKeyWithPassphrase(keyData, []byte(passphrase))
	if err != nil {
		return nil, fmt.Errorf("decrypting key %s: %w", t.Config.KeyPath, err)
	}
	return signer, nil
}

// dial establishes an SSH client connection using the appropriate method:
// ProxyCommand, ProxyJump, or direct TCP dial.
func (t *Tunnel) dial(ctx context.Context, sshConfig *ssh.ClientConfig) (*ssh.Client, error) {
	addr := fmt.Sprintf("%s:%d", t.Config.Host, t.Config.Port)

	switch {
	case t.Config.ProxyCommand != "":
		return t.dialViaProxyCommand(ctx, addr, sshConfig)
	case t.Config.ProxyJump != "":
		return t.dialViaProxyJump(ctx, addr, sshConfig)
	default:
		slog.Info("connecting to SSH server", "tunnel", t.Config.Name, "addr", addr)
		client, err := ssh.Dial("tcp", addr, sshConfig)
		if err != nil {
			return nil, fmt.Errorf("dialing %s: %w", addr, err)
		}
		return client, nil
	}
}

func (t *Tunnel) dialViaProxyCommand(ctx context.Context, addr string, sshConfig *ssh.ClientConfig) (*ssh.Client, error) {
	cmdStr := expandProxyCommand(t.Config.ProxyCommand, t.Config.Host, t.Config.Port, t.Config.User)
	slog.Info("connecting via ProxyCommand", "tunnel", t.Config.Name, "command", cmdStr)

	cmd := exec.CommandContext(ctx, "sh", "-c", cmdStr)
	applySysProcAttr(cmd)
	cmd.Stderr = os.Stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("creating stdin pipe for ProxyCommand: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("creating stdout pipe for ProxyCommand: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting ProxyCommand %q: %w", cmdStr, err)
	}

	conn := &proxyConn{cmd: cmd, stdin: stdin, stdout: stdout}

	c, chans, reqs, err := ssh.NewClientConn(conn, addr, sshConfig)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("SSH handshake via ProxyCommand: %w", err)
	}

	return ssh.NewClient(c, chans, reqs), nil
}

func (t *Tunnel) dialViaProxyJump(ctx context.Context, addr string, sshConfig *ssh.ClientConfig) (*ssh.Client, error) {
	jumpHost := t.Config.ProxyJump
	// Default to port 22 if no port specified
	if !strings.Contains(jumpHost, ":") {
		jumpHost = jumpHost + ":22"
	}
	slog.Info("connecting via ProxyJump", "tunnel", t.Config.Name, "jump", jumpHost, "target", addr)

	jumpClient, err := ssh.Dial("tcp", jumpHost, sshConfig)
	if err != nil {
		return nil, fmt.Errorf("dialing jump host %s: %w", jumpHost, err)
	}

	conn, err := jumpClient.Dial("tcp", addr)
	if err != nil {
		jumpClient.Close()
		return nil, fmt.Errorf("dialing %s via jump host: %w", addr, err)
	}

	c, chans, reqs, err := ssh.NewClientConn(conn, addr, sshConfig)
	if err != nil {
		conn.Close()
		jumpClient.Close()
		return nil, fmt.Errorf("SSH handshake via ProxyJump: %w", err)
	}

	client := ssh.NewClient(c, chans, reqs)

	// Close jump client when the tunneled client connection closes
	go func() {
		client.Wait()
		jumpClient.Close()
	}()

	return client, nil
}

// expandProxyCommand substitutes SSH-style tokens in a ProxyCommand string:
// %h → hostname, %p → port, %r → user, %% → literal %.
func expandProxyCommand(cmd string, host string, port int, user string) string {
	portStr := fmt.Sprintf("%d", port)
	var b strings.Builder
	b.Grow(len(cmd))
	for i := 0; i < len(cmd); i++ {
		if cmd[i] == '%' && i+1 < len(cmd) {
			switch cmd[i+1] {
			case 'h':
				b.WriteString(host)
				i++
				continue
			case 'p':
				b.WriteString(portStr)
				i++
				continue
			case 'r':
				b.WriteString(user)
				i++
				continue
			case '%':
				b.WriteByte('%')
				i++
				continue
			}
		}
		b.WriteByte(cmd[i])
	}
	return b.String()
}

func (t *Tunnel) forwardPort(ctx context.Context, client *ssh.Client, pf config.PortForward) {
	localAddr := fmt.Sprintf("127.0.0.1:%d", pf.LocalPort)
	listener, err := net.Listen("tcp", localAddr)
	if err != nil {
		slog.Error("failed to listen on local port",
			"tunnel", t.Config.Name, "addr", localAddr, "error", err)
		return
	}
	defer listener.Close()

	slog.Info("port forward listening",
		"tunnel", t.Config.Name,
		"local", localAddr,
		"remote", fmt.Sprintf("%s:%d", pf.RemoteHost, pf.RemotePort))

	// Close listener when context is cancelled
	go func() {
		<-ctx.Done()
		listener.Close()
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			slog.Error("accept failed", "tunnel", t.Config.Name, "error", err)
			return
		}
		go t.handleForwardConn(ctx, client, conn, pf)
	}
}

func (t *Tunnel) handleForwardConn(ctx context.Context, client *ssh.Client, local net.Conn, pf config.PortForward) {
	defer local.Close()

	remoteAddr := fmt.Sprintf("%s:%d", pf.RemoteHost, pf.RemotePort)
	remote, err := client.Dial("tcp", remoteAddr)
	if err != nil {
		slog.Error("failed to dial remote",
			"tunnel", t.Config.Name, "remote", remoteAddr, "error", err)
		return
	}
	defer remote.Close()

	var wg sync.WaitGroup
	wg.Add(2)

	// Copy in both directions
	copyDone := func(dst io.Writer, src io.Reader) {
		defer wg.Done()
		_, _ = io.Copy(dst, src)
	}
	go copyDone(remote, local)
	go copyDone(local, remote)

	// Close both when context cancelled
	go func() {
		<-ctx.Done()
		local.Close()
		remote.Close()
	}()

	wg.Wait()
}

func (t *Tunnel) keepAlive(ctx context.Context, client *ssh.Client) {
	ticker := time.NewTicker(keepAliveInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_, _, err := client.SendRequest("keepalive@openssh.com", true, nil)
			if err != nil {
				slog.Warn("keep-alive failed", "tunnel", t.Config.Name, "error", err)
				return
			}
		}
	}
}
