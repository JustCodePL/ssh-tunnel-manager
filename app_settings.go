package main

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"ssh-tunnel-manager/internal/config"
	"ssh-tunnel-manager/internal/prefs"
	"ssh-tunnel-manager/internal/tray"
	"ssh-tunnel-manager/internal/windows"
)

// AppSettings is what the settings window edits for the whole app.
type AppSettings struct {
	Theme             string            `json:"theme"`
	WindowGrouping    string            `json:"windowGrouping"`
	AfterDisconnect   string            `json:"afterDisconnect"`
	HideToolsWithMain bool              `json:"hideToolsWithMain"`
	Refresh           map[string]int    `json:"refresh"`
	Row               map[string]string `json:"row"`
}

// EffectiveSettings are a tunnel's settings after applying its overrides.
type EffectiveSettings struct {
	AfterDisconnect string            `json:"afterDisconnect"`
	Refresh         map[string]int    `json:"refresh"`
	Row             map[string]string `json:"row"`
	Clipboard       string            `json:"clipboard"`
}

// GetAppSettings returns the app-wide settings.
func (a *App) GetAppSettings() AppSettings {
	p := a.prefs.Get()
	return AppSettings{
		Theme:             p.Theme,
		WindowGrouping:    p.WindowGrouping,
		AfterDisconnect:   p.AfterDisconnect,
		HideToolsWithMain: p.HideToolsWithMain,
		Refresh:           prefs.ResolveRefresh(p.Refresh, nil),
		Row:               prefs.ResolveRow(p.Row, nil),
	}
}

// SetAppSettings validates and stores the app-wide settings.
func (a *App) SetAppSettings(s AppSettings) error {
	if s.Theme != prefs.ThemeTerminal && s.Theme != prefs.ThemeGraphite {
		return fmt.Errorf("unknown theme %q", s.Theme)
	}
	if !windows.ValidGrouping(windows.Grouping(s.WindowGrouping)) {
		return fmt.Errorf("unknown window grouping %q", s.WindowGrouping)
	}
	if err := validateAfterDisconnect(s.AfterDisconnect); err != nil {
		return err
	}
	if err := validateRefresh(s.Refresh); err != nil {
		return err
	}
	if err := validateRow(s.Row); err != nil {
		return err
	}
	p := a.prefs.Get()
	p.Theme = s.Theme
	p.WindowGrouping = s.WindowGrouping
	p.AfterDisconnect = s.AfterDisconnect
	p.HideToolsWithMain = s.HideToolsWithMain
	p.Refresh = s.Refresh
	p.Row = s.Row
	if err := a.prefs.Set(p); err != nil {
		return err
	}
	a.emit(eventSettingsChange)
	return nil
}

// GetTunnelSettings returns a tunnel's overrides. Empty fields inherit.
func (a *App) GetTunnelSettings(tunnelID string) prefs.TunnelSettings {
	return a.prefs.Get().Tunnels[tunnelID]
}

// SetTunnelSettings validates and stores a tunnel's overrides.
func (a *App) SetTunnelSettings(tunnelID string, s prefs.TunnelSettings) error {
	if _, ok := a.store.GetTunnel(tunnelID); !ok {
		return &tunnelNotFoundError{tunnelID}
	}
	if s.AfterDisconnect != "" {
		if err := validateAfterDisconnect(s.AfterDisconnect); err != nil {
			return err
		}
	}
	if err := validateRefresh(s.Refresh); err != nil {
		return err
	}
	if err := validateRow(s.Row); err != nil {
		return err
	}
	switch s.Clipboard {
	case "", prefs.ClipboardBoth, prefs.ClipboardTo, prefs.ClipboardFrom, prefs.ClipboardOff:
	default:
		return fmt.Errorf("unknown clipboard mode %q", s.Clipboard)
	}
	switch s.RDP {
	case "", rdpModeOn, rdpModeOff:
	default:
		return fmt.Errorf("unknown remote desktop mode %q", s.RDP)
	}
	p := a.prefs.Get()
	tunnels := make(map[string]prefs.TunnelSettings, len(p.Tunnels)+1)
	for id, settings := range p.Tunnels {
		tunnels[id] = settings
	}
	tunnels[tunnelID] = s
	p.Tunnels = tunnels
	if err := a.prefs.Set(p); err != nil {
		return err
	}
	a.emit(eventSettingsChange)
	return nil
}

// ApplyRowToGroup copies a tunnel's row layout onto the other tunnels of its
// group. It is a one-time copy: later changes do not follow.
func (a *App) ApplyRowToGroup(tunnelID string) (int, error) {
	source, ok := a.store.GetTunnel(tunnelID)
	if !ok {
		return 0, &tunnelNotFoundError{tunnelID}
	}
	if source.Group == "" {
		return 0, fmt.Errorf("%s is not in a group", source.Name)
	}
	p := a.prefs.Get()
	layout := p.Tunnels[tunnelID].Row
	tunnels := make(map[string]prefs.TunnelSettings, len(p.Tunnels))
	for id, settings := range p.Tunnels {
		tunnels[id] = settings
	}
	copied := 0
	for _, t := range a.store.GetTunnels() {
		if t.Group != source.Group || t.ID == tunnelID {
			continue
		}
		settings := tunnels[t.ID]
		settings.Row = cloneRow(layout)
		tunnels[t.ID] = settings
		copied++
	}
	p.Tunnels = tunnels
	if err := a.prefs.Set(p); err != nil {
		return 0, err
	}
	a.emit(eventSettingsChange)
	return copied, nil
}

func cloneRow(row map[string]string) map[string]string {
	if row == nil {
		return nil
	}
	out := make(map[string]string, len(row))
	for k, v := range row {
		out[k] = v
	}
	return out
}

func (a *App) effectiveTunnelSettings(tunnelID string) EffectiveSettings {
	p := a.prefs.Get()
	override := p.Tunnels[tunnelID]
	effective := EffectiveSettings{
		AfterDisconnect: p.AfterDisconnect,
		Refresh:         prefs.ResolveRefresh(p.Refresh, override.Refresh),
		Row:             prefs.ResolveRow(p.Row, override.Row),
		Clipboard:       prefs.ClipboardBoth,
	}
	if override.AfterDisconnect != "" {
		effective.AfterDisconnect = override.AfterDisconnect
	}
	if override.Clipboard != "" {
		effective.Clipboard = override.Clipboard
	}
	return effective
}

// GetEffectiveSettings returns the effective settings of every tunnel.
func (a *App) GetEffectiveSettings() map[string]EffectiveSettings {
	out := make(map[string]EffectiveSettings)
	for _, t := range a.store.GetTunnels() {
		out[t.ID] = a.effectiveTunnelSettings(t.ID)
	}
	return out
}

func validateAfterDisconnect(value string) error {
	switch value {
	case prefs.AfterDisconnectResume, prefs.AfterDisconnectKeep, prefs.AfterDisconnectClose:
		return nil
	}
	return fmt.Errorf("unknown after-disconnect mode %q", value)
}

func validateRefresh(refresh map[string]int) error {
	for monitor, seconds := range refresh {
		switch monitor {
		case "stats", "htop", "disk", "docker":
		default:
			return fmt.Errorf("unknown monitor %q", monitor)
		}
		if seconds < 0 || seconds > 3600 {
			return fmt.Errorf("refresh interval of %s must be 0 to 3600 seconds, got %d", monitor, seconds)
		}
	}
	return nil
}

func validateRow(row map[string]string) error {
	known := prefs.DefaultRow()
	for module, place := range row {
		if _, ok := known[module]; !ok {
			return fmt.Errorf("unknown row module %q", module)
		}
		switch place {
		case prefs.RowInRow, prefs.RowInPanel, prefs.RowOff:
		default:
			return fmt.Errorf("unknown row place %q", place)
		}
	}
	return nil
}

// AttachOption is one session Attach can open.
type AttachOption struct {
	Protocol   string `json:"protocol"`
	Label      string `json:"label"`
	RemoteHost string `json:"remoteHost,omitempty"`
	RemotePort int    `json:"remotePort,omitempty"`
}

// GetAttachOptions lists the sessions a tunnel itself can open: its shell, and
// its remote desktop once one was detected.
func (a *App) GetAttachOptions(tunnelID string) []AttachOption {
	options := []AttachOption{{Protocol: config.ForwardProtocolSSH, Label: "Terminal"}}
	p := a.prefs.Get()
	mode := p.Tunnels[tunnelID].RDP
	if mode == rdpModeOn || (mode != rdpModeOff && p.HostTools[tunnelID].RDP) {
		options = append(options, AttachOption{Protocol: config.ForwardProtocolRDP, Label: "Remote desktop (RDP)"})
	}
	return options
}

// trayAttachOptions adds the tunnel's forwards to what the tray can open.
func (a *App) trayAttachOptions(tunnelID string) []tray.AttachOption {
	var options []tray.AttachOption
	for _, option := range a.GetAttachOptions(tunnelID) {
		options = append(options, tray.AttachOption{Protocol: option.Protocol, Label: option.Label})
	}
	cfg, ok := a.store.GetTunnel(tunnelID)
	if !ok {
		return options
	}
	for _, pf := range cfg.PortForwards {
		label := pf.Description
		if label == "" {
			label = fmt.Sprintf("%s:%d", pf.RemoteHost, pf.RemotePort)
		}
		switch pf.EffectiveProtocol() {
		case config.ForwardProtocolRDP:
			label = "Remote desktop: " + label
		case config.ForwardProtocolSSH:
			label = "Terminal: " + label
		default:
			continue
		}
		options = append(options, tray.AttachOption{
			Protocol: pf.EffectiveProtocol(), Label: label, RemoteHost: pf.RemoteHost, RemotePort: pf.RemotePort,
		})
	}
	return options
}

func (a *App) trayAttach(tunnelID string, option tray.AttachOption) error {
	tool := windows.ToolTerminal
	if option.Protocol == config.ForwardProtocolRDP {
		tool = windows.ToolRDP
	}
	_, err := a.OpenTool(windows.Spec{
		Tool: tool, TunnelID: tunnelID, RemoteHost: option.RemoteHost, RemotePort: option.RemotePort,
	})
	return err
}

// A tunnel's override of remote desktop detection.
const (
	rdpModeOn  = "on"
	rdpModeOff = "off"
)

// GetGroups returns the groups created by hand.
func (a *App) GetGroups() []string {
	return a.prefs.Get().Groups
}

// AddGroup creates a group that can exist before its first tunnel.
func (a *App) AddGroup(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("a group needs a name")
	}
	p := a.prefs.Get()
	if slices.Contains(p.Groups, name) {
		return nil
	}
	p.Groups = append(slices.Clone(p.Groups), name)
	if err := a.prefs.Set(p); err != nil {
		return err
	}
	a.emitTunnelsChanged()
	return nil
}

// RemoveGroup forgets a group created by hand. A group with tunnels stays, because
// those tunnels name it.
func (a *App) RemoveGroup(name string) error {
	p := a.prefs.Get()
	if !slices.Contains(p.Groups, name) {
		return nil
	}
	p.Groups = slices.DeleteFunc(slices.Clone(p.Groups), func(group string) bool { return group == name })
	if err := a.prefs.Set(p); err != nil {
		return err
	}
	a.emitTunnelsChanged()
	return nil
}

// renameSavedGroup follows a group's rename in the list of groups made by hand.
func (a *App) renameSavedGroup(oldName, newName string) error {
	p := a.prefs.Get()
	if !slices.Contains(p.Groups, oldName) {
		return nil
	}
	groups := slices.DeleteFunc(slices.Clone(p.Groups), func(group string) bool { return group == oldName })
	if !slices.Contains(groups, newName) {
		groups = append(groups, newName)
	}
	p.Groups = groups
	return a.prefs.Set(p)
}

// emitTunnelsChanged tells every window to read the tunnels again. A change often
// comes from another window, such as a tunnel's settings.
func (a *App) emitTunnelsChanged() {
	if a.ctx != nil {
		a.emit("tunnels:changed")
	}
}
