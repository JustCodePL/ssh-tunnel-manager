//go:build server

package main

import (
	"os"
	"strconv"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/zalando/go-keyring"
)

// The end-to-end build must not touch the machine's real keychain.
func init() { keyring.MockInit() }

const isServerBuild = true

// serverOptions binds the end-to-end test build. Only that build uses the
// server tag; release builds are always desktop builds.
func serverOptions() application.ServerOptions {
	port := 34115
	if value, err := strconv.Atoi(os.Getenv("STM_E2E_PORT")); err == nil {
		port = value
	}
	return application.ServerOptions{Host: "127.0.0.1", Port: port}
}

func setApplicationMenu(*application.App) {}

// setSystemClipboard does nothing here: the HTTP build has no system clipboard,
// and the end-to-end tests should not touch the clipboard of the machine.
func (a *App) setSystemClipboard(string) bool {
	return true
}
