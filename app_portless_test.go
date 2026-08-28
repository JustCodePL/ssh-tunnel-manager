package main

import (
	"errors"
	"fmt"
	"strings"
	"syscall"
	"testing"

	"ssh-tunnel-manager/internal/config"
	"ssh-tunnel-manager/internal/ssh"
)

func TestHandlePortlessDNSStartFailureDegradesAddressConflict(t *testing.T) {
	app := &App{
		manager: ssh.NewManager(func(ssh.StatusEvent) {}, nil),
		logBuf:  make(map[string][]config.LogEntry),
	}
	cfg := config.TunnelConfig{ID: "bastion", Name: "bastion"}
	err := fmt.Errorf("binding UDP: %w", syscall.EADDRINUSE)

	if !app.handlePortlessDNSStartFailure(cfg, err) {
		t.Fatal("address conflict should degrade instead of remaining fatal")
	}
	if app.portlessFallback == nil {
		t.Fatal("address conflict should persist the fallback warning")
	}
	logs := app.logBuf[cfg.ID]
	if len(logs) != 1 || logs[0].Level != "warn" {
		t.Fatalf("fallback logs = %+v, want one warning", logs)
	}
	if !strings.Contains(logs[0].Message, "fell back") {
		t.Fatalf("fallback log = %q", logs[0].Message)
	}
}

func TestHandlePortlessDNSStartFailureKeepsOtherErrorsFatal(t *testing.T) {
	app := &App{
		manager: ssh.NewManager(func(ssh.StatusEvent) {}, nil),
		logBuf:  make(map[string][]config.LogEntry),
	}
	cfg := config.TunnelConfig{ID: "bastion", Name: "bastion"}

	if app.handlePortlessDNSStartFailure(cfg, errors.New("unexpected DNS startup failure")) {
		t.Fatal("non-bind DNS error must remain fatal")
	}
	if app.portlessFallback != nil {
		t.Fatal("non-bind DNS error must not show the fallback warning")
	}
	if len(app.logBuf[cfg.ID]) != 0 {
		t.Fatalf("unexpected fallback logs: %+v", app.logBuf[cfg.ID])
	}
}
