package main

import (
	"runtime"

	"github.com/wailsapp/wails/v3/pkg/application"

	"ssh-tunnel-manager/internal/prefs"
)

// macWindowStyle keeps the system title bar on macOS, with its window buttons,
// dragging and zoom, and lets the window background show through it. The dark
// appearance keeps its title and buttons readable on both themes.
func macWindowStyle() application.MacWindow {
	return application.MacWindow{
		Appearance: application.NSAppearanceNameDarkAqua,
		TitleBar:   application.MacTitleBar{AppearsTransparent: true},
	}
}

// windowBackground is the colour behind a window's page. On macOS it also colours
// the title bar, so it follows --surface of the theme in frontend/src/app.css.
func windowBackground(goos, theme string) application.RGBA {
	if goos != "darwin" {
		return application.NewRGBA(10, 10, 10, 255)
	}
	if theme == prefs.ThemeGraphite {
		return application.NewRGBA(0x1e, 0x25, 0x2c, 255)
	}
	return application.NewRGBA(0x11, 0x11, 0x11, 255)
}

// applyWindowBackground recolours the open windows after a theme change. Elsewhere
// than on macOS the page covers the whole window, so nothing changes.
func (a *App) applyWindowBackground(theme string) {
	if runtime.GOOS != "darwin" {
		return
	}
	colour := windowBackground(runtime.GOOS, theme)
	if a.mainWindow != nil {
		a.mainWindow.SetBackgroundColour(colour)
	}
	a.toolWindowsMu.Lock()
	defer a.toolWindowsMu.Unlock()
	for _, w := range a.toolWindows {
		w.SetBackgroundColour(colour)
	}
}
