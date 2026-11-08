package ssh

import (
	"bytes"
	"context"
	"errors"
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
	"syscall"
	"time"

	"golang.org/x/crypto/ssh"

	"ssh-tunnel-manager/internal/config"
	"ssh-tunnel-manager/internal/dns"
	"ssh-tunnel-manager/internal/netcap"
	"ssh-tunnel-manager/internal/portless"
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
	// viaHandshakeTimeout bounds an SSH handshake carried over a tunnel when the
	// client config sets no timeout of its own.
	viaHandshakeTimeout = 15 * time.Second
)

// PassphraseFunc is called when an encrypted key needs a passphrase.
// keyPath is the path to the encrypted key file.
// Returns the passphrase or an error if the user cancelled.
type PassphraseFunc func(keyPath string) (string, error)

// PasswordLogin is the saved password login assigned to a tunnel.
type PasswordLogin struct {
	// Username replaces the tunnel's user when it is not empty.
	Username string
	Password string
}

// LoginFunc returns the password login of a tunnel, or nil when it has none.
type LoginFunc func(tunnelID string) (*PasswordLogin, error)

// Tunnel manages a single SSH connection and its port forwards.
type Tunnel struct {
	Config        config.TunnelConfig
	OnConnected   func() // called after SSH dial succeeds, before blocking
	GetPassphrase PassphraseFunc
	// GetLogin returns the tunnel's password login, if it has one.
	GetLogin LoginFunc
	// CheckHostKey decides whether to trust each server on the way. Required.
	CheckHostKey HostKeyFunc
	LogFunc      func(level, msg string) // optional: called for connection events
	// OnBindError is called when a portless forward cannot bind its listener.
	// The App layer surfaces it to the user (banner + tray) — most importantly
	// the Linux privileged-port case that needs CAP_NET_BIND_SERVICE.
	OnBindError func(PortlessBindError)
	// OnForwardError is called when the SSH server accepts the connection but
	// refuses a direct-tcpip channel used by a configured port forward.
	OnForwardError func(PortForwardError)
	// DNSRegistry is consulted for portless forwards to allocate a loopback
	// IP and register the *.ssh-local domain. nil disables portless mode.
	DNSRegistry dns.ForwardRegistry

	// Via, when set, carries the SSH connection over an existing tunnel instead
	// of dialing directly. ProxyCommand and ProxyJump are then ignored.
	Via RemoteDialer

	mu     sync.Mutex
	client *ssh.Client
}

// RemoteDialer opens a TCP connection to addr as seen from another SSH server.
type RemoteDialer func(ctx context.Context, addr string) (net.Conn, error)

func (t *Tunnel) log(level, msg string) {
	if t.LogFunc != nil {
		t.LogFunc(level, msg)
	}
}

// CheckPortConflicts tests whether all local ports for this tunnel are
// available. Returns a descriptive error if any port is already in use.
// Active Portless forwards are skipped — they bind to a freshly allocated
// loopback IP that cannot collide with anything else on 127.0.0.1. When the
// DNS registry is unavailable, Portless forwards degrade to their ordinary
// 127.0.0.1 local ports and must be checked like standard forwards.
func (t *Tunnel) CheckPortConflicts() error {
	for _, pf := range t.Config.PortForwards {
		if pf.Portless && t.DNSRegistry != nil {
			continue
		}
		localPort := pf.LocalPort
		if pf.Portless {
			localPort = fallbackLocalPort(pf)
		}
		addr := fmt.Sprintf("127.0.0.1:%d", localPort)
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			return fmt.Errorf("port %d is already in use", localPort)
		}
		ln.Close()
	}
	return nil
}

func fallbackLocalPort(pf config.PortForward) int {
	if pf.LocalPort > 0 {
		return pf.LocalPort
	}
	return pf.RemotePort
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

	// The connection's own context also ends when the server closes it, so the
	// tunnel reconnects at once instead of waiting for the next keep-alive, and
	// sessions do not keep failing on a dead client in between.
	parent := ctx
	ctx, cancelConnection := context.WithCancel(parent)
	defer cancelConnection()
	go func() {
		_ = client.Wait()
		cancelConnection()
	}()

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

	// Wait for cancellation by the caller or for the connection to end
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
	return parent.Err()
}

func (t *Tunnel) buildSSHConfig() (*ssh.ClientConfig, error) {
	return buildClientConfig(t.Config, t.GetPassphrase, t.GetLogin, t.CheckHostKey)
}

// runCommandTimeout is a fallback cap for RunCommand callers that pass a
// context without a deadline, so a hung remote command can't block forever.
// Callers that need a specific budget (e.g. slow docker commands) set their own
// deadline on the context, which takes precedence over this fallback.
const runCommandTimeout = 8 * time.Second

// RunCommand opens a new session on the tunnel's live SSH client, runs cmd,
// and returns its combined stdout+stderr. It returns an error if the tunnel is
// not currently connected. The command is bounded by runCommandTimeout.
func (t *Tunnel) RunCommand(ctx context.Context, cmd string) (string, error) {
	t.mu.Lock()
	client := t.client
	t.mu.Unlock()
	if client == nil {
		return "", errors.New("tunnel is not connected")
	}

	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("opening session: %w", err)
	}
	defer session.Close()

	type result struct {
		out []byte
		err error
	}
	ch := make(chan result, 1)
	start := time.Now()
	go func() {
		out, err := session.CombinedOutput(cmd)
		ch <- result{out, err}
	}()

	// Honor the caller's context deadline; only fall back to runCommandTimeout
	// when the context carries none (a nil channel blocks forever in select, so
	// the fallback case is inert when a deadline is present).
	var fallback <-chan time.Time
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		fallback = time.After(runCommandTimeout)
	}

	select {
	case r := <-ch:
		if r.err != nil {
			return string(r.out), fmt.Errorf("running %q: %w", cmd, r.err)
		}
		return string(r.out), nil
	case <-ctx.Done():
		_ = session.Close()
		return "", fmt.Errorf("running %q: %w (after %s)", cmd, ctx.Err(), time.Since(start).Round(time.Second))
	case <-fallback:
		_ = session.Close()
		return "", fmt.Errorf("running %q: timed out after %s", cmd, runCommandTimeout)
	}
}

// DialRemote opens a TCP connection to addr as seen from the SSH server. The
// returned connection does not support deadlines.
func (t *Tunnel) DialRemote(ctx context.Context, addr string) (net.Conn, error) {
	t.mu.Lock()
	client := t.client
	t.mu.Unlock()
	if client == nil {
		return nil, errors.New("tunnel is not connected")
	}

	type result struct {
		conn net.Conn
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		conn, err := client.Dial("tcp", addr)
		ch <- result{conn, err}
	}()
	select {
	case r := <-ch:
		if r.err != nil {
			return nil, fmt.Errorf("dialing %s through SSH: %w", addr, r.err)
		}
		return r.conn, nil
	case <-ctx.Done():
		go func() {
			if r := <-ch; r.conn != nil {
				_ = r.conn.Close()
			}
		}()
		return nil, fmt.Errorf("dialing %s through SSH: %w", addr, ctx.Err())
	}
}

// dialVia runs the SSH handshake over a connection opened through a tunnel. The
// connection does not support deadlines, so a timeout closes it instead.
func (t *Tunnel) dialVia(ctx context.Context, addr string, sshConfig *ssh.ClientConfig) (*ssh.Client, error) {
	timeout := sshConfig.Timeout
	if timeout <= 0 {
		timeout = viaHandshakeTimeout
	}
	dialCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	conn, err := t.Via(dialCtx, addr)
	if err != nil {
		return nil, err
	}
	timer := time.AfterFunc(timeout, func() { _ = conn.Close() })
	defer timer.Stop()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()

	// The host key check may wait for the user, so its time does not count.
	handshakeConfig := *sshConfig
	check := sshConfig.HostKeyCallback
	handshakeConfig.HostKeyCallback = func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		timer.Stop()
		defer timer.Reset(timeout)
		return check(hostname, remote, key)
	}

	c, chans, reqs, err := ssh.NewClientConn(conn, addr, &handshakeConfig)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("SSH handshake with %s through tunnel: %w", addr, err)
	}
	return ssh.NewClient(c, chans, reqs), nil
}

// IsRemoteDialRefused reports whether DialRemote failed because the SSH server
// answered that it could not open the connection, as opposed to a problem with
// the SSH connection itself.
func IsRemoteDialRefused(err error) bool {
	var openErr *ssh.OpenChannelError
	return errors.As(err, &openErr)
}

// dial establishes an SSH client connection using the appropriate method:
// ProxyCommand, ProxyJump, or direct TCP dial.
func (t *Tunnel) dial(ctx context.Context, sshConfig *ssh.ClientConfig) (*ssh.Client, error) {
	addr := fmt.Sprintf("%s:%d", t.Config.Host, t.Config.Port)

	switch {
	case t.Via != nil:
		t.log("info", fmt.Sprintf("Connecting to %s@%s:%d through a tunnel", t.Config.User, t.Config.Host, t.Config.Port))
		return t.dialVia(ctx, addr, sshConfig)
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
	proxyEnv := proxyCommandEnv()
	effectivePath := environmentValue(proxyEnv, "PATH")
	slog.Info("connecting via ProxyCommand", "tunnel", t.Config.Name, "command", cmdStr, "path", effectivePath)

	cmd := proxyCommandExec(ctx, cmdStr)
	applySysProcAttr(cmd)

	// Ensure PATH and SSH_AUTH_SOCK are usable from GUI launches, where macOS
	// provides a much smaller environment than an interactive shell.
	cmd.Env = proxyEnv

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
			proxyMessage := proxyCommandFailureMessage(proxyErr, effectivePath)
			t.log("error", proxyMessage)
			return nil, fmt.Errorf("SSH handshake via ProxyCommand: %w (%s)", err, proxyMessage)
		}
		return nil, fmt.Errorf("SSH handshake via ProxyCommand: %w", err)
	}

	return ssh.NewClient(c, chans, reqs), nil
}

// proxyCommandEnv returns the environment for ProxyCommand subprocesses. It
// starts with the current process environment, then fills in macOS GUI-launch
// gaps: common package-manager paths and the launchd-managed SSH agent socket.
func proxyCommandEnv() []string {
	env := os.Environ()

	if runtime.GOOS == "darwin" {
		home, err := os.UserHomeDir()
		if err != nil {
			slog.Debug("could not resolve home directory for ProxyCommand PATH", "error", err)
		}
		env = appendPathEntries(env, macOSProxyCommandPathEntries(home)...)

		if environmentValue(env, "SSH_AUTH_SOCK") == "" {
			if sock := findMacOSAgentSocket(); sock != "" {
				slog.Info("discovered macOS SSH agent socket", "path", sock)
				env = append(env, "SSH_AUTH_SOCK="+sock)
			}
		}
	}

	return env
}

func macOSProxyCommandPathEntries(home string) []string {
	entries := []string{
		"/opt/homebrew/bin",
		"/usr/local/bin",
		"/opt/local/bin",
	}
	if home != "" {
		entries = append(entries, filepath.Join(home, ".local", "bin"))
	}
	return entries
}

// appendPathEntries adds directories to PATH without reordering or duplicating
// existing entries. Updating the environment slice directly keeps paths that
// contain spaces intact; no shell expansion or word splitting is involved.
func appendPathEntries(env []string, additions ...string) []string {
	pathEntries := filepath.SplitList(environmentValue(env, "PATH"))
	seen := make(map[string]struct{}, len(pathEntries)+len(additions))
	for _, entry := range pathEntries {
		seen[entry] = struct{}{}
	}
	for _, entry := range additions {
		if entry == "" {
			continue
		}
		if _, exists := seen[entry]; exists {
			continue
		}
		pathEntries = append(pathEntries, entry)
		seen[entry] = struct{}{}
	}

	return setEnvironmentValue(env, "PATH", strings.Join(pathEntries, string(os.PathListSeparator)))
}

func environmentValue(env []string, key string) string {
	prefix := key + "="
	for i := len(env) - 1; i >= 0; i-- {
		if strings.HasPrefix(env[i], prefix) {
			return strings.TrimPrefix(env[i], prefix)
		}
	}
	return ""
}

func setEnvironmentValue(env []string, key, value string) []string {
	prefix := key + "="
	result := make([]string, 0, len(env)+1)
	replaced := false
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			if !replaced {
				result = append(result, prefix+value)
				replaced = true
			}
			continue
		}
		result = append(result, entry)
	}
	if !replaced {
		result = append(result, prefix+value)
	}
	return result
}

func proxyCommandFailureMessage(proxyErr, effectivePath string) string {
	if isProxyCommandHelperNotFound(proxyErr) {
		return fmt.Sprintf(
			"ProxyCommand helper was not found. Effective PATH: %s. Install the helper in a PATH directory or use an absolute path. Proxy stderr: %s",
			effectivePath, proxyErr,
		)
	}
	return fmt.Sprintf("ProxyCommand stderr: %s", proxyErr)
}

func isProxyCommandHelperNotFound(proxyErr string) bool {
	message := strings.ToLower(proxyErr)
	return strings.Contains(message, "executable file not found in $path") ||
		strings.Contains(message, "command not found") ||
		strings.Contains(message, "sessionmanagerplugin is not found") ||
		(strings.HasPrefix(message, "sh:") && strings.Contains(message, ": not found"))
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

// reportBindError logs a portless bind failure and notifies the App layer.
// The Linux privileged-port case (EACCES on a port below
// net.ipv4.ip_unprivileged_port_start) gets a specific, actionable message
// naming CAP_NET_BIND_SERVICE and the exact setcap command for this binary.
func (t *Tunnel) reportBindError(pf config.PortForward, port int, lastErr error) {
	be := PortlessBindError{
		TunnelID: t.Config.ID,
		Domain:   pf.Domain,
		Port:     port,
	}

	if errors.Is(lastErr, syscall.EACCES) && netcap.IsPrivilegedPort(port) {
		selfPath, err := os.Executable()
		if err != nil {
			selfPath = "ssh-tunnel-manager"
		}
		be.NeedsCapability = true
		be.SetcapCommand = netcap.SetcapCommand(selfPath)
		be.Message = fmt.Sprintf(
			"Portless %q can't bind privileged port %d — on Linux this needs the %s capability.",
			pf.Domain, port, netcap.CapName)
		t.log("error", be.Message+" Grant it once: "+be.SetcapCommand)
		slog.Error("portless bind denied: missing CAP_NET_BIND_SERVICE",
			"tunnel", t.Config.Name, "domain", pf.Domain, "port", port, "setcap", be.SetcapCommand)
	} else {
		be.Message = fmt.Sprintf("Portless bind failed for %q: %v", pf.Domain, lastErr)
		t.log("error", be.Message)
		slog.Error("portless bind exhausted retries",
			"tunnel", t.Config.Name, "domain", pf.Domain, "error", lastErr)
	}

	if t.OnBindError != nil {
		t.OnBindError(be)
	}
}

func (t *Tunnel) forwardPort(ctx context.Context, client *ssh.Client, pf config.PortForward) {
	var listener net.Listener
	var localAddr string

	if pf.Portless && t.DNSRegistry != nil {
		// ExposePort > 0 means the user explicitly chose a different listen
		// port for this domain (e.g. 80 so a browser doesn't need ":8080").
		// Otherwise we mirror the remote port so the URL conveniently uses
		// the standard service port.
		exposePort := pf.RemotePort
		if pf.ExposePort > 0 {
			exposePort = pf.ExposePort
		}
		listenPort := portless.ListenerPort(exposePort)

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
			addr := fmt.Sprintf("%s:%d", e.IP.String(), listenPort)
			ln, lnErr := net.Listen("tcp", addr)
			if lnErr == nil {
				entry = e
				listener = ln
				localAddr = addr
				break
			}
			lastErr = lnErr
			// A privileged-port permission error is not the loopback IP's fault
			// — every IP will fail the same way. Stop retrying immediately and
			// let the EACCES handler below explain the capability requirement.
			if errors.Is(lnErr, syscall.EACCES) && netcap.IsPrivilegedPort(listenPort) {
				t.DNSRegistry.Release(pf.Domain)
				break
			}
			slog.Warn("portless bind retry", "tunnel", t.Config.Name, "addr", addr, "error", lnErr)
			t.log("warn", fmt.Sprintf("Portless: %s in use, trying another IP", addr))
			t.DNSRegistry.Block(e.IP)
			t.DNSRegistry.Release(pf.Domain)
		}
		if listener == nil {
			t.reportBindError(pf, exposePort, lastErr)
			return
		}
		defer t.DNSRegistry.Release(pf.Domain)
		publicAddr := fmt.Sprintf("%s.%s:%d", entry.Domain, dns.TLD, exposePort)
		if listenPort != exposePort {
			t.log("info", fmt.Sprintf("Portless: %s → %s (privileged-port redirect)", publicAddr, localAddr))
		} else {
			t.log("info", fmt.Sprintf("Portless: %s → %s", publicAddr, localAddr))
		}
	} else {
		localPort := pf.LocalPort
		if pf.Portless {
			localPort = fallbackLocalPort(pf)
		}
		localAddr = fmt.Sprintf("127.0.0.1:%d", localPort)
		ln, err := net.Listen("tcp", localAddr)
		if err != nil {
			slog.Error("failed to listen on local port",
				"tunnel", t.Config.Name, "addr", localAddr, "error", err)
			t.log("error", fmt.Sprintf("Local port %d in use: %v", localPort, err))
			return
		}
		listener = ln
		if pf.Portless {
			t.log("warn", fmt.Sprintf("Portless unavailable: %s fell back to %s", pf.Domain, localAddr))
			slog.Warn("portless forward using local-port fallback",
				"tunnel", t.Config.Name, "domain", pf.Domain, "local", localAddr)
		}
	}
	defer listener.Close()

	slog.Info("port forward listening",
		"tunnel", t.Config.Name,
		"local", localAddr,
		"remote", fmt.Sprintf("%s:%d", pf.RemoteHost, pf.RemotePort))
	var blockedOnce sync.Once

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
		t.serveHTTPProxy(ctx, client, listener, pf, header, localAddr, &blockedOnce)
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
		go t.handleForwardConn(ctx, client, conn, pf, localAddr, &blockedOnce)
	}
}

// effectiveHostHeader returns the Host header value to send upstream, or ""
// to keep raw TCP forwarding. The rewrite is opt-in: it stays off (raw TCP)
// unless the user explicitly enables it via HostHeaderOn, because rewriting
// broke plain TCP / non-HTTP services when it ran by default. When enabled,
// an explicit HostHeader wins; failing that, a portless + FQDN-looking
// RemoteHost (e.g. "dev.mix-dev.com") auto-uses the remote host so
// jumphost-routed services like Portainer "just work".
func effectiveHostHeader(pf config.PortForward) string {
	if !pf.HostHeaderOn {
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

func (t *Tunnel) serveHTTPProxy(ctx context.Context, client *ssh.Client, listener net.Listener, pf config.PortForward, hostHeader, localAddr string, blockedOnce *sync.Once) {
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
					t.reportForwardingBlocked(blockedOnce, pf, localAddr, r.err)
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

func (t *Tunnel) handleForwardConn(ctx context.Context, client *ssh.Client, local net.Conn, pf config.PortForward, localAddr string, blockedOnce *sync.Once) {
	defer local.Close()

	remoteAddr := fmt.Sprintf("%s:%d", pf.RemoteHost, pf.RemotePort)
	remote, err := client.Dial("tcp", remoteAddr)
	if err != nil {
		t.reportForwardingBlocked(blockedOnce, pf, localAddr, err)
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

func isForwardingAdministrativelyProhibited(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "administratively prohibited")
}

func (t *Tunnel) reportForwardingBlocked(once *sync.Once, pf config.PortForward, localAddr string, err error) {
	if once == nil || !isForwardingAdministrativelyProhibited(err) {
		return
	}
	once.Do(func() {
		displayAddr := localAddr
		if pf.Portless && pf.Domain != "" {
			exposePort := pf.RemotePort
			if pf.ExposePort > 0 {
				exposePort = pf.ExposePort
			}
			displayAddr = fmt.Sprintf("%s.%s:%d", pf.Domain, dns.TLD, exposePort)
		}
		remoteAddr := fmt.Sprintf("%s:%d", pf.RemoteHost, pf.RemotePort)
		message := fmt.Sprintf(
			"SSH server blocked port forwarding: %s → %s. Enable AllowTcpForwarding on the server, then reconnect this tunnel.",
			displayAddr,
			remoteAddr,
		)
		forwardErr := PortForwardError{
			TunnelID:      t.Config.ID,
			LocalAddress:  displayAddr,
			RemoteAddress: remoteAddr,
			Message:       message,
		}
		t.log("error", message)
		if t.OnForwardError != nil {
			t.OnForwardError(forwardErr)
		}
	})
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
