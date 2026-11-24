package ssh

import (
	"fmt"
	"io"
	"net"
	"os/exec"
	"sync"
	"time"
)

// proxyCommandWaitDelay bounds how long closing a ProxyCommand connection
// waits for the command's stderr after the command has exited.
const proxyCommandWaitDelay = 2 * time.Second

// proxyConn wraps an exec.Cmd's stdin/stdout as a net.Conn, used for
// ProxyCommand-based SSH connections.
type proxyConn struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser

	closeOnce sync.Once
}

// startProxyConn starts cmd with its stdin and stdout as the connection and
// its stderr written to stderr.
func startProxyConn(cmd *exec.Cmd, stderr io.Writer) (*proxyConn, error) {
	cmd.Stderr = stderr
	// A background process started by the command can keep stderr open after
	// the command itself is killed; stop waiting for it so Close returns.
	cmd.WaitDelay = proxyCommandWaitDelay
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("creating stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("creating stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting: %w", err)
	}
	return &proxyConn{cmd: cmd, stdin: stdin, stdout: stdout}, nil
}

// Read reads from the subprocess stdout.
func (c *proxyConn) Read(b []byte) (int, error) {
	return c.stdout.Read(b)
}

// Write writes to the subprocess stdin.
func (c *proxyConn) Write(b []byte) (int, error) {
	return c.stdin.Write(b)
}

// Close kills the subprocess and closes pipes. It is safe to call more than
// once and from several goroutines, as net.Conn requires: x/crypto/ssh closes
// the connection from its key exchange goroutine and again from
// NewClientConn when a handshake fails. A second cmd.Wait running at the same
// time would wait forever for the stderr copy result the first one took.
func (c *proxyConn) Close() error {
	c.closeOnce.Do(func() {
		_ = c.stdin.Close()
		_ = c.stdout.Close()
		// Kill the process if still running, then wait to reap it.
		if c.cmd.Process != nil {
			_ = c.cmd.Process.Kill()
		}
		_ = c.cmd.Wait()
	})
	return nil
}

// LocalAddr returns a dummy address.
func (c *proxyConn) LocalAddr() net.Addr {
	return proxyAddr("proxy-local")
}

// RemoteAddr returns a dummy address.
func (c *proxyConn) RemoteAddr() net.Addr {
	return proxyAddr("proxy-remote")
}

// SetDeadline is a no-op (subprocess pipes don't support deadlines).
func (c *proxyConn) SetDeadline(_ time.Time) error { return nil }

// SetReadDeadline is a no-op.
func (c *proxyConn) SetReadDeadline(_ time.Time) error { return nil }

// SetWriteDeadline is a no-op.
func (c *proxyConn) SetWriteDeadline(_ time.Time) error { return nil }

// proxyAddr implements net.Addr for proxy connections.
type proxyAddr string

func (a proxyAddr) Network() string { return "proxy" }
func (a proxyAddr) String() string  { return string(a) }
