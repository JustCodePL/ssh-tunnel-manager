package windows

import "testing"

func spec(tool Tool, tunnel string) Spec { return Spec{Tool: tool, TunnelID: tunnel} }

func TestKeyPerGrouping(t *testing.T) {
	s := Spec{Tool: ToolRDP, TunnelID: "nas", RemoteHost: "192.168.1.20", RemotePort: 3389}
	cases := map[Grouping]string{
		GroupToolTarget: "rdp|nas>192.168.1.20:3389",
		GroupTool:       "rdp",
		GroupTarget:     "target|nas>192.168.1.20:3389",
	}
	for grouping, want := range cases {
		if got := Key(s, grouping); got != want {
			t.Errorf("Key(%s) = %q, want %q", grouping, got, want)
		}
	}
}

func TestSettingsNeverShareWindows(t *testing.T) {
	a := Key(spec(ToolTunnelSettings, "a"), GroupTool)
	b := Key(spec(ToolTunnelSettings, "b"), GroupTool)
	if a == b {
		t.Fatalf("tunnel settings share a window: %q", a)
	}
}

func TestServersBehindOneBastionGetOwnWindows(t *testing.T) {
	r := NewRegistry()
	a := r.Open(Spec{Tool: ToolRDP, TunnelID: "nas", RemoteHost: "10.0.0.1", RemotePort: 3389}, GroupToolTarget)
	b := r.Open(Spec{Tool: ToolRDP, TunnelID: "nas", RemoteHost: "10.0.0.2", RemotePort: 3389}, GroupToolTarget)
	if !a.Created || !b.Created || a.WindowName == b.WindowName {
		t.Fatalf("expected two new windows, got %+v and %+v", a, b)
	}
}

func TestTerminalAddsTabOtherToolsReturn(t *testing.T) {
	r := NewRegistry()
	first := r.Open(spec(ToolTerminal, "a"), GroupToolTarget)
	second := r.Open(spec(ToolTerminal, "a"), GroupToolTarget)
	if second.Created || second.WindowName != first.WindowName || second.TabID == first.TabID {
		t.Fatalf("second terminal = %+v, first = %+v", second, first)
	}
	w, _ := r.Get(first.WindowName)
	if len(w.Tabs) != 2 || w.Active != second.TabID {
		t.Fatalf("window = %+v", w)
	}

	docker := r.Open(spec(ToolDocker, "a"), GroupToolTarget)
	again := r.Open(spec(ToolDocker, "a"), GroupToolTarget)
	if again.Created || again.TabID != docker.TabID {
		t.Fatalf("docker reopened as %+v, was %+v", again, docker)
	}
}

func TestGroupToolPutsTargetsInTabs(t *testing.T) {
	r := NewRegistry()
	a := r.Open(spec(ToolDocker, "a"), GroupTool)
	b := r.Open(spec(ToolDocker, "b"), GroupTool)
	if b.Created || a.WindowName != b.WindowName {
		t.Fatalf("a=%+v b=%+v", a, b)
	}
}

func TestGroupTargetPutsToolsInTabs(t *testing.T) {
	r := NewRegistry()
	a := r.Open(spec(ToolDocker, "a"), GroupTarget)
	b := r.Open(spec(ToolHtop, "a"), GroupTarget)
	c := r.Open(spec(ToolHtop, "b"), GroupTarget)
	if b.Created || a.WindowName != b.WindowName || !c.Created {
		t.Fatalf("a=%+v b=%+v c=%+v", a, b, c)
	}
}

func TestCloseTabReturnsSessionsAndMovesActive(t *testing.T) {
	r := NewRegistry()
	a := r.Open(spec(ToolTerminal, "x"), GroupToolTarget)
	b := r.Open(spec(ToolTerminal, "x"), GroupToolTarget)
	r.BindSession(b.TabID, Session{Kind: "terminal", ID: "s1"})

	sessions, empty := r.CloseTab(b.WindowName, b.TabID)
	if empty || len(sessions) != 1 || sessions[0].ID != "s1" {
		t.Fatalf("sessions=%v empty=%v", sessions, empty)
	}
	w, _ := r.Get(a.WindowName)
	if w.Active != a.TabID {
		t.Fatalf("active = %q, want %q", w.Active, a.TabID)
	}

	if _, empty := r.CloseTab(a.WindowName, a.TabID); !empty {
		t.Fatal("window should be empty")
	}
	if again := r.Open(spec(ToolTerminal, "x"), GroupToolTarget); !again.Created {
		t.Fatal("the key must be free after the last tab closed")
	}
}

func TestRemoveWindowReturnsAllSessions(t *testing.T) {
	r := NewRegistry()
	a := r.Open(spec(ToolTerminal, "x"), GroupToolTarget)
	b := r.Open(spec(ToolTerminal, "x"), GroupToolTarget)
	r.BindSession(a.TabID, Session{"terminal", "1"})
	r.BindSession(b.TabID, Session{"terminal", "2"})

	if name, ok := r.WindowOfSession("2"); !ok || name != a.WindowName {
		t.Fatalf("WindowOfSession = %q %v", name, ok)
	}
	key, sessions := r.RemoveWindow(a.WindowName)
	if key != "terminal|x" || len(sessions) != 2 {
		t.Fatalf("key=%q sessions=%v", key, sessions)
	}
	if _, ok := r.WindowOfSession("2"); ok {
		t.Fatal("session of a removed window still resolves")
	}
}

func TestUnbindSession(t *testing.T) {
	r := NewRegistry()
	a := r.Open(spec(ToolTerminal, "x"), GroupToolTarget)
	r.BindSession(a.TabID, Session{"terminal", "1"})
	r.UnbindSession("1")
	if _, sessions := r.RemoveWindow(a.WindowName); len(sessions) != 0 {
		t.Fatalf("sessions = %v", sessions)
	}
}

func TestTabsOfTunnelSkipsSettings(t *testing.T) {
	r := NewRegistry()
	r.Open(spec(ToolTunnelSettings, "x"), GroupToolTarget)
	files := r.Open(Spec{Tool: ToolFiles, TunnelID: "x", RemoteHost: "h", RemotePort: 22}, GroupToolTarget)
	r.Open(spec(ToolFiles, "y"), GroupToolTarget)
	tabs := r.TabsOfTunnel("x")
	if len(tabs) != 1 || tabs[0].TabID != files.TabID {
		t.Fatalf("tabs = %+v", tabs)
	}
}

func TestRenameTunnelKeepsWindowFindable(t *testing.T) {
	r := NewRegistry()
	a := r.Open(spec(ToolDocker, "old"), GroupToolTarget)
	r.RenameTunnel("old", "new", GroupToolTarget)
	again := r.Open(spec(ToolDocker, "new"), GroupToolTarget)
	if again.Created || again.WindowName != a.WindowName {
		t.Fatalf("again = %+v", again)
	}
}

func TestVisibleOn(t *testing.T) {
	screen := []Rect{{0, 0, 1000, 800}}
	cases := []struct {
		r    Rect
		want bool
	}{
		{Rect{100, 100, 400, 300}, true},
		{Rect{950, 100, 400, 300}, false},
		{Rect{2000, 100, 400, 300}, false},
		{Rect{100, 900, 400, 300}, false},
		{Rect{100, 100, 0, 0}, false},
	}
	for _, c := range cases {
		if got := c.r.VisibleOn(screen); got != c.want {
			t.Errorf("%+v.VisibleOn = %v, want %v", c.r, got, c.want)
		}
	}
}
