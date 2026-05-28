package ssh

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"

	"ssh-tunnel-manager/internal/config"
	"ssh-tunnel-manager/internal/dns"
)

const (
	keepAliveInterval = 30 * time.Second
	keepAliveTimeout  = 10 * time.Second
	// shutdownTimeout caps how long Tunnel.Connect waits for its port-forward
	// and keep-alive goroutines to finish after ctx is cancelled and the SSH
	// client has been closed. If we hit this, we log a warning and return so
	// the UI ("saving..." spinner during UpdateTunnel) doesn't lock up. Stray
	// goroutines wake up and clean themselves up shortly after.
	shutdownTimeout = 5 * time.Second
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
	LogFunc       func(level, msg string) // optional: called for connection events
	// DNSRegistry is consulted for portless forwards to allocate a loopback
	// IP and register the *.ssh-local domain. nil disables portless mode.
	DNSRegistry *dns.Registry

	mu     sync.Mutex
	client *ssh.Client
}

func (t *Tunnel) log(level, msg string) {
	if t.LogFunc != nil {
		t.LogFunc(level, msg)
	}
}

// CheckPortConflicts tests whether all local ports for this tunnel are
// available. Returns a descriptive error if any port is already in use.
// Portless forwards are skipped — they bind to a freshly allocated loopback
// IP that cannot collide with anything else on 127.0.0.1.
func (t *Tunnel) CheckPortConflicts() error {
	for _, pf := range t.Config.PortForwards {
		if pf.Portless {
			continue
		}
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

	t.log("info", "Connected")
	if t.OnConnected != nil {
		t.OnConnected()
	}

	defer func() {
		t.mu.Lock()
		t.client = nil
		t.mu.Unlock()
		client.Close()
		t.log("info", "Disconnected")
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
	// Close the underlying SSH transport so any in-flight SendRequest (in
	// keepAlive) and Accept/Read calls unblock immediately. Without this,
	// wg.Wait below can deadlock waiting for keepAlive to notice the ctx
	// cancellation.
	_ = client.Close()

	// Bounded wait — portless forwards in HTTP-proxy mode can occasionally
	// take a moment to drain in-flight requests, and on Windows Accept on
	// 127.0.1.x loopback IPs has been observed to stall briefly after the
	// listener is closed. Don't let that block the caller (UpdateTunnel save
	// → UI spinner) forever.
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(shutdownTimeout):
		slog.Warn("tunnel shutdown timed out; goroutines may finish later",
			"tunnel", t.Config.Name, "timeout", shutdownTimeout)
	}
	return ctx.Err()
}

func (t *Tunnel) buildSSHConfig() (*ssh.ClientConfig, error) {
	return buildClientConfig(t.Config, t.GetPassphrase)
}

// dial establishes an SSH client connection using the appropriate method:
// ProxyCommand, ProxyJump, or direct TCP dial.
func (t *Tunnel) dial(ctx context.Context, sshConfig *ssh.ClientConfig) (*ssh.Client, error) {
	addr := fmt.Sprintf("%s:%d", t.Config.Host, t.Config.Port)

	switch {
	case t.Config.ProxyCommand != "":
		t.log("info", fmt.Sprintf("Connecting to %s@%s:%d via ProxyCommand", t.Config.User, t.Config.Host, t.Config.Port))
		return t.dialViaProxyCommand(ctx, addr, sshConfig)
	case t.Config.ProxyJump != "":
		t.log("info", fmt.Sprintf("Connecting to %s@%s:%d via ProxyJump", t.Config.User, t.Config.Host, t.Config.Port))
		return t.dialViaProxyJump(ctx, addr, sshConfig)
	default:
		t.log("info", fmt.Sprintf("Connecting to %s@%s:%d", t.Config.User, t.Config.Host, t.Config.Port))
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

	cmd := proxyCommandExec(ctx, cmdStr)
	applySysProcAttr(cmd)

	// Ensure SSH_AUTH_SOCK is available for the subprocess.
	// On macOS, GUI apps may not inherit the shell environment.
	cmd.Env = proxyCommandEnv()

	// Capture stderr so we can surface proxy errors to the user
	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf

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
		proxyErr := strings.TrimSpace(stderrBuf.String())
		if proxyErr != "" {
			slog.Error("ProxyCommand stderr", "tunnel", t.Config.Name, "stderr", proxyErr)
			t.log("error", fmt.Sprintf("ProxyCommand stderr: %s", proxyErr))
			return nil, fmt.Errorf("SSH handshake via ProxyCommand: %w (proxy: %s)", err, proxyErr)
		}
		return nil, fmt.Errorf("SSH handshake via ProxyCommand: %w", err)
	}

	return ssh.NewClient(c, chans, reqs), nil
}

// proxyCommandEnv returns the environment for ProxyCommand subprocesses.
// It starts with the current process environment, then ensures SSH_AUTH_SOCK
// is set — on macOS GUI apps the shell environment may not be inherited.
func proxyCommandEnv() []string {
	env := os.Environ()

	// If SSH_AUTH_SOCK is already set, nothing to do
	if os.Getenv("SSH_AUTH_SOCK") != "" {
		return env
	}

	// On macOS, try to discover the launchd-managed agent socket
	if runtime.GOOS == "darwin" {
		if sock := findMacOSAgentSocket(); sock != "" {
			slog.Info("discovered macOS SSH agent socket", "path", sock)
			env = append(env, "SSH_AUTH_SOCK="+sock)
		}
	}

	return env
}

// findMacOSAgentSocket looks for the launchd SSH agent socket in common
// locations. Returns empty string if not found.
func findMacOSAgentSocket() string {
	// launchd sockets are typically in /private/tmp/com.apple.launchd.*/Listeners
	matches, err := filepath.Glob("/private/tmp/com.apple.launchd.*/Listeners")
	if err != nil || len(matches) == 0 {
		return ""
	}
	// Verify the socket is actually connectable
	for _, sock := range matches {
		conn, err := net.DialTimeout("unix", sock, 2*time.Second)
		if err == nil {
			conn.Close()
			return sock
		}
	}
	return ""
}

// proxyCommandExec builds an exec.Cmd for running a ProxyCommand string.
// It prefers sh -c (matching OpenSSH behaviour), falling back to cmd /c on
// Windows when no POSIX shell is available.
func proxyCommandExec(ctx context.Context, cmdStr string) *exec.Cmd {
	if runtime.GOOS != "windows" {
		return exec.CommandContext(ctx, "sh", "-c", cmdStr)
	}

	// On Windows, try to find sh (Git for Windows / MSYS2).
	if sh := findWindowsShell(); sh != "" {
		slog.Debug("using POSIX shell for ProxyCommand", "shell", sh)
		return exec.CommandContext(ctx, sh, "-c", cmdStr)
	}

	// Fallback: cmd /c. Works for simple commands that don't need POSIX
	// quoting. If the ProxyCommand itself invokes sh, the user needs Git
	// for Windows installed anyway.
	slog.Warn("sh not found, falling back to cmd /c for ProxyCommand; " +
		"install Git for Windows for full ProxyCommand support")
	return exec.CommandContext(ctx, "cmd", "/c", cmdStr)
}

// findWindowsShell searches for sh.exe in PATH and common Git for Windows
// locations. Returns empty string if not found.
func findWindowsShell() string {
	// Try PATH first
	if p, err := exec.LookPath("sh"); err == nil {
		return p
	}

	// Common Git for Windows / MSYS2 locations
	roots := []string{
		os.Getenv("ProgramFiles"),
		os.Getenv("ProgramFiles(x86)"),
		filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs"),
	}
	for _, root := range roots {
		if root == "" {
			continue
		}
		for _, rel := range []string{
			filepath.Join("Git", "bin", "sh.exe"),
			filepath.Join("Git", "usr", "bin", "sh.exe"),
		} {
			p := filepath.Join(root, rel)
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}

	return ""
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
	var listener net.Listener
	var localAddr string

	if pf.Portless {
		if t.DNSRegistry == nil {
			msg := fmt.Sprintf("Portless forward %q skipped: DNS registry unavailable", pf.Domain)
			t.log("error", msg)
			slog.Error("portless forward missing DNS registry", "tunnel", t.Config.Name, "domain", pf.Domain)
			return
		}
		// ExposePort > 0 means the user explicitly chose a different listen
		// port for this domain (e.g. 80 so a browser doesn't need ":8080").
		// Otherwise we mirror the remote port so the URL conveniently uses
		// the standard service port.
		exposePort := pf.RemotePort
		if pf.ExposePort > 0 {
			exposePort = pf.ExposePort
		}

		// Try to bind, retrying with a different loopback IP when the chosen
		// one is held by another process (zombie listener from a previous
		// run, WSL2 grabbing 127.0.1.1, etc.). Each failed IP is blocked for
		// the rest of this session so we don't loop on it.
		const maxAttempts = 10
		var entry dns.Entry
		var lastErr error
		for attempt := 0; attempt < maxAttempts; attempt++ {
			e, err := t.DNSRegistry.Allocate(pf.Domain, exposePort)
			if err != nil {
				t.log("error", fmt.Sprintf("Portless allocation failed for %q: %v", pf.Domain, err))
				slog.Error("portless allocation failed", "tunnel", t.Config.Name, "domain", pf.Domain, "error", err)
				return
			}
			addr := fmt.Sprintf("%s:%d", e.IP.String(), exposePort)
			ln, lnErr := net.Listen("tcp", addr)
			if lnErr == nil {
				entry = e
				listener = ln
				localAddr = addr
				break
			}
			lastErr = lnErr
			slog.Warn("portless bind retry", "tunnel", t.Config.Name, "addr", addr, "error", lnErr)
			t.log("warn", fmt.Sprintf("Portless: %s in use, trying another IP", addr))
			t.DNSRegistry.Block(e.IP)
			t.DNSRegistry.Release(pf.Domain)
		}
		if listener == nil {
			t.log("error", fmt.Sprintf("Portless bind failed after %d attempts: %v", maxAttempts, lastErr))
			slog.Error("portless bind exhausted retries", "tunnel", t.Config.Name, "domain", pf.Domain, "error", lastErr)
			return
		}
		defer t.DNSRegistry.Release(pf.Domain)
		t.log("info", fmt.Sprintf("Portless: %s.%s → %s", entry.Domain, dns.TLD, localAddr))
	} else {
		localAddr = fmt.Sprintf("127.0.0.1:%d", pf.LocalPort)
		ln, err := net.Listen("tcp", localAddr)
		if err != nil {
			slog.Error("failed to listen on local port",
				"tunnel", t.Config.Name, "addr", localAddr, "error", err)
			t.log("error", fmt.Sprintf("Local port %d in use: %v", pf.LocalPort, err))
			return
		}
		listener = ln
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

	// HTTP-aware path: rewrite the Host header on its way upstream so a
	// reverse proxy on the remote (e.g. Traefik in front of Portainer) routes
	// correctly even though the browser hit something.ssh-local. Only kicks
	// in for portless forwards — for plain 127.0.0.1:port tunnels the user
	// already controls what the browser sends.
	if header := effectiveHostHeader(pf); header != "" {
		t.serveHTTPProxy(ctx, client, listener, pf, header)
		return
	}

	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			errMsg := fmt.Sprintf("Port forward error: %v", err)
			t.log("error", errMsg)
			slog.Error("accept failed", "tunnel", t.Config.Name, "error", err)
			return
		}
		go t.handleForwardConn(ctx, client, conn, pf)
	}
}

// effectiveHostHeader returns the Host header value to send upstream, or ""
// to keep raw TCP forwarding. HostHeaderOff disables the rewrite entirely.
// Otherwise explicit HostHeader wins; failing that, portless + FQDN-looking
// RemoteHost (e.g. "dev.mix-dev.com") auto-uses the remote host so
// jumphost-routed services like Portainer "just work".
func effectiveHostHeader(pf config.PortForward) string {
	if pf.HostHeaderOff {
		return ""
	}
	if pf.HostHeader != "" {
		return pf.HostHeader
	}
	if !pf.Portless {
		return ""
	}
	if looksLikeFQDN(pf.RemoteHost) {
		return pf.RemoteHost
	}
	return ""
}

func looksLikeFQDN(h string) bool {
	if h == "" {
		return false
	}
	if net.ParseIP(h) != nil {
		return false
	}
	if !strings.Contains(h, ".") {
		return false
	}
	if strings.EqualFold(h, "localhost") {
		return false
	}
	return true
}

func (t *Tunnel) serveHTTPProxy(ctx context.Context, client *ssh.Client, listener net.Listener, pf config.PortForward, hostHeader string) {
	target := &url.URL{
		Scheme: "http",
		Host:   fmt.Sprintf("%s:%d", pf.RemoteHost, pf.RemotePort),
	}
	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = target.Scheme
			req.URL.Host = target.Host
			req.Host = hostHeader
		},
		Transport: &http.Transport{
			// ssh.Client.Dial has no ctx variant, so we race it against the
			// caller's ctx. On disconnect (Server.Close cancels in-flight req
			// contexts), an open SSH channel dial returns promptly instead of
			// holding up shutdown.
			DialContext: func(dialCtx context.Context, _, addr string) (net.Conn, error) {
				type result struct {
					c   net.Conn
					err error
				}
				ch := make(chan result, 1)
				go func() {
					c, err := client.Dial("tcp", addr)
					ch <- result{c, err}
				}()
				select {
				case r := <-ch:
					return r.c, r.err
				case <-dialCtx.Done():
					go func() {
						if r := <-ch; r.c != nil {
							r.c.Close()
						}
					}()
					return nil, dialCtx.Err()
				}
			},
			DisableKeepAlives: true,
		},
		ErrorLog: slog.NewLogLogger(slog.Default().Handler(), slog.LevelWarn),
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, err error) {
			slog.Warn("http proxy error", "tunnel", t.Config.Name, "error", err)
			http.Error(w, "ssh-tunnel-manager: upstream error: "+err.Error(), http.StatusBadGateway)
		},
	}
	srv := &http.Server{Handler: proxy}
	go func() {
		<-ctx.Done()
		_ = srv.Close()
	}()
	t.log("info", fmt.Sprintf("HTTP proxy: %s → %s (Host: %s)", listener.Addr().String(), target.Host, hostHeader))
	if err := srv.Serve(listener); err != nil && err != http.ErrServerClosed && ctx.Err() == nil {
		t.log("error", fmt.Sprintf("HTTP proxy error: %v", err))
		slog.Error("http proxy serve failed", "tunnel", t.Config.Name, "error", err)
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
				// Intentional disconnect closes the client; the resulting
				// SendRequest error is expected, not worth logging.
				if ctx.Err() != nil {
					return
				}
				t.log("warn", fmt.Sprintf("Keep-alive failed: %v", err))
				slog.Warn("keep-alive failed", "tunnel", t.Config.Name, "error", err)
				return
			}
		}
	}
}
