// Package tray renders the system tray. BuildMenu is a pure function from the
// app state to a tree of menu nodes, so the menu's content can be tested
// without a native tray; Tray applies that tree through a Surface.
package tray

import (
	"fmt"
	"log/slog"
	"sync"

	"ssh-tunnel-manager/internal/config"
	"ssh-tunnel-manager/internal/dns"
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
	// AttachOptions lists what Attach can open for a tunnel, in menu order.
	AttachOptions func(id string) []AttachOption
	// Attach opens the chosen session.
	Attach func(id string, option AttachOption) error
}

// AttachOption is one way to attach to a tunnel or to a target behind it.
type AttachOption struct {
	Protocol   string
	Label      string
	RemoteHost string
	RemotePort int
}

// Node is one entry of the tray menu.
type Node struct {
	Label     string
	Tooltip   string
	Disabled  bool
	Separator bool
	Icon      []byte
	OnClick   func()
	Children  []Node
}

// Surface is the native tray the menu and icon are applied to.
type Surface interface {
	SetMenu(nodes []Node)
	SetIcon(icon []byte)
	SetTooltip(tooltip string)
	Destroy()
}

// Tray keeps the tray's state and refreshes its Surface.
type Tray struct {
	cb Callbacks

	mu            sync.Mutex
	menuMu        sync.Mutex // serializes menu rebuilds
	statuses      map[string]ssh.StatusEvent
	updateVersion string
	surface       Surface

	dots dotIcons
}

type dotIcons struct {
	green, blue, orange, red, gray []byte
}

// New creates a Tray with the given callbacks.
func New(cb Callbacks) *Tray {
	return &Tray{
		cb:       cb,
		statuses: make(map[string]ssh.StatusEvent),
		dots: dotIcons{
			green:  StatusDotIcon(DotGreen),
			blue:   StatusDotIcon(DotBlue),
			orange: StatusDotIcon(DotOrange),
			red:    StatusDotIcon(DotRed),
			gray:   StatusDotIcon(DotGray),
		},
	}
}

// Start attaches the tray to its native surface and draws it.
func (t *Tray) Start(surface Surface) {
	t.mu.Lock()
	t.surface = surface
	t.mu.Unlock()
	slog.Info("system tray ready")
	t.RefreshMenu()
}

// RefreshMenu redraws the icon and the menu from the current state.
func (t *Tray) RefreshMenu() {
	t.updateIcon()
	t.rebuildMenu()
}

// Stop tears down the system tray.
func (t *Tray) Stop() {
	t.mu.Lock()
	surface := t.surface
	t.surface = nil
	t.mu.Unlock()
	if surface != nil {
		surface.Destroy()
	}
}

// HandleStatusEvent records a tunnel status change, refreshes the tray and
// sends a desktop notification on meaningful transitions. Tunnel errors are not
// notified here: the frontend shows them inside the app.
func (t *Tray) HandleStatusEvent(event ssh.StatusEvent) {
	t.mu.Lock()
	prev, hadPrev := t.statuses[event.TunnelID]
	t.statuses[event.TunnelID] = event
	t.mu.Unlock()

	t.RefreshMenu()

	if !hadPrev {
		return
	}
	switch event.Status {
	case config.StatusConnected:
		if prev.Status != config.StatusConnected {
			notify("Connected", fmt.Sprintf("Tunnel %q is now connected", event.TunnelID))
		}
	case config.StatusDisconnected:
		if prev.Status == config.StatusConnected {
			notify("Disconnected", fmt.Sprintf("Tunnel %q disconnected", event.TunnelID))
		}
	}
}

// Notify sends a desktop notification for an out-of-band event, such as a
// portless forward failing to bind a privileged port.
func (t *Tray) Notify(title, body string) {
	notify(title, body)
}

// SetUpdateAvailable stores the version and redraws the menu with an update item.
func (t *Tray) SetUpdateAvailable(version string) {
	t.mu.Lock()
	t.updateVersion = version
	t.mu.Unlock()
	t.rebuildMenu()
}

func (t *Tray) snapshot() (map[string]ssh.StatusEvent, string, Surface) {
	t.mu.Lock()
	defer t.mu.Unlock()
	statuses := make(map[string]ssh.StatusEvent, len(t.statuses))
	for k, v := range t.statuses {
		statuses[k] = v
	}
	return statuses, t.updateVersion, t.surface
}

func (t *Tray) updateIcon() {
	statuses, _, surface := t.snapshot()
	if surface == nil {
		return
	}
	var connected, errored int
	for _, s := range statuses {
		switch s.Status {
		case config.StatusConnected:
			connected++
		case config.StatusError:
			errored++
		}
	}
	sc := ColorGray
	switch {
	case errored > 0:
		sc = ColorRed
	case connected > 0:
		sc = ColorGreen
	}
	surface.SetIcon(GenerateIcon(sc, connected))
	surface.SetTooltip(Tooltip(connected, errored))
}

// Tooltip returns the tray tooltip for the given counts.
func Tooltip(connected, errored int) string {
	switch {
	case connected > 0 && errored > 0:
		return fmt.Sprintf("SSH Tunnel Manager: %d connected, %d errors", connected, errored)
	case connected > 0:
		return fmt.Sprintf("SSH Tunnel Manager: %d connected", connected)
	case errored > 0:
		return fmt.Sprintf("SSH Tunnel Manager: %d errors", errored)
	}
	return "SSH Tunnel Manager"
}

func (t *Tray) rebuildMenu() {
	// Status events come from independent tunnel goroutines, so rebuilds can
	// arrive concurrently and must not interleave.
	t.menuMu.Lock()
	defer t.menuMu.Unlock()

	statuses, updateVersion, surface := t.snapshot()
	if surface == nil {
		return
	}
	surface.SetMenu(t.BuildMenu(t.cb.GetTunnels(), statuses, updateVersion))
}

// BuildMenu returns the complete tray menu for the given state. Ungrouped
// tunnels come first, then one submenu per group in order of first appearance.
func (t *Tray) BuildMenu(tunnels []config.TunnelConfig, statuses map[string]ssh.StatusEvent, updateVersion string) []Node {
	var totalActive int
	for _, s := range statuses {
		if isActive(s.Status) {
			totalActive++
		}
	}
	header := "SSH Tunnel Manager"
	if totalActive > 0 {
		header = fmt.Sprintf("SSH Tunnel Manager: %d active", totalActive)
	}
	nodes := []Node{{Label: header, Disabled: true}, {Separator: true}}

	var groupOrder []string
	groups := make(map[string][]config.TunnelConfig)
	for _, tc := range tunnels {
		if tc.Group == "" {
			nodes = append(nodes, t.tunnelNode(tc, statuses[tc.ID]))
			continue
		}
		if _, seen := groups[tc.Group]; !seen {
			groupOrder = append(groupOrder, tc.Group)
		}
		groups[tc.Group] = append(groups[tc.Group], tc)
	}
	for _, g := range groupOrder {
		nodes = append(nodes, t.groupNode(g, groups[g], statuses))
	}
	if len(tunnels) == 0 {
		nodes = append(nodes, Node{Label: "No tunnels configured", Disabled: true})
	}
	nodes = append(nodes, Node{Separator: true})

	if updateVersion != "" {
		nodes = append(nodes,
			Node{
				Label:   "Update to v" + updateVersion,
				Tooltip: "Download and install the latest version",
				OnClick: func() {
					slog.Info("tray: user triggered update", "version", updateVersion)
					if t.cb.OnUpdate != nil {
						t.cb.OnUpdate()
					}
				},
			},
			Node{Separator: true},
		)
	}
	nodes = append(nodes,
		Node{Label: "Show Window", Tooltip: "Open the manager window", OnClick: func() {
			if t.cb.ShowWindow != nil {
				t.cb.ShowWindow()
			}
		}},
		Node{Label: "Quit", Tooltip: "Quit SSH Tunnel Manager", OnClick: func() {
			if t.cb.Quit != nil {
				t.cb.Quit()
			}
		}},
	)
	return nodes
}

func (t *Tray) groupNode(group string, members []config.TunnelConfig, statuses map[string]ssh.StatusEvent) Node {
	ids := make([]string, len(members))
	var connected int
	for i, tc := range members {
		ids[i] = tc.ID
		if s, ok := statuses[tc.ID]; ok && isActive(s.Status) {
			connected++
		}
	}
	each := func(verb string, action func(string) error) func() {
		return func() {
			slog.Info("tray: "+verb+" group", "group", group)
			for _, id := range ids {
				if err := action(id); err != nil {
					slog.Error("tray: "+verb+" failed", "tunnel", id, "error", err)
				}
			}
		}
	}
	var children []Node
	if connected < len(members) {
		children = append(children, Node{Label: "Connect All", OnClick: each("connecting", t.cb.Connect)})
	}
	if connected > 0 {
		children = append(children, Node{Label: "Disconnect All", OnClick: each("disconnecting", t.cb.Disconnect)})
	}
	for _, tc := range members {
		children = append(children, t.tunnelNode(tc, statuses[tc.ID]))
	}
	return Node{Label: fmt.Sprintf("%s (%d/%d)", group, connected, len(members)), Children: children}
}

// isActive returns true for connected, connecting and reconnecting.
func isActive(s config.TunnelStatus) bool {
	switch s {
	case config.StatusConnected, config.StatusConnecting, config.StatusReconnecting:
		return true
	}
	return false
}

func (t *Tray) tunnelNode(tc config.TunnelConfig, se ssh.StatusEvent) Node {
	status := se.Status
	if status == "" {
		status = config.StatusDisconnected
	}
	var tooltip, prefix string
	var icon []byte
	switch status {
	case config.StatusConnected:
		tooltip, prefix, icon = tc.Name+": connected", "● ", t.dots.green
	case config.StatusConnecting:
		tooltip, prefix, icon = tc.Name+": connecting", "◐ ", t.dots.blue
	case config.StatusReconnecting:
		tooltip, prefix, icon = tc.Name+": reconnecting", "◌ ", t.dots.orange
	case config.StatusError:
		errMsg := se.Error
		if errMsg == "" {
			errMsg = "unknown error"
		}
		tooltip, prefix, icon = tc.Name+": "+errMsg, "! ", t.dots.red
	default:
		tooltip, prefix, icon = tc.Name+": disconnected", "○ ", t.dots.gray
	}

	id, name := tc.ID, tc.Name
	var children []Node
	if isActive(status) {
		children = append(children, Node{Label: "Disconnect", OnClick: func() {
			slog.Info("tray: disconnecting tunnel", "tunnel", name)
			if err := t.cb.Disconnect(id); err != nil {
				slog.Error("tray: disconnect failed", "tunnel", name, "error", err)
			}
		}})
	} else {
		children = append(children, Node{Label: "Connect", OnClick: func() {
			slog.Info("tray: connecting tunnel", "tunnel", name)
			if err := t.cb.Connect(id); err != nil {
				slog.Error("tray: connect failed", "tunnel", name, "error", err)
			}
		}})
	}
	if attach := t.attachNode(id); attach != nil {
		children = append(children, *attach)
	}
	for _, pf := range tc.PortForwards {
		children = append(children, t.forwardNode(pf))
	}
	return Node{Label: prefix + tc.Name, Tooltip: tooltip, Icon: icon, Children: children}
}

// attachNode returns one Attach entry when there is a single option, and a
// submenu only when there are several.
func (t *Tray) attachNode(id string) *Node {
	if t.cb.AttachOptions == nil || t.cb.Attach == nil {
		return nil
	}
	options := t.cb.AttachOptions(id)
	click := func(option AttachOption) func() {
		return func() {
			if err := t.cb.Attach(id, option); err != nil {
				slog.Error("tray: attach failed", "tunnel", id, "error", err)
			}
		}
	}
	switch len(options) {
	case 0:
		return nil
	case 1:
		return &Node{Label: "Attach", Tooltip: options[0].Label, OnClick: click(options[0])}
	}
	node := &Node{Label: "Attach"}
	for _, option := range options {
		node.Children = append(node.Children, Node{Label: option.Label, OnClick: click(option)})
	}
	return node
}

func (t *Tray) forwardNode(pf config.PortForward) Node {
	clip := ForwardAddress(pf)
	label := clip
	if !pf.Portless || pf.Domain == "" {
		label = fmt.Sprintf("%d", pf.LocalPort)
	}
	if pf.Description != "" {
		label = fmt.Sprintf("%s (%s)", label, pf.Description)
	}
	return Node{Label: label, Tooltip: "Copy " + clip, OnClick: func() {
		if t.cb.CopyToClip == nil {
			return
		}
		if err := t.cb.CopyToClip(clip); err != nil {
			slog.Error("tray: clipboard copy failed", "error", err)
		}
	}}
}

// ForwardAddress returns the address a forward is reached at locally.
func ForwardAddress(pf config.PortForward) string {
	if !pf.Portless || pf.Domain == "" {
		return fmt.Sprintf("127.0.0.1:%d", pf.LocalPort)
	}
	port := pf.RemotePort
	if pf.ExposePort > 0 {
		port = pf.ExposePort
	}
	host := fmt.Sprintf("%s.%s", pf.Domain, dns.TLD)
	switch port {
	case 80:
		return "http://" + host
	case 443:
		return "https://" + host
	}
	return fmt.Sprintf("%s:%d", host, port)
}
