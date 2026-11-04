// Package windows tracks which tools are open in which native window. It holds
// no UI code: the app layer creates and closes the real windows and asks the
// registry where a tool belongs.
package windows

import (
	"fmt"
	"strconv"
	"sync"
)

// Tool identifies what a tab shows.
type Tool string

// Tools that open in their own windows.
const (
	ToolTerminal       Tool = "terminal"
	ToolRDP            Tool = "rdp"
	ToolFiles          Tool = "files"
	ToolEditor         Tool = "editor"
	ToolDocker         Tool = "docker"
	ToolHtop           Tool = "htop"
	ToolDisk           Tool = "disk"
	ToolLogs           Tool = "logs"
	ToolSettings       Tool = "settings"
	ToolTunnelSettings Tool = "tunnel-settings"
)

// Grouping decides which tabs share a window.
type Grouping string

// Grouping modes.
const (
	// GroupToolTarget gives every tool on every target its own window, with tabs
	// for several sessions of it. This is the default.
	GroupToolTarget Grouping = "tool-target"
	// GroupTool gives every tool one window, with a tab per target.
	GroupTool Grouping = "tool"
	// GroupTarget gives every target one window, with a tab per tool.
	GroupTarget Grouping = "target"
)

// ValidGrouping reports whether g is a known grouping mode.
func ValidGrouping(g Grouping) bool {
	return g == GroupToolTarget || g == GroupTool || g == GroupTarget
}

// Spec describes one tool on one target. The target is a tunnel, or a forward
// of that tunnel when RemoteHost is set.
type Spec struct {
	Tool       Tool   `json:"tool"`
	TunnelID   string `json:"tunnelId"`
	RemoteHost string `json:"remoteHost,omitempty"`
	RemotePort int    `json:"remotePort,omitempty"`
	// Path is the remote file an editor tab has open.
	Path string `json:"path,omitempty"`
}

// Target returns the identity of the machine the spec points at.
func (s Spec) Target() string {
	if s.RemoteHost == "" {
		return s.TunnelID
	}
	return s.TunnelID + ">" + s.RemoteHost + ":" + strconv.Itoa(s.RemotePort)
}

// standalone reports tools that never share a window with others.
func (s Spec) standalone() bool {
	return s.Tool == ToolSettings || s.Tool == ToolTunnelSettings
}

// Key returns the window a spec belongs to under the given grouping.
func Key(s Spec, grouping Grouping) string {
	if s.standalone() {
		return string(s.Tool) + "|" + s.Target()
	}
	switch grouping {
	case GroupTool:
		return string(s.Tool)
	case GroupTarget:
		return "target|" + s.Target()
	default:
		return string(s.Tool) + "|" + s.Target()
	}
}

// multiSession reports tools where each Attach starts another session instead
// of returning to the open one.
func (s Spec) multiSession() bool { return s.Tool == ToolTerminal }

// Session is a backend resource owned by a tab.
type Session struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

// Tab is one tool instance in a window.
type Tab struct {
	ID       string    `json:"id"`
	Spec     Spec      `json:"spec"`
	Sessions []Session `json:"-"`
}

// Window is a snapshot of one tool window.
type Window struct {
	Name   string `json:"name"`
	Key    string `json:"key"`
	Active string `json:"active"`
	Tabs   []Tab  `json:"tabs"`
}

// Opened tells the app layer what Open did.
type Opened struct {
	WindowName string
	TabID      string
	Created    bool
}

// Registry is the set of open tool windows. It is safe for concurrent use.
type Registry struct {
	mu      sync.Mutex
	windows map[string]*Window
	byKey   map[string]string
	nextID  int
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{windows: make(map[string]*Window), byKey: make(map[string]string)}
}

// Open places spec into its window. Tools with one session per target return to
// their open tab; a terminal always adds a tab.
func (r *Registry) Open(spec Spec, grouping Grouping) Opened {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := Key(spec, grouping)
	if name, ok := r.byKey[key]; ok {
		w := r.windows[name]
		if !spec.multiSession() {
			for _, tab := range w.Tabs {
				if tab.Spec == spec {
					w.Active = tab.ID
					return Opened{WindowName: name, TabID: tab.ID}
				}
			}
		}
		tab := r.newTab(spec)
		w.Tabs = append(w.Tabs, tab)
		w.Active = tab.ID
		return Opened{WindowName: name, TabID: tab.ID}
	}

	r.nextID++
	name := fmt.Sprintf("tool-%d", r.nextID)
	tab := r.newTab(spec)
	r.windows[name] = &Window{Name: name, Key: key, Active: tab.ID, Tabs: []Tab{tab}}
	r.byKey[key] = name
	return Opened{WindowName: name, TabID: tab.ID, Created: true}
}

func (r *Registry) newTab(spec Spec) Tab {
	r.nextID++
	return Tab{ID: fmt.Sprintf("tab-%d", r.nextID), Spec: spec}
}

// Get returns a copy of a window.
func (r *Registry) Get(name string) (Window, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	w, ok := r.windows[name]
	if !ok {
		return Window{}, false
	}
	return copyWindow(w), true
}

// All returns copies of every window.
func (r *Registry) All() []Window {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Window, 0, len(r.windows))
	for _, w := range r.windows {
		out = append(out, copyWindow(w))
	}
	return out
}

func copyWindow(w *Window) Window {
	c := *w
	c.Tabs = make([]Tab, len(w.Tabs))
	for i, tab := range w.Tabs {
		c.Tabs[i] = tab
		c.Tabs[i].Sessions = append([]Session(nil), tab.Sessions...)
	}
	return c
}

// Activate selects a tab.
func (r *Registry) Activate(windowName, tabID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	w, ok := r.windows[windowName]
	if !ok {
		return false
	}
	for _, tab := range w.Tabs {
		if tab.ID == tabID {
			w.Active = tabID
			return true
		}
	}
	return false
}

// CloseTab removes a tab and returns its sessions. empty is true when the
// window has no tabs left and has been forgotten.
func (r *Registry) CloseTab(windowName, tabID string) (sessions []Session, empty bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	w, ok := r.windows[windowName]
	if !ok {
		return nil, false
	}
	for i, tab := range w.Tabs {
		if tab.ID != tabID {
			continue
		}
		sessions = tab.Sessions
		w.Tabs = append(w.Tabs[:i], w.Tabs[i+1:]...)
		if w.Active == tabID && len(w.Tabs) > 0 {
			next := i
			if next >= len(w.Tabs) {
				next = len(w.Tabs) - 1
			}
			w.Active = w.Tabs[next].ID
		}
		break
	}
	if len(w.Tabs) == 0 {
		delete(r.windows, windowName)
		delete(r.byKey, w.Key)
		return sessions, true
	}
	return sessions, false
}

// RemoveWindow forgets a window and returns the sessions of all its tabs.
func (r *Registry) RemoveWindow(name string) (key string, sessions []Session) {
	r.mu.Lock()
	defer r.mu.Unlock()
	w, ok := r.windows[name]
	if !ok {
		return "", nil
	}
	for _, tab := range w.Tabs {
		sessions = append(sessions, tab.Sessions...)
	}
	delete(r.windows, name)
	delete(r.byKey, w.Key)
	return w.Key, sessions
}

// BindSession records a backend session as owned by a tab, replacing nothing:
// a tab can own several, for example after reconnecting.
func (r *Registry) BindSession(tabID string, session Session) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, w := range r.windows {
		for i := range w.Tabs {
			if w.Tabs[i].ID == tabID {
				w.Tabs[i].Sessions = append(w.Tabs[i].Sessions, session)
				return true
			}
		}
	}
	return false
}

// UnbindSession forgets a session that ended on its own.
func (r *Registry) UnbindSession(sessionID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, w := range r.windows {
		for i := range w.Tabs {
			kept := w.Tabs[i].Sessions[:0]
			for _, s := range w.Tabs[i].Sessions {
				if s.ID != sessionID {
					kept = append(kept, s)
				}
			}
			w.Tabs[i].Sessions = kept
		}
	}
}

// WindowOfSession returns the window whose tab owns a session.
func (r *Registry) WindowOfSession(sessionID string) (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for name, w := range r.windows {
		for _, tab := range w.Tabs {
			for _, s := range tab.Sessions {
				if s.ID == sessionID {
					return name, true
				}
			}
		}
	}
	return "", false
}

// TabRef points at one tab of one window.
type TabRef struct {
	WindowName string
	TabID      string
}

// TabsOfTunnel returns the tabs working on a tunnel or on its forwards. Settings
// tabs are left out: they do not depend on the connection.
func (r *Registry) TabsOfTunnel(tunnelID string) []TabRef {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []TabRef
	for name, w := range r.windows {
		for _, tab := range w.Tabs {
			if tab.Spec.TunnelID == tunnelID && !tab.Spec.standalone() {
				out = append(out, TabRef{WindowName: name, TabID: tab.ID})
			}
		}
	}
	return out
}

// RenameTunnel updates open tabs after a tunnel got a new ID.
func (r *Registry) RenameTunnel(oldID, newID string, grouping Grouping) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, w := range r.windows {
		changed := false
		for i := range w.Tabs {
			if w.Tabs[i].Spec.TunnelID == oldID {
				w.Tabs[i].Spec.TunnelID = newID
				changed = true
			}
		}
		if changed {
			delete(r.byKey, w.Key)
			w.Key = Key(w.Tabs[0].Spec, grouping)
			r.byKey[w.Key] = w.Name
		}
	}
}

// Rect is a window's position and size in screen coordinates.
type Rect struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

// VisibleOn reports whether enough of r is on one of the screens to grab its
// title bar, so a window saved on a disconnected monitor is not restored
// off-screen.
func (r Rect) VisibleOn(screens []Rect) bool {
	if r.Width <= 0 || r.Height <= 0 {
		return false
	}
	const minVisible = 80
	for _, s := range screens {
		left := max(r.X, s.X)
		right := min(r.X+r.Width, s.X+s.Width)
		if right-left >= minVisible && r.Y >= s.Y && r.Y < s.Y+s.Height-20 {
			return true
		}
	}
	return false
}
