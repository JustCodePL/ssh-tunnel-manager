package tray

import (
	"fmt"
	"image/color"
	"log/slog"
	"sync"

	"github.com/energye/systray"

	"ssh-tunnel-manager/internal/config"
	"ssh-tunnel-manager/internal/ssh"
)

// Callbacks groups the functions the tray calls back into the app layer.
type Callbacks struct {
	ShowWindow func()
	Quit       func()
	Connect    func(id string) error
	Disconnect func(id string) error
	CopyToClip func(text string) error
	GetTunnels func() []config.TunnelConfig
	OnUpdate   func()
}

// Tray manages the system tray icon and context menu.
type Tray struct {
	cb Callbacks

	mu            sync.Mutex
	statuses      map[string]ssh.StatusEvent
	updateVersion string // set when an update is available
	endFunc       func() // called to tear down systray

	// Pre-rendered status dot icons (cached)
	dotGreen  []byte
	dotBlue   []byte
	dotOrange []byte
	dotRed    []byte
	dotGray   []byte
}

// New creates a new Tray with the given callbacks.
func New(cb Callbacks) *Tray {
	return &Tray{
		cb:        cb,
		statuses:  make(map[string]ssh.StatusEvent),
		dotGreen:  StatusDotIcon(DotGreen),
		dotBlue:   StatusDotIcon(DotBlue),
		dotOrange: StatusDotIcon(DotOrange),
		dotRed:    StatusDotIcon(DotRed),
		dotGray:   StatusDotIcon(DotGray),
	}
}

// Start initializes the system tray. Must be called from the main OS thread
// (i.e. from the Wails startup callback) — AppKit requires NSStatusBar
// operations to run on the main thread.
func (t *Tray) Start() {
	start, end := systray.RunWithExternalLoop(t.onReady, t.onExit)
	t.mu.Lock()
	t.endFunc = end
	t.mu.Unlock()
	start()
}

// RefreshMenu rebuilds the tray menu from current tunnel config.
// Call this after any tunnel config change (add, update, delete, import).
func (t *Tray) RefreshMenu() {
	t.updateIcon()
	t.rebuildMenu()
}

// Stop tears down the system tray.
func (t *Tray) Stop() {
	t.mu.Lock()
	end := t.endFunc
	t.mu.Unlock()
	if end != nil {
		end()
	}
}

// HandleStatusEvent processes a tunnel status change — updates internal state,
// refreshes the tray icon and menu, and sends desktop notifications.
func (t *Tray) HandleStatusEvent(event ssh.StatusEvent) {
	t.mu.Lock()
	prev, hadPrev := t.statuses[event.TunnelID]
	t.statuses[event.TunnelID] = event
	t.mu.Unlock()

	t.updateIcon()
	t.rebuildMenu()

	// Notifications — only on meaningful transitions
	if !hadPrev {
		return
	}
	switch event.Status {
	case config.StatusConnected:
		if prev.Status != config.StatusConnected {
			notify("Connected", fmt.Sprintf("Tunnel %q is now connected", event.TunnelID))
		}
	case config.StatusError:
		msg := event.Error
		if msg == "" {
			msg = "unknown error"
		}
		notify("Tunnel Error", fmt.Sprintf("%s: %s", event.TunnelID, msg))
	case config.StatusDisconnected:
		if prev.Status == config.StatusConnected {
			notify("Disconnected", fmt.Sprintf("Tunnel %q disconnected", event.TunnelID))
		}
	}
}

// SetUpdateAvailable stores the version and rebuilds the menu to show an update item.
func (t *Tray) SetUpdateAvailable(version string) {
	t.mu.Lock()
	t.updateVersion = version
	t.mu.Unlock()
	t.rebuildMenu()
}

func (t *Tray) onReady() {
	slog.Info("system tray ready")
	systray.SetTooltip("SSH Tunnel Manager")

	// Left-click: show the manager window
	systray.SetOnClick(func(menu systray.IMenu) {
		if t.cb.ShowWindow != nil {
			t.cb.ShowWindow()
		}
	})

	// Right-click: show context menu (default behavior on most platforms)
	systray.SetOnRClick(func(menu systray.IMenu) {
		menu.ShowMenu()
	})

	t.updateIcon()
	t.rebuildMenu()
}

func (t *Tray) onExit() {
	slog.Info("system tray exiting")
}

// updateIcon sets the tray icon based on aggregate tunnel status.
// Connected tunnels with assigned colors get colored dots on the icon.
func (t *Tray) updateIcon() {
	t.mu.Lock()
	statuses := make(map[string]ssh.StatusEvent, len(t.statuses))
	for k, v := range t.statuses {
		statuses[k] = v
	}
	t.mu.Unlock()

	tunnels := t.cb.GetTunnels()

	// Build a map of tunnel ID → color for quick lookup
	colorMap := make(map[string]string, len(tunnels))
	for _, tc := range tunnels {
		if tc.Color != "" {
			colorMap[tc.ID] = tc.Color
		}
	}

	var connected, errors int
	var dots []color.RGBA
	for _, s := range statuses {
		switch s.Status {
		case config.StatusConnected:
			connected++
			if hex, ok := colorMap[s.TunnelID]; ok {
				dots = append(dots, ParseHexColor(hex))
			}
		case config.StatusError:
			errors++
		}
	}

	var sc StatusColor
	switch {
	case errors > 0:
		sc = ColorRed
	case connected > 0:
		sc = ColorGreen
	default:
		sc = ColorGray
	}

	var icon []byte
	if len(dots) > 0 {
		icon = GenerateIconWithDots(sc, dots)
	} else {
		icon = GenerateIcon(sc, connected)
	}
	systray.SetIcon(icon)

	// Tooltip with count
	switch {
	case connected > 0 && errors > 0:
		systray.SetTooltip(fmt.Sprintf("SSH Tunnel Manager — %d connected, %d errors", connected, errors))
	case connected > 0:
		systray.SetTooltip(fmt.Sprintf("SSH Tunnel Manager — %d connected", connected))
	case errors > 0:
		systray.SetTooltip(fmt.Sprintf("SSH Tunnel Manager — %d errors", errors))
	default:
		systray.SetTooltip("SSH Tunnel Manager")
	}
}

// rebuildMenu clears and rebuilds the entire context menu from current state.
// Tunnels with a Group are placed under group submenus; ungrouped tunnels
// appear at the top level.
func (t *Tray) rebuildMenu() {
	systray.ResetMenu()

	// Tunnel items — grouped
	tunnels := t.cb.GetTunnels()
	t.mu.Lock()
	statuses := make(map[string]ssh.StatusEvent, len(t.statuses))
	for k, v := range t.statuses {
		statuses[k] = v
	}
	t.mu.Unlock()

	// Count total active tunnels for header
	var totalActive int
	for _, s := range statuses {
		if isActive(s.Status) {
			totalActive++
		}
	}

	// Header with active count
	headerLabel := "SSH Tunnel Manager"
	if totalActive > 0 {
		headerLabel = fmt.Sprintf("SSH Tunnel Manager — %d active", totalActive)
	}
	header := systray.AddMenuItem(headerLabel, "")
	header.Disable()

	systray.AddSeparator()

	// Collect groups in order of first appearance
	var groupOrder []string
	groups := make(map[string][]config.TunnelConfig)
	var ungrouped []config.TunnelConfig

	for _, tc := range tunnels {
		if tc.Group != "" {
			if _, seen := groups[tc.Group]; !seen {
				groupOrder = append(groupOrder, tc.Group)
			}
			groups[tc.Group] = append(groups[tc.Group], tc)
		} else {
			ungrouped = append(ungrouped, tc)
		}
	}

	// Ungrouped tunnels first, at top level
	for _, tc := range ungrouped {
		t.addTunnelMenuItem(tc, statuses[tc.ID], nil)
	}

	// Grouped tunnels under submenus
	for _, g := range groupOrder {
		members := groups[g]

		// Count connected in this group
		var groupConnected int
		var allConnected, noneConnected bool
		for _, tc := range members {
			if s, ok := statuses[tc.ID]; ok && isActive(s.Status) {
				groupConnected++
			}
		}
		allConnected = groupConnected == len(members)
		noneConnected = groupConnected == 0

		groupLabel := fmt.Sprintf("%s (%d/%d)", g, groupConnected, len(members))
		groupItem := systray.AddMenuItem(groupLabel, "")

		// Connect All / Disconnect All as first submenu item
		if allConnected {
			disconnectAll := groupItem.AddSubMenuItem("Disconnect All", "")
			groupName := g
			ids := make([]string, len(members))
			for i, tc := range members {
				ids[i] = tc.ID
			}
			disconnectAll.Click(func() {
				slog.Info("tray: disconnecting group", "group", groupName)
				for _, id := range ids {
					if err := t.cb.Disconnect(id); err != nil {
						slog.Error("tray: disconnect failed", "tunnel", id, "error", err)
					}
				}
			})
		} else if noneConnected {
			connectAll := groupItem.AddSubMenuItem("Connect All", "")
			groupName := g
			ids := make([]string, len(members))
			for i, tc := range members {
				ids[i] = tc.ID
			}
			connectAll.Click(func() {
				slog.Info("tray: connecting group", "group", groupName)
				for _, id := range ids {
					if err := t.cb.Connect(id); err != nil {
						slog.Error("tray: connect failed", "tunnel", id, "error", err)
					}
				}
			})
		} else {
			// Mixed state — show both
			connectRest := groupItem.AddSubMenuItem("Connect All", "")
			disconnectAll := groupItem.AddSubMenuItem("Disconnect All", "")
			groupName := g
			ids := make([]string, len(members))
			for i, tc := range members {
				ids[i] = tc.ID
			}
			connectRest.Click(func() {
				slog.Info("tray: connecting group", "group", groupName)
				for _, id := range ids {
					if err := t.cb.Connect(id); err != nil {
						slog.Error("tray: connect failed", "tunnel", id, "error", err)
					}
				}
			})
			disconnectAll.Click(func() {
				slog.Info("tray: disconnecting group", "group", groupName)
				for _, id := range ids {
					if err := t.cb.Disconnect(id); err != nil {
						slog.Error("tray: disconnect failed", "tunnel", id, "error", err)
					}
				}
			})
		}

		for _, tc := range members {
			t.addTunnelMenuItem(tc, statuses[tc.ID], groupItem)
		}
	}

	if len(tunnels) == 0 {
		noTunnels := systray.AddMenuItem("No tunnels configured", "")
		noTunnels.Disable()
	}

	systray.AddSeparator()

	t.mu.Lock()
	updateVer := t.updateVersion
	t.mu.Unlock()

	if updateVer != "" {
		updateItem := systray.AddMenuItem("⬆ Update to v"+updateVer, "Download and install the latest version")
		updateItem.Click(func() {
			slog.Info("tray: user triggered update", "version", updateVer)
			if t.cb.OnUpdate != nil {
				t.cb.OnUpdate()
			}
		})
		systray.AddSeparator()
	}

	// Show Window
	showItem := systray.AddMenuItem("Show Window", "Open the manager window")
	showItem.Click(func() {
		if t.cb.ShowWindow != nil {
			t.cb.ShowWindow()
		}
	})

	// Quit
	quitItem := systray.AddMenuItem("Quit", "Quit SSH Tunnel Manager")
	quitItem.Click(func() {
		if t.cb.Quit != nil {
			t.cb.Quit()
		}
	})
}

// isActive returns true for statuses that count as "active" (connected,
// connecting, or reconnecting).
func isActive(s config.TunnelStatus) bool {
	switch s {
	case config.StatusConnected, config.StatusConnecting, config.StatusReconnecting:
		return true
	}
	return false
}

func (t *Tray) addTunnelMenuItem(tc config.TunnelConfig, se ssh.StatusEvent, parent *systray.MenuItem) {
	status := se.Status
	if status == "" {
		status = config.StatusDisconnected
	}

	// Tooltip shows status on hover
	var tooltip string
	switch status {
	case config.StatusConnected:
		tooltip = tc.Name + " — connected"
	case config.StatusConnecting:
		tooltip = tc.Name + " — connecting..."
	case config.StatusReconnecting:
		tooltip = tc.Name + " — reconnecting..."
	case config.StatusError:
		errMsg := se.Error
		if errMsg == "" {
			errMsg = "unknown error"
		}
		tooltip = tc.Name + " — error: " + errMsg
	default:
		tooltip = tc.Name + " — disconnected"
	}

	// Unicode status prefix for visibility
	var prefix string
	switch status {
	case config.StatusConnected:
		prefix = "● "
	case config.StatusConnecting:
		prefix = "◐ "
	case config.StatusReconnecting:
		prefix = "◌ "
	case config.StatusError:
		prefix = "✖ "
	default:
		prefix = "○ "
	}
	label := prefix + tc.Name

	var item *systray.MenuItem
	if parent != nil {
		item = parent.AddSubMenuItem(label, tooltip)
	} else {
		item = systray.AddMenuItem(label, tooltip)
	}

	// Colored icon for connection state (supplement to text prefix)
	switch status {
	case config.StatusConnected:
		item.SetIcon(t.dotGreen)
	case config.StatusConnecting:
		item.SetIcon(t.dotBlue)
	case config.StatusReconnecting:
		item.SetIcon(t.dotOrange)
	case config.StatusError:
		item.SetIcon(t.dotRed)
	default:
		item.SetIcon(t.dotGray)
	}

	// Connect or Disconnect action as first submenu item
	tunnelID := tc.ID
	tunnelName := tc.Name
	switch status {
	case config.StatusConnected, config.StatusConnecting, config.StatusReconnecting:
		disconnectItem := item.AddSubMenuItem("Disconnect", "")
		disconnectItem.Click(func() {
			slog.Info("tray: disconnecting tunnel", "tunnel", tunnelName)
			if err := t.cb.Disconnect(tunnelID); err != nil {
				slog.Error("tray: disconnect failed", "tunnel", tunnelName, "error", err)
			}
		})
	default:
		connectItem := item.AddSubMenuItem("Connect", "")
		connectItem.Click(func() {
			slog.Info("tray: connecting tunnel", "tunnel", tunnelName)
			if err := t.cb.Connect(tunnelID); err != nil {
				slog.Error("tray: connect failed", "tunnel", tunnelName, "error", err)
			}
		})
	}

	// Port forward submenu items — copy to clipboard on click
	for _, pf := range tc.PortForwards {
		portLabel := fmt.Sprintf("%d", pf.LocalPort)
		if pf.Description != "" {
			portLabel = fmt.Sprintf("%d (%s)", pf.LocalPort, pf.Description)
		}
		clipText := fmt.Sprintf("127.0.0.1:%d", pf.LocalPort)
		sub := item.AddSubMenuItem(portLabel, "Copy "+clipText)
		sub.Click(func() {
			if t.cb.CopyToClip != nil {
				if err := t.cb.CopyToClip(clipText); err != nil {
					slog.Error("tray: clipboard copy failed", "error", err)
				}
			}
		})
	}
}
