//go:build !windows

package ssh

import (
	"bufio"
	"bytes"
	"context"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func startTestProxyConn(t *testing.T, script string) *proxyConn {
	t.Helper()
	// The context is cancelled only after the test, so Close runs while a
	// connection attempt still holds it, as in the app.
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	var stderr bytes.Buffer
	conn, err := startProxyConn(proxyCommandExec(ctx, script), &stderr)
	if err != nil {
		t.Fatal(err)
	}
	return conn
}

func closeWithin(t *testing.T, limit time.Duration, closes ...func() error) {
	t.Helper()
	var wg sync.WaitGroup
	done := make(chan struct{})
	for _, closeFn := range closes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = closeFn()
		}()
	}
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(limit):
		t.Fatalf("Close did not return within %s", limit)
	}
}

// x/crypto/ssh closes a failed handshake's connection from two goroutines at
// once; both calls have to return.
func TestProxyConnCloseFromTwoGoroutines(t *testing.T) {
	for range 50 {
		conn := startTestProxyConn(t, "exec cat")
		closeWithin(t, 10*time.Second, conn.Close, conn.Close)
		if err := conn.Close(); err != nil {
			t.Fatalf("closing again: %v", err)
		}
	}
}

// A process the command left in the background keeps stderr open; Close
// must not wait for it.
func TestProxyConnCloseDoesNotWaitForBackgroundProcesses(t *testing.T) {
	conn := startTestProxyConn(t, "sleep 30 & echo $!; exec cat")
	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if p, err := os.FindProcess(pid); err == nil {
			_ = p.Kill()
		}
	})
	closeWithin(t, proxyCommandWaitDelay+5*time.Second, conn.Close)
}
