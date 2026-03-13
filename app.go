package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"ssh-tunnel-manager/internal/autostart"
	"ssh-tunnel-manager/internal/config"
	"ssh-tunnel-manager/internal/keychain"
	"ssh-tunnel-manager/internal/prefs"
	"ssh-tunnel-manager/internal/ssh"
	"ssh-tunnel-manager/internal/sshconfig"
	"ssh-tunnel-manager/internal/tray"
	"ssh-tunnel-manager/internal/updater"
)

// App is the Wails application struct, bridging Go backend and frontend.
type App struct {
	ctx         context.Context
	store       *config.Store
	prefs       *prefs.Store
	manager     *ssh.Manager
	termMgr     *ssh.TerminalManager
	tray        *tray.Tray
	startHidden bool
	forceQuit   bool

	// Passphrase prompt coordination
	passphraseMu   sync.Mutex
	passphraseChan chan string

	// Pending update info (nil if no update available)
	updateMu      sync.Mutex
	pendingUpdate *updater.UpdateInfo

	// Per-tunnel log buffer (ring buffer, max 300 entries per tunnel)
	logMu  sync.Mutex
	logBuf map[string][]config.LogEntry
}

// NewApp creates a new App instance.
func NewApp(store *config.Store, prefsStore *prefs.Store, startHidden bool) *App {
	app := &App{
		store:       store,
		prefs:       prefsStore,
		startHidden: startHidden,
		logBuf:      make(map[string][]config.LogEntry),
	}
	app.termMgr = ssh.NewTerminalManager(app.getPassphrase)
	app.manager = ssh.NewManager(func(event ssh.StatusEvent) {
		app.emitStatus(event)
		app.tray.HandleStatusEvent(event)
	}, app.getPassphrase)
	app.manager.WithLogEmitter(func(tunnelID, level, msg string) {
		entry := config.LogEntry{Timestamp: time.Now(), Level: level, Message: msg}
		app.logMu.Lock()
		buf := app.logBuf[tunnelID]
		if len(buf) >= 300 {
			buf = buf[1:]
		}
		app.logBuf[tunnelID] = append(buf, entry)
		app.logMu.Unlock()
		if app.ctx != nil {
			runtime.EventsEmit(app.ctx, "tunnel:log", map[string]any{
				"tunnelId": tunnelID,
				"entry":    entry,
			})
		}
	})
	app.tray = tray.New(tray.Callbacks{
		ShowWindow: func() { app.showWindow() },
		Quit:       func() { app.quit() },
		Connect:    func(id string) error { return app.ConnectTunnel(id) },
		Disconnect: func(id string) error { return app.DisconnectTunnel(id) },
		CopyToClip: func(text string) error { return app.copyToClipboard(text) },
		GetTunnels: func() []config.TunnelConfig { return store.GetTunnels() },
		OnUpdate:   func() { app.installUpdateFromTray() },
	})
	return app
}

// startup is called by Wails when the application starts.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.tray.Start()

	if !a.startHidden {
		runtime.WindowShow(a.ctx)
	}

	go func() {
		info, err := updater.Check(ctx, Version, "JustCodePL/ssh-tunnel-manager")
		if err != nil {
			slog.Warn("update check failed", "error", err)
			return
		}
		if info == nil {
			return
		}
		slog.Info("update available", "version", info.LatestVersion)
		a.updateMu.Lock()
		a.pendingUpdate = info
		a.updateMu.Unlock()
		runtime.EventsEmit(a.ctx, "updater:update-available", info)
		a.tray.SetUpdateAvailable(info.LatestVersion)
	}()
}

// shutdown is called by Wails when the application is closing.
func (a *App) shutdown(ctx context.Context) {
	slog.Info("shutting down, disconnecting all tunnels")
	a.tray.Stop()
	a.termMgr.CloseAll()
	a.manager.DisconnectAll()
}

// GetTunnels returns all configured tunnels.
func (a *App) GetTunnels() []config.TunnelConfig {
	return a.store.GetTunnels()
}

// GetConfigFiles returns the list of SSH config source files (main + included),
// with paths collapsed to use ~/ where possible.
func (a *App) GetConfigFiles() []string {
	files := a.store.GetConfigFiles()
	for i, f := range files {
		files[i] = sshconfig.CollapseTildePath(f)
	}
	return files
}

// GetIncludedConfigFiles lists the files that can be written to when Include
// directives are present in the main config.
func (a *App) GetIncludedConfigFiles() []config.ConfigFileInfo {
	files, err := a.store.GetIncludedConfigFiles()
	if err != nil {
		slog.Warn("failed to list included config files", "error", err)
		return nil
	}
	return files
}

// AddTunnel persists a new tunnel configuration. ID is set to Name.
func (a *App) AddTunnel(t config.TunnelConfig) error {
	t.ID = t.Name
	if err := a.store.AddTunnel(t); err != nil {
		return err
	}
	a.tray.RefreshMenu()
	return nil
}

// UpdateTunnel updates an existing tunnel configuration.
// If the name changed, the tunnel is disconnected under the old ID first.
func (a *App) UpdateTunnel(t config.TunnelConfig) error {
	old, ok := a.store.GetTunnel(t.ID)
	if ok && old.Name != t.Name {
		_ = a.manager.Disconnect(t.ID)
	}
	if err := a.store.UpdateTunnel(t); err != nil {
		return err
	}
	a.tray.RefreshMenu()
	return nil
}

// DeleteTunnel removes a tunnel by ID. Disconnects it first if running.
func (a *App) DeleteTunnel(id string) error {
	_ = a.manager.Disconnect(id) // ignore error if not connected
	if err := a.store.DeleteTunnel(id); err != nil {
		return err
	}
	a.tray.RefreshMenu()
	return nil
}

// SetTunnelPinned toggles the pinned state of a tunnel and persists it.
func (a *App) SetTunnelPinned(id string, pinned bool) error {
	t, ok := a.store.GetTunnel(id)
	if !ok {
		return &tunnelNotFoundError{id}
	}
	t.Pinned = pinned
	if err := a.store.UpdateTunnel(t); err != nil {
		return err
	}
	a.tray.RefreshMenu()
	return nil
}

// ConnectTunnel starts an SSH connection for the given tunnel ID.
func (a *App) ConnectTunnel(id string) error {
	cfg, ok := a.store.GetTunnel(id)
	if !ok {
		return &tunnelNotFoundError{id}
	}
	return a.manager.Connect(cfg)
}

// DisconnectTunnel stops the SSH connection for the given tunnel ID.
func (a *App) DisconnectTunnel(id string) error {
	return a.manager.Disconnect(id)
}

// GetTunnelStatuses returns the current status of all tunnels.
func (a *App) GetTunnelStatuses() map[string]ssh.StatusEvent {
	return a.manager.GetStatuses()
}

// OpenTerminal opens an interactive SSH terminal session for the given tunnel.
func (a *App) OpenTerminal(tunnelID string) (string, error) {
	cfg, ok := a.store.GetTunnel(tunnelID)
	if !ok {
		return "", &tunnelNotFoundError{tunnelID}
	}

	sessionID, err := a.termMgr.OpenSession(cfg,
		func(sessionID, data string) {
			if a.ctx != nil {
				runtime.EventsEmit(a.ctx, "terminal:output", map[string]any{
					"sessionId": sessionID,
					"data":      data,
				})
			}
		},
		func(sessionID string) {
			if a.ctx != nil {
				runtime.EventsEmit(a.ctx, "terminal:closed", map[string]any{
					"sessionId": sessionID,
				})
			}
		},
	)
	if err != nil {
		return "", fmt.Errorf("opening terminal for %s: %w", cfg.Name, err)
	}
	return sessionID, nil
}

// TerminalWrite sends input data to a terminal session.
func (a *App) TerminalWrite(sessionID, data string) error {
	ts, ok := a.termMgr.GetSession(sessionID)
	if !ok {
		return fmt.Errorf("terminal session %s not found", sessionID)
	}
	return ts.Write(data)
}

// CloseTerminal closes a terminal session.
func (a *App) CloseTerminal(sessionID string) error {
	a.termMgr.CloseSession(sessionID)
	return nil
}

// ResizeTerminal resizes the PTY of a terminal session.
func (a *App) ResizeTerminal(sessionID string, cols, rows int) error {
	ts, ok := a.termMgr.GetSession(sessionID)
	if !ok {
		return fmt.Errorf("terminal session %s not found", sessionID)
	}
	return ts.Resize(cols, rows)
}

func (a *App) showWindow() {
	if a.ctx != nil {
		runtime.WindowShow(a.ctx)
	}
}

func (a *App) quit() {
	if a.ctx != nil {
		a.forceQuit = true
		runtime.Quit(a.ctx)
	}
}

func (a *App) copyToClipboard(text string) error {
	if a.ctx == nil {
		return nil
	}
	return runtime.ClipboardSetText(a.ctx, text)
}

func (a *App) emitStatus(event ssh.StatusEvent) {
	if a.ctx == nil {
		return
	}
	runtime.EventsEmit(a.ctx, "tunnel:status-changed", event)
	if event.Error != "" {
		runtime.EventsEmit(a.ctx, "tunnel:error", event)
	}
}

// getPassphrase is called by the SSH manager when an encrypted key needs
// a passphrase. It first checks the OS keychain, then prompts the user
// via the frontend.
func (a *App) getPassphrase(keyPath string) (string, error) {
	// Try keychain first
	stored, err := keychain.GetPassphrase(keyPath)
	if err != nil {
		slog.Warn("keychain lookup failed", "keyPath", keyPath, "error", err)
	}
	if stored != "" {
		return stored, nil
	}

	// Ask frontend for passphrase
	a.passphraseMu.Lock()
	a.passphraseChan = make(chan string, 1)
	a.passphraseMu.Unlock()

	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "passphrase:request", keyPath)
	}

	// Wait for the frontend to call SubmitPassphrase
	passphrase := <-a.passphraseChan

	a.passphraseMu.Lock()
	a.passphraseChan = nil
	a.passphraseMu.Unlock()

	if passphrase == "" {
		return "", fmt.Errorf("passphrase entry cancelled")
	}

	// Store in keychain for next time
	if err := keychain.SetPassphrase(keyPath, passphrase); err != nil {
		slog.Warn("failed to store passphrase in keychain", "keyPath", keyPath, "error", err)
	}

	return passphrase, nil
}

// SubmitPassphrase is called by the frontend to provide a passphrase
// for an encrypted SSH key.
func (a *App) SubmitPassphrase(passphrase string) {
	a.passphraseMu.Lock()
	ch := a.passphraseChan
	a.passphraseMu.Unlock()

	if ch != nil {
		ch <- passphrase
	}
}

// GetAutostart returns whether the app is set to start on login.
func (a *App) GetAutostart() bool {
	enabled, _ := autostart.IsEnabled()
	return enabled
}

// SetAutostart enables or disables starting the app on login.
func (a *App) SetAutostart(enabled bool) error {
	if enabled {
		return autostart.Enable()
	}
	return autostart.Disable()
}

// GetCurrentVersion returns the compiled-in application version.
func (a *App) GetCurrentVersion() string {
	return Version
}

// CheckForUpdate contacts GitHub for a newer release. Returns nil if already
// up to date. Stores result in pendingUpdate and emits the update-available event.
func (a *App) CheckForUpdate() (*updater.UpdateInfo, error) {
	info, err := updater.Check(a.ctx, Version, "JustCodePL/ssh-tunnel-manager")
	if err != nil {
		return nil, err
	}
	if info == nil {
		return nil, nil
	}
	a.updateMu.Lock()
	a.pendingUpdate = info
	a.updateMu.Unlock()
	runtime.EventsEmit(a.ctx, "updater:update-available", info)
	a.tray.SetUpdateAvailable(info.LatestVersion)
	return info, nil
}

// InstallUpdate downloads and runs the pending installer, then quits the app.
func (a *App) InstallUpdate() error {
	a.updateMu.Lock()
	info := a.pendingUpdate
	a.updateMu.Unlock()

	if info == nil {
		return fmt.Errorf("no pending update")
	}

	if err := updater.Install(a.ctx, info); err != nil {
		return err
	}
	a.quit()
	return nil
}

func (a *App) installUpdateFromTray() {
	if err := a.InstallUpdate(); err != nil {
		slog.Error("tray: install update failed", "error", err)
	}
}

// SelectFile opens a native file dialog for picking an SSH config file.
func (a *App) SelectFile() (string, error) {
	return runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Select SSH Config File",
		Filters: []runtime.FileFilter{
			{DisplayName: "All Files", Pattern: "*"},
		},
	})
}

// SelectSaveFile opens a native save dialog.
func (a *App) SelectSaveFile(defaultName string) (string, error) {
	return runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "Export Tunnels",
		DefaultFilename: defaultName,
	})
}

// ImportPreview parses an SSH config file and returns the host entries found
// without importing them. The frontend uses this to show a preview.
func (a *App) ImportPreview(path string) ([]config.TunnelConfig, error) {
	entries, err := sshconfig.ParseFile(path)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	var tunnels []config.TunnelConfig
	for _, e := range entries {
		tunnels = append(tunnels, entryToTunnel(e))
	}
	return tunnels, nil
}

// ImportTunnels imports the selected tunnels (by name) from the given file.
func (a *App) ImportTunnels(path string, names []string) (int, error) {
	entries, err := sshconfig.ParseFile(path)
	if err != nil {
		return 0, fmt.Errorf("parsing %s: %w", path, err)
	}

	nameSet := make(map[string]bool, len(names))
	for _, n := range names {
		nameSet[n] = true
	}

	var imported int
	for _, e := range entries {
		if !nameSet[e.Alias] {
			continue
		}
		tc := entryToTunnel(e)
		tc.ID = tc.Name
		if err := a.store.AddTunnel(tc); err != nil {
			slog.Warn("import: skipping duplicate tunnel", "name", tc.Name, "error", err)
			continue
		}
		imported++
	}

	if imported > 0 {
		runtime.EventsEmit(a.ctx, "tunnels:changed")
		a.tray.RefreshMenu()
	}
	return imported, nil
}

// ExportTunnels writes selected tunnels (by ID) to an SSH config file.
func (a *App) ExportTunnels(path string, ids []string) error {
	idSet := make(map[string]bool, len(ids))
	for _, id := range ids {
		idSet[id] = true
	}

	tunnels := a.store.GetTunnels()
	var blocks []string
	for _, t := range tunnels {
		if !idSet[t.ID] {
			continue
		}
		blocks = append(blocks, sshconfig.RenderHostBlock(tunnelToEntry(t)))
	}

	content := strings.Join(blocks, "\n")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("writing export file: %w", err)
	}
	return nil
}

func entryToTunnel(e sshconfig.HostEntry) config.TunnelConfig {
	t := config.TunnelConfig{
		ID:              e.Alias,
		Name:            e.Alias,
		Host:            e.HostName,
		Port:            e.Port,
		User:            e.User,
		KeyPath:         e.IdentityFile,
		ProxyCommand:    e.ProxyCommand,
		ProxyJump:       e.ProxyJump,
		Color:           e.Color,
		Group:           e.Group,
		AutoConnect:     e.AutoConnect,
		Pinned:          e.Pinned,
		SourceFile:      e.SourceFile,
		SourceFileLabel: sshconfig.CollapseTildePath(e.SourceFile),
	}
	for _, pf := range e.PortForwards {
		t.PortForwards = append(t.PortForwards, config.PortForward{
			LocalPort:   pf.LocalPort,
			RemoteHost:  pf.RemoteHost,
			RemotePort:  pf.RemotePort,
			Description: pf.Description,
		})
	}
	return t
}

func tunnelToEntry(t config.TunnelConfig) sshconfig.HostEntry {
	e := sshconfig.HostEntry{
		Alias:        t.Name,
		HostName:     t.Host,
		Port:         t.Port,
		User:         t.User,
		IdentityFile: t.KeyPath,
		ProxyCommand: t.ProxyCommand,
		ProxyJump:    t.ProxyJump,
		Color:        t.Color,
		Group:        t.Group,
		AutoConnect:  t.AutoConnect,
		Pinned:       t.Pinned,
		SourceFile:   t.SourceFile,
	}
	for _, pf := range t.PortForwards {
		e.PortForwards = append(e.PortForwards, sshconfig.PortForwardEntry{
			LocalPort:   pf.LocalPort,
			RemoteHost:  pf.RemoteHost,
			RemotePort:  pf.RemotePort,
			Description: pf.Description,
		})
	}
	return e
}

// GetTunnelLogs returns the buffered log entries for a given tunnel ID.
func (a *App) GetTunnelLogs(tunnelID string) []config.LogEntry {
	a.logMu.Lock()
	defer a.logMu.Unlock()
	buf := a.logBuf[tunnelID]
	if len(buf) == 0 {
		return []config.LogEntry{}
	}
	out := make([]config.LogEntry, len(buf))
	copy(out, buf)
	return out
}

// ClearTunnelLogs removes all buffered log entries for a given tunnel ID.
func (a *App) ClearTunnelLogs(tunnelID string) {
	a.logMu.Lock()
	defer a.logMu.Unlock()
	delete(a.logBuf, tunnelID)
}

// ConnectGroup connects all tunnels belonging to the given group.
func (a *App) ConnectGroup(group string) error {
	tunnels := a.store.GetTunnels()
	var firstErr error
	for _, t := range tunnels {
		if t.Group != group {
			continue
		}
		if err := a.manager.Connect(t); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// DisconnectGroup disconnects all tunnels belonging to the given group.
func (a *App) DisconnectGroup(group string) error {
	tunnels := a.store.GetTunnels()
	var firstErr error
	for _, t := range tunnels {
		if t.Group != group {
			continue
		}
		if err := a.manager.Disconnect(t.ID); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// RenameGroup renames a group across all tunnels that belong to it.
func (a *App) RenameGroup(oldName, newName string) error {
	newName = strings.TrimSpace(newName)
	if newName == "" {
		return fmt.Errorf("new group name cannot be empty")
	}
	if oldName == newName {
		return nil
	}
	tunnels := a.store.GetTunnels()
	for _, t := range tunnels {
		if t.Group != oldName {
			continue
		}
		t.Group = newName
		if err := a.store.UpdateTunnel(t); err != nil {
			return fmt.Errorf("renaming group for %s: %w", t.Name, err)
		}
	}
	a.tray.RefreshMenu()
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "tunnels:changed")
	}
	return nil
}

type tunnelNotFoundError struct {
	id string
}

func (e *tunnelNotFoundError) Error() string {
	return "tunnel not found: " + e.id
}

// GetCloseToTray returns whether the window close button hides to tray (true)
// or quits the application (false).
func (a *App) GetCloseToTray() bool {
	return a.prefs.Get().CloseToTray
}

// SetCloseToTray configures the close button behaviour and persists the setting.
func (a *App) SetCloseToTray(enabled bool) error {
	p := a.prefs.Get()
	p.CloseToTray = enabled
	return a.prefs.Set(p)
}
