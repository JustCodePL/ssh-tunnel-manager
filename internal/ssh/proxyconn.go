package ssh

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"os/exec"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	// proxyCommandWaitDelay bounds how long closing a ProxyCommand connection
	// waits for the command's stderr after the command has exited.
	proxyCommandWaitDelay = 2 * time.Second
	// proxyCommandStderrLimit is how much of a ProxyCommand's stderr is kept
	// for error messages. The command runs for the whole session, so only the
	// most recent output is kept.
	proxyCommandStderrLimit = 16 << 10
)

// proxyConn wraps an exec.Cmd's stdin/stdout as a net.Conn, used for
// ProxyCommand-based SSH connections.
type proxyConn struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	// kill stops the command and every process it started.
	kill func()

	closeOnce sync.Once
}

// startProxyConn starts cmd with its stdin and stdout as the connection and
// its stderr written to stderr. The command runs in a process group (a Job
// Object on Windows) that Close kills as a whole. cmd must have been made with
// exec.CommandContext, because the context's cancellation kills the group.
func startProxyConn(cmd *exec.Cmd, stderr io.Writer) (*proxyConn, error) {
	cmd.Stderr = stderr
	// A process that left the command's process group (or, on Windows, was
	// started before the command joined its job) can keep stderr open after
	// the command is killed; stop waiting for it so Close returns.
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
	kill, err := startProcessTree(cmd)
	if err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return nil, fmt.Errorf("starting: %w", err)
	}
	return &proxyConn{cmd: cmd, stdin: stdin, stdout: stdout, kill: kill}, nil
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
		// Kill the command and the processes it started, then reap it. The
		// group has to be killed before Wait, which releases its ID.
		c.kill()
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

// tailBuffer is an io.Writer that keeps only the last limit bytes written to
// it. It is safe for concurrent use. It holds up to twice the limit, so that
// trimming copies the kept bytes only once per limit bytes written.
type tailBuffer struct {
	mu    sync.Mutex
	limit int
	buf   []byte
	// dropped is set once older output was removed, and lastDropped is the
	// last byte removed.
	dropped     bool
	lastDropped byte
}

func newTailBuffer(limit int) *tailBuffer {
	return &tailBuffer{limit: limit}
}

// Write keeps the last limit bytes of everything written so far.
func (b *tailBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf = append(b.buf, p...)
	if len(b.buf) > 2*b.limit {
		cut := len(b.buf) - b.limit
		b.dropped = true
		b.lastDropped = b.buf[cut-1]
		b.buf = append(b.buf[:0], b.buf[cut:]...)
	}
	return len(p), nil
}

// String returns the last limit bytes written. When older output was dropped
// in the middle of a line, it starts at the next full line, or, if there is
// none, with the cut character removed.
func (b *tailBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	kept, dropped, lastDropped := b.buf, b.dropped, b.lastDropped
	if len(kept) > b.limit {
		cut := len(kept) - b.limit
		dropped, lastDropped = true, kept[cut-1]
		kept = kept[cut:]
	}
	if !dropped || lastDropped == '\n' {
		return string(kept)
	}
	if i := bytes.IndexByte(kept, '\n'); i >= 0 && i < len(kept)-1 {
		return string(kept[i+1:])
	}
	// Skip the continuation bytes of a character cut in two.
	for len(kept) > 0 && !utf8.RuneStart(kept[0]) {
		kept = kept[1:]
	}
	return string(kept)
}
