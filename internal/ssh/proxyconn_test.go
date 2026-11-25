//go:build !windows

package ssh

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"ssh-tunnel-manager/internal/config"
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

// A process that left the command's process group keeps stderr open; Close
// must not wait for it.
func TestProxyConnCloseDoesNotWaitForBackgroundProcesses(t *testing.T) {
	// The helper prints its PID only after leaving the group, so Close cannot
	// kill it while it is still a member.
	conn := startTestProxyConn(t, `perl -e '$| = 1; setpgrp(0, 0); print "$$\n"; exec "sleep", "30"' & exec cat`)
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

// processAlive reports whether a process with pid still exists.
func processAlive(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}

// readPID reads one line holding a PID from r.
func readPID(t *testing.T, r *bufio.Reader) int {
	t.Helper()
	line, err := r.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil {
		t.Fatal(err)
	}
	// Once the test saw the process gone, its PID may belong to another one.
	t.Cleanup(func() {
		if t.Failed() {
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
	})
	return pid
}

func waitUntilGone(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for processAlive(pid) {
		if time.Now().After(deadline) {
			t.Fatalf("process %d still runs after Close", pid)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// A helper the command started in the background, as `aws ssm start-session`
// or `ssh -W` under `sh -c` can be, must not outlive the connection.
func TestProxyConnCloseKillsProcessesTheCommandStarted(t *testing.T) {
	conn := startTestProxyConn(t, "sleep 30 & echo $!; exec cat")
	pid := readPID(t, bufio.NewReader(conn))
	closeWithin(t, 5*time.Second, conn.Close)
	waitUntilGone(t, pid)
}

// Cancelling the connection attempt kills the helpers too, even when the
// command's shell is still waiting for them.
func TestProxyConnContextCancelKillsProcessesTheCommandStarted(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	conn, err := startProxyConn(proxyCommandExec(ctx, "sleep 30 & echo $!; wait"), newTailBuffer(proxyCommandStderrLimit))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	pid := readPID(t, bufio.NewReader(conn))
	cancel()
	waitUntilGone(t, pid)
}

// A ProxyCommand that never answers must not hang the connection attempt.
func TestHandshakeWithinTimesOutOnSilentProxyCommand(t *testing.T) {
	conn := startTestProxyConn(t, "exec sleep 30")
	start := time.Now()
	_, _, _, err := handshakeWithin(context.Background(), conn, "example:22",
		&ssh.ClientConfig{HostKeyCallback: ssh.InsecureIgnoreHostKey()}, 200*time.Millisecond)
	if err == nil {
		t.Fatal("expected a timeout")
	}
	if !strings.Contains(err.Error(), "no answer within 200ms") {
		t.Fatalf("err = %v", err)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("took %v", elapsed)
	}
}

// Cancelling the context ends a ProxyCommand handshake, even when a helper
// still holds the command's output open.
func TestDialViaProxyCommandStopsWhenContextEnds(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	tun := &Tunnel{Config: config.TunnelConfig{Name: "t", Host: "example", Port: 22, ProxyCommand: "sleep 30; :"}}
	time.AfterFunc(200*time.Millisecond, cancel)
	start := time.Now()
	_, err := tun.dial(ctx, &ssh.ClientConfig{HostKeyCallback: ssh.InsecureIgnoreHostKey(), Timeout: time.Second})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("took %v", elapsed)
	}
}

func TestTailBufferKeepsTheLatestOutput(t *testing.T) {
	b := newTailBuffer(16)
	_, _ = b.Write([]byte("short\n"))
	if got := b.String(); got != "short\n" {
		t.Fatalf("got %q", got)
	}
	for i := range 1000 {
		_, _ = fmt.Fprintf(b, "line %d\n", i)
	}
	if got := b.String(); got != "line 999\n" {
		t.Fatalf("got %q", got)
	}
	if len(b.buf) > 32 {
		t.Fatalf("holds %d bytes", len(b.buf))
	}
	_, _ = b.Write([]byte(strings.Repeat("x", 100) + "\nlast"))
	if got := b.String(); got != "last" {
		t.Fatalf("got %q", got)
	}
}

// A cut that falls exactly at a line break keeps the whole next line.
func TestTailBufferKeepsALineThatStartsAtTheCut(t *testing.T) {
	b := newTailBuffer(6)
	_, _ = b.Write([]byte("a\n"))
	_, _ = b.Write([]byte("bbbb\nc"))
	if got := b.String(); got != "bbbb\nc" {
		t.Fatalf("got %q", got)
	}
	b = newTailBuffer(6)
	_, _ = b.Write([]byte("a\nbbbb\nc"))
	if got := b.String(); got != "bbbb\nc" {
		t.Fatalf("got %q", got)
	}
	b = newTailBuffer(5)
	_, _ = b.Write([]byte("a\nbbbb\nc"))
	if got := b.String(); got != "c" {
		t.Fatalf("got %q", got)
	}
	b = newTailBuffer(5)
	_, _ = b.Write([]byte("a\nbbb\n"))
	if got := b.String(); got != "bbb\n" {
		t.Fatalf("got %q", got)
	}
}

func TestTailBufferDropsCutCharacter(t *testing.T) {
	b := newTailBuffer(5)
	_, _ = b.Write([]byte("ąąą")) // 6 bytes; the first character is cut
	if got := b.String(); got != "ąą" {
		t.Fatalf("got %q", got)
	}
}

// After connecting, the command's stderr is kept only up to the limit.
func TestProxyConnStderrIsCapped(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stderr := newTailBuffer(1024)
	conn, err := startProxyConn(proxyCommandExec(ctx, "head -c 1000000 /dev/zero | tr '\\0' x >&2; echo done >&2; exec cat"), stderr)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Write([]byte("ping\n")); err != nil {
		t.Fatal(err)
	}
	if _, err := bufio.NewReader(conn).ReadString('\n'); err != nil {
		t.Fatal(err)
	}
	closeWithin(t, 5*time.Second, conn.Close)
	if got := stderr.String(); !strings.HasSuffix(got, "done\n") || len(got) > 1024 || len(stderr.buf) > 2048 {
		t.Fatalf("kept %d of %d bytes, ending %q", len(got), len(stderr.buf), got[max(0, len(got)-10):])
	}
}
