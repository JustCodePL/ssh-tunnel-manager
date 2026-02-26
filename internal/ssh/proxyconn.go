package ssh

import (
	"io"
	"net"
	"os/exec"
	"time"
)

// proxyConn wraps an exec.Cmd's stdin/stdout as a net.Conn, used for
// ProxyCommand-based SSH connections.
type proxyConn struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
}

// Read reads from the subprocess stdout.
func (c *proxyConn) Read(b []byte) (int, error) {
	return c.stdout.Read(b)
}

// Write writes to the subprocess stdin.
func (c *proxyConn) Write(b []byte) (int, error) {
	return c.stdin.Write(b)
}

// Close kills the subprocess and closes pipes.
func (c *proxyConn) Close() error {
	_ = c.stdin.Close()
	_ = c.stdout.Close()
	// Kill the process if still running, then wait to reap it
	if c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
	}
	_ = c.cmd.Wait()
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
