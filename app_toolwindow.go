package main

import (
	"fmt"
	"log/slog"
	"net/url"
	"runtime"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"ssh-tunnel-manager/internal/config"
	"ssh-tunnel-manager/internal/prefs"
	"ssh-tunnel-manager/internal/windows"
)

// Session kinds a tab can own.
const (
	sessionTerminal = "terminal"
	sessionSFTP     = "sftp"
	sessionRDP      = "rdp"
)

const (
	eventWindowsChanged = "windows:changed"
	eventSettingsChange = "settings:changed"
	eventPing           = "events:ping"
	boundsSaveDelay     = 500 * time.Millisecond
)

// ToolOpenResult says where a tool opened. External is true when no native
// window exists, as in the server build, and the caller must open URL itself.
type ToolOpenResult struct {
	Window   string `json:"window"`
	Tab      string `json:"tab"`
	URL      string `json:"url"`
	External bool   `json:"external"`
}

// OpenTool opens a tool on a target in its window, creating the window if needed.
func (a *App) OpenTool(spec windows.Spec) (ToolOpenResult, error) {
	if err := a.validateToolSpec(spec); err != nil {
		return ToolOpenResult{}, err
	}
	if a.toolNeedsConnection(spec) && !a.isTunnelActive(spec.TunnelID) {
		if err := a.ConnectTunnel(spec.TunnelID); err != nil {
			return ToolOpenResult{}, fmt.Errorf("connecting %s: %w", spec.TunnelID, err)
		}
	}
	grouping := windows.Grouping(a.prefs.Get().WindowGrouping)
	opened := a.windowReg.Open(spec, grouping)
	result := ToolOpenResult{
		Window:   opened.WindowName,
		Tab:      opened.TabID,
		URL:      toolWindowURL(opened.WindowName),
		External: a.wails == nil || isServerBuild,
	}
	if result.External {
		result.URL += "&external=1"
	}
	if opened.Created {
		a.createToolWindow(opened.WindowName, spec, grouping)
	} else {
		a.focusToolWindow(opened.WindowName)
	}
	a.emit(eventWindowsChanged, opened.WindowName)
	return result, nil
}

func toolWindowURL(name string) string {
	return "/?window=" + url.QueryEscape(name)
}

func (a *App) validateToolSpec(spec windows.Spec) error {
	switch spec.Tool {
	case windows.ToolSettings:
		return nil
	case windows.ToolTerminal, windows.ToolRDP, windows.ToolFiles, windows.ToolEditor, windows.ToolDocker,
		windows.ToolHtop, windows.ToolDisk, windows.ToolLogs, windows.ToolTunnelSettings:
	default:
		return fmt.Errorf("unknown tool %q", spec.Tool)
	}
	cfg, ok := a.store.GetTunnel(spec.TunnelID)
	if !ok {
		return &tunnelNotFoundError{spec.TunnelID}
	}
	if spec.RemoteHost == "" {
		return nil
	}
	target := ForwardTarget{RemoteHost: spec.RemoteHost, RemotePort: spec.RemotePort}
	protocol := config.ForwardProtocolSSH
	if spec.Tool == windows.ToolRDP {
		protocol = config.ForwardProtocolRDP
	}
	_, err := findForward(cfg, target, protocol)
	return err
}

// toolNeedsConnection reports tools that run over the tunnel's own connection.
// A terminal or file browser on the host itself opens its own.
func (a *App) toolNeedsConnection(spec windows.Spec) bool {
	switch spec.Tool {
	case windows.ToolSettings, windows.ToolTunnelSettings, windows.ToolLogs:
		return false
	case windows.ToolTerminal, windows.ToolFiles, windows.ToolEditor:
		return spec.RemoteHost != ""
	}
	return true
}

func defaultToolSize(tool windows.Tool) (int, int) {
	switch tool {
	case windows.ToolRDP:
		return 1100, 720
	case windows.ToolDocker, windows.ToolFiles, windows.ToolEditor:
		return 960, 620
	case windows.ToolHtop:
		return 860, 640
	case windows.ToolDisk:
		return 640, 420
	case windows.ToolLogs:
		return 800, 480
	case windows.ToolSettings, windows.ToolTunnelSettings:
		return 820, 600
	}
	return 900, 560
}

func toolTitle(spec windows.Spec) string {
	names := map[windows.Tool]string{
		windows.ToolTerminal:       "Terminal",
		windows.ToolRDP:            "Remote desktop",
		windows.ToolFiles:          "Files",
		windows.ToolEditor:         "Editor",
		windows.ToolDocker:         "Docker",
		windows.ToolHtop:           "htop",
		windows.ToolDisk:           "Disk",
		windows.ToolLogs:           "Logs",
		windows.ToolSettings:       "Settings",
		windows.ToolTunnelSettings: "Tunnel settings",
	}
	switch {
	case spec.TunnelID == "":
		return names[spec.Tool]
	case spec.RemoteHost != "":
		return fmt.Sprintf("%s: %s to %s:%d", names[spec.Tool], spec.TunnelID, spec.RemoteHost, spec.RemotePort)
	}
	return names[spec.Tool] + ": " + spec.TunnelID
}

func (a *App) createToolWindow(name string, spec windows.Spec, grouping windows.Grouping) {
	if a.wails == nil || isServerBuild {
		return
	}
	key := windows.Key(spec, grouping)
	width, height := defaultToolSize(spec.Tool)
	options := application.WebviewWindowOptions{
		Name:             name,
		Title:            toolTitle(spec),
		Width:            width,
		Height:           height,
		MinWidth:         420,
		MinHeight:        280,
		BackgroundColour: windowBackground(runtime.GOOS, a.prefs.Get().Theme),
		URL:              toolWindowURL(name),
		EnableFileDrop:   spec.Tool == windows.ToolFiles,
		Mac:              macWindowStyle(),
		Windows:          application.WindowsWindow{Theme: application.Dark},
	}
	if saved, ok := a.prefs.Get().WindowBounds[key]; ok {
		rect := windows.Rect{X: saved.X, Y: saved.Y, Width: saved.Width, Height: saved.Height}
		options.Width, options.Height = saved.Width, saved.Height
		if rect.VisibleOn(a.screenRects()) {
			options.InitialPosition = application.WindowXY
			options.X, options.Y = saved.X, saved.Y
		}
	}
	w := a.wails.Window.NewWithOptions(options)

	a.toolWindowsMu.Lock()
	a.toolWindows[name] = w
	a.toolWindowsMu.Unlock()

	w.RegisterHook(events.Common.WindowClosing, func(*application.WindowEvent) {
		a.saveToolWindowBounds(key, w)
		a.forgetToolWindow(name)
	})
	w.OnWindowEvent(events.Common.WindowDidMove, func(*application.WindowEvent) { a.queueBoundsSave(key, w) })
	w.OnWindowEvent(events.Common.WindowDidResize, func(*application.WindowEvent) { a.queueBoundsSave(key, w) })
}

func (a *App) screenRects() []windows.Rect {
	if a.wails == nil {
		return nil
	}
	var rects []windows.Rect
	for _, screen := range a.wails.Screen.GetAll() {
		rects = append(rects, windows.Rect{
			X: screen.Bounds.X, Y: screen.Bounds.Y, Width: screen.Bounds.Width, Height: screen.Bounds.Height,
		})
	}
	return rects
}

// queueBoundsSave coalesces the many move and resize events of one drag.
func (a *App) queueBoundsSave(key string, w *application.WebviewWindow) {
	a.toolWindowsMu.Lock()
	defer a.toolWindowsMu.Unlock()
	if timer, ok := a.boundsTimers[key]; ok {
		timer.Reset(boundsSaveDelay)
		return
	}
	a.boundsTimers[key] = time.AfterFunc(boundsSaveDelay, func() {
		a.toolWindowsMu.Lock()
		delete(a.boundsTimers, key)
		a.toolWindowsMu.Unlock()
		a.saveToolWindowBounds(key, w)
	})
}

func (a *App) saveToolWindowBounds(key string, w *application.WebviewWindow) {
	if w.IsMinimised() {
		return
	}
	bounds := w.Bounds()
	if bounds.Width <= 0 || bounds.Height <= 0 {
		return
	}
	a.boundsMu.Lock()
	defer a.boundsMu.Unlock()
	p := a.prefs.Get()
	saved := make(map[string]prefs.WindowBounds, len(p.WindowBounds)+1)
	for k, v := range p.WindowBounds {
		saved[k] = v
	}
	saved[key] = prefs.WindowBounds{X: bounds.X, Y: bounds.Y, Width: bounds.Width, Height: bounds.Height}
	p.WindowBounds = saved
	if err := a.prefs.Set(p); err != nil {
		slog.Warn("saving window position failed", "window", key, "error", err)
	}
}

func (a *App) toolWindow(name string) *application.WebviewWindow {
	a.toolWindowsMu.Lock()
	defer a.toolWindowsMu.Unlock()
	return a.toolWindows[name]
}

func (a *App) focusToolWindow(name string) {
	if w := a.toolWindow(name); w != nil {
		if w.IsMinimised() {
			w.UnMinimise()
		}
		w.Show()
		w.Focus()
	}
}

// forgetToolWindow drops a closed window and ends everything its tabs owned.
func (a *App) forgetToolWindow(name string) {
	a.toolWindowsMu.Lock()
	delete(a.toolWindows, name)
	a.toolWindowsMu.Unlock()
	_, sessions := a.windowReg.RemoveWindow(name)
	a.closeSessions(sessions)
	a.emit(eventWindowsChanged, name)
}

func (a *App) closeSessions(sessions []windows.Session) {
	for _, session := range sessions {
		switch session.Kind {
		case sessionTerminal:
			if err := a.CloseTerminal(session.ID); err != nil {
				slog.Debug("closing terminal failed", "session", session.ID, "error", err)
			}
		case sessionSFTP:
			if err := a.CloseSFTP(session.ID); err != nil {
				slog.Debug("closing SFTP failed", "session", session.ID, "error", err)
			}
		case sessionRDP:
			a.CloseRDP(session.ID)
		}
	}
}

// GetToolWindow returns the tabs of a tool window.
func (a *App) GetToolWindow(name string) (windows.Window, error) {
	w, ok := a.windowReg.Get(name)
	if !ok {
		return windows.Window{}, fmt.Errorf("no tool window %s", name)
	}
	return w, nil
}

// ActivateTab selects a tab.
func (a *App) ActivateTab(windowName, tabID string) {
	if a.windowReg.Activate(windowName, tabID) {
		a.emit(eventWindowsChanged, windowName)
	}
}

// CloseTab closes a tab and the sessions it owns, and the window with its last tab.
func (a *App) CloseTab(windowName, tabID string) {
	w := a.toolWindow(windowName)
	sessions, empty := a.windowReg.CloseTab(windowName, tabID)
	a.closeSessions(sessions)
	if empty {
		a.toolWindowsMu.Lock()
		delete(a.toolWindows, windowName)
		a.toolWindowsMu.Unlock()
		if w != nil {
			w.Close()
		}
	}
	a.emit(eventWindowsChanged, windowName)
}

// BindTabSession makes a tab the owner of a session, so closing the tab or its
// window ends the session even if the webview is already gone.
func (a *App) BindTabSession(tabID, kind, sessionID string) {
	a.windowReg.BindSession(tabID, windows.Session{Kind: kind, ID: sessionID})
}

// PingEvents sends an event back to the frontend, so a window can tell that
// events reach it. Output sent before that would be lost.
func (a *App) PingEvents(token string) {
	a.emit(eventPing, token)
}

// emitToSession delivers an event only to the window that owns the session, so
// a busy terminal does not wake every other window. Before the tab has bound the
// session the event goes to all windows, and the frontend filters by session ID.
func (a *App) emitToSession(sessionID, name string, data any) {
	if a.wails == nil {
		return
	}
	if windowName, ok := a.windowReg.WindowOfSession(sessionID); ok {
		if w := a.toolWindow(windowName); w != nil {
			w.DispatchWailsEvent(&application.CustomEvent{Name: name, Data: data})
			return
		}
	}
	a.emit(name, data)
}

// handleToolsAfterDisconnect closes the tunnel's tabs when its settings ask for
// that. The other modes are handled inside the tabs.
func (a *App) handleToolsAfterDisconnect(tunnelID string, status config.TunnelStatus) {
	if status != config.StatusDisconnected && status != config.StatusError {
		return
	}
	if a.effectiveTunnelSettings(tunnelID).AfterDisconnect != prefs.AfterDisconnectClose {
		return
	}
	for _, ref := range a.windowReg.TabsOfTunnel(tunnelID) {
		a.CloseTab(ref.WindowName, ref.TabID)
	}
}

// hideToolWindows and showToolWindows follow the main window when the user chose
// to hide tools together with it.
func (a *App) hideToolWindows() {
	a.toolWindowsMu.Lock()
	defer a.toolWindowsMu.Unlock()
	for _, w := range a.toolWindows {
		w.Hide()
	}
}

func (a *App) showToolWindows() {
	a.toolWindowsMu.Lock()
	defer a.toolWindowsMu.Unlock()
	for _, w := range a.toolWindows {
		w.Show()
	}
}
