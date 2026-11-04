package main

import (
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"ssh-tunnel-manager/internal/tray"
	"ssh-tunnel-manager/internal/windows"
)

const mainWindowName = "main"

// attachWails gives the app its Wails application. Everything that talks to it
// tolerates a nil value, so the app logic runs in tests without a native UI.
func (a *App) attachWails(wailsApp *application.App) {
	a.wails = wailsApp
}

// attachMainWindow registers the main window and its close behaviour.
func (a *App) attachMainWindow(w *application.WebviewWindow) {
	a.mainWindow = w
	w.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) {
		if a.isQuitting() {
			return
		}
		if a.GetCloseToTray() {
			w.Hide()
			if a.prefs.Get().HideToolsWithMain {
				a.hideToolWindows()
			}
			event.Cancel()
			return
		}
		a.quit()
	})
}

func (a *App) setAppHidden() {
	a.quitMu.Lock()
	defer a.quitMu.Unlock()
	a.appHidden = true
}

// takeAppHidden reports whether the application is hidden, and clears the flag.
func (a *App) takeAppHidden() bool {
	a.quitMu.Lock()
	defer a.quitMu.Unlock()
	hidden := a.appHidden
	a.appHidden = false
	return hidden
}

func (a *App) markQuitting() {
	a.quitMu.Lock()
	defer a.quitMu.Unlock()
	a.forceQuit = true
}

func (a *App) isQuitting() bool {
	a.quitMu.Lock()
	defer a.quitMu.Unlock()
	return a.forceQuit
}

// emit sends an event to every window.
func (a *App) emit(name string, data ...any) {
	if a.wails != nil {
		a.wails.Event.Emit(name, data...)
	}
}

func (a *App) saveDialog(title, defaultName string) (string, error) {
	if a.wails == nil {
		return "", nil
	}
	dialog := a.wails.Dialog.SaveFile().SetFilename(defaultName).SetMessage(title).CanCreateDirectories(true)
	return dialog.PromptForSingleSelection()
}

func (a *App) openFileDialog(title string) (string, error) {
	if a.wails == nil {
		return "", nil
	}
	return a.wails.Dialog.OpenFile().SetTitle(title).CanChooseFiles(true).ShowHiddenFiles(true).PromptForSingleSelection()
}

func (a *App) openFilesDialog(title string) ([]string, error) {
	if a.wails == nil {
		return nil, nil
	}
	return a.wails.Dialog.OpenFile().SetTitle(title).CanChooseFiles(true).ShowHiddenFiles(true).PromptForMultipleSelection()
}

// startTray creates the native tray. Server builds have no desktop, so they
// skip it and the tray stays a no-op.
func (a *App) startTray() {
	if a.wails == nil || isServerBuild {
		return
	}
	a.tray.Start(newTraySurface(a.wails, a.showWindow))
}

// traySurface draws the tray menu with Wails.
type traySurface struct {
	app  *application.App
	tray *application.SystemTray
}

func newTraySurface(app *application.App, show func()) *traySurface {
	t := app.SystemTray.New()
	t.OnClick(show)
	t.OnDoubleClick(show)
	return &traySurface{app: app, tray: t}
}

// SetMenu implements tray.Surface.
func (s *traySurface) SetMenu(nodes []tray.Node) {
	menu := s.app.NewMenu()
	addTrayNodes(menu, nodes)
	s.tray.SetMenu(menu)
}

func addTrayNodes(menu *application.Menu, nodes []tray.Node) {
	for _, node := range nodes {
		if node.Separator {
			menu.AddSeparator()
			continue
		}
		if len(node.Children) > 0 {
			item := menu.AddSubmenu(node.Label)
			addTrayNodes(item, node.Children)
			continue
		}
		item := menu.Add(node.Label)
		if node.Tooltip != "" {
			item.SetTooltip(node.Tooltip)
		}
		if node.Disabled {
			item.SetEnabled(false)
		}
		if len(node.Icon) > 0 {
			item.SetBitmap(node.Icon)
		}
		if node.OnClick != nil {
			click := node.OnClick
			item.OnClick(func(*application.Context) { click() })
		}
	}
}

// SetIcon implements tray.Surface.
func (s *traySurface) SetIcon(icon []byte) { s.tray.SetIcon(icon) }

// SetTooltip implements tray.Surface.
func (s *traySurface) SetTooltip(tooltip string) { s.tray.SetTooltip(tooltip) }

// Destroy implements tray.Surface.
func (s *traySurface) Destroy() { s.tray.Destroy() }

// writeFileAtomic writes through a temporary file beside the target, so a failed
// save does not leave a truncated file.
func writeFileAtomic(path string, data []byte) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".stm-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}

// Smoke test: with STM_SMOKE_TEST=1 the app opens a tool window and exits with
// status 0 only after the main window and the tool window have both called the
// backend. CI runs this on every platform, because it is the only check that
// exercises the real webviews.
const smokeTimeout = 60 * time.Second

func isSmokeTest() bool { return os.Getenv("STM_SMOKE_TEST") == "1" }

func (a *App) runSmokeTest() {
	fail := func(code int, message string, args ...any) {
		slog.Error("smoke test: "+message, args...)
		os.Exit(code)
	}
	waitFor := func(what string, code int, condition func() bool) {
		deadline := time.Now().Add(smokeTimeout)
		for !condition() {
			if time.Now().After(deadline) {
				fail(code, what+" did not happen in time")
			}
			time.Sleep(100 * time.Millisecond)
		}
	}

	spec := windows.Spec{Tool: windows.ToolSettings}
	result, err := a.OpenTool(spec)
	if err != nil {
		fail(3, "opening a tool window failed", "error", err)
	}
	waitFor("the main window reaching the backend", 4, func() bool { return a.smokeSawWindow(mainWindowName) })
	waitFor("the tool window reaching the backend", 5, func() bool { return a.smokeSawWindow(result.Window) })

	// A second window must not stop the first: closing the main window hides it
	// and leaves the tool window working.
	w := a.toolWindow(result.Window)
	if w == nil {
		fail(6, "the tool window has no native window")
	}
	a.mainWindow.Close()
	waitFor("the main window hiding", 7, func() bool { return !a.mainWindow.IsVisible() })
	if !w.IsVisible() {
		fail(8, "closing the main window hid the tool window")
	}
	a.showWindow()
	waitFor("the main window coming back", 9, a.mainWindow.IsVisible)

	// The window's place is remembered, and closing it forgets its tabs.
	key := windows.Key(spec, windows.Grouping(a.prefs.Get().WindowGrouping))
	w.SetPosition(120, 90)
	a.saveToolWindowBounds(key, w)
	if _, ok := a.prefs.Get().WindowBounds[key]; !ok {
		fail(10, "the window position was not saved")
	}
	w.Close()
	waitFor("the tool window closing", 11, func() bool {
		_, err := a.GetToolWindow(result.Window)
		return err != nil
	})

	slog.Info("smoke test passed")
	a.quit()
}

// ReportWindowReady tells the backend that a window's frontend started.
func (a *App) ReportWindowReady(name string) {
	a.smokeMu.Lock()
	defer a.smokeMu.Unlock()
	if a.smokeWindows == nil {
		a.smokeWindows = make(map[string]bool)
	}
	a.smokeWindows[name] = true
}

func (a *App) smokeSawWindow(name string) bool {
	a.smokeMu.Lock()
	defer a.smokeMu.Unlock()
	return a.smokeWindows[name]
}
