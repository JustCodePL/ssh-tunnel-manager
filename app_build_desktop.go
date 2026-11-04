//go:build !server

package main

import (
	goruntime "runtime"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const isServerBuild = false

func serverOptions() application.ServerOptions { return application.ServerOptions{} }

// setApplicationMenu gives macOS the standard edit shortcuts, so copy and paste
// keep working inside the webview.
func setApplicationMenu(app *application.App) {
	if goruntime.GOOS == "darwin" {
		app.Menu.Set(application.DefaultApplicationMenu())
	}
}

// setSystemClipboard puts text on the operating system's clipboard.
func (a *App) setSystemClipboard(text string) bool {
	return a.wails.Clipboard.SetText(text)
}
