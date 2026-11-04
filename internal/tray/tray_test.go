package tray

import (
	"strings"
	"testing"

	"ssh-tunnel-manager/internal/config"
	"ssh-tunnel-manager/internal/ssh"
)

func labels(nodes []Node) []string {
	out := make([]string, 0, len(nodes))
	for _, n := range nodes {
		if !n.Separator {
			out = append(out, n.Label)
		}
	}
	return out
}

func find(t *testing.T, nodes []Node, label string) Node {
	t.Helper()
	for _, n := range nodes {
		if n.Label == label {
			return n
		}
	}
	t.Fatalf("no node %q in %v", label, labels(nodes))
	return Node{}
}

func TestBuildMenuGroupsAndOrder(t *testing.T) {
	tr := New(Callbacks{})
	nodes := tr.BuildMenu([]config.TunnelConfig{
		{ID: "a", Name: "a", Group: "Mikrus"},
		{ID: "b", Name: "b"},
		{ID: "c", Name: "c", Group: "Mikrus"},
	}, map[string]ssh.StatusEvent{"a": {Status: config.StatusConnected}}, "")

	got := strings.Join(labels(nodes), "|")
	want := "SSH Tunnel Manager: 1 active|○ b|Mikrus (1/2)|Show Window|Quit"
	if got != want {
		t.Fatalf("menu = %q, want %q", got, want)
	}
	group := find(t, nodes, "Mikrus (1/2)")
	if strings.Join(labels(group.Children), "|") != "Connect All|Disconnect All|● a|○ c" {
		t.Fatalf("group children = %v", labels(group.Children))
	}
}

func TestBuildMenuMarksFailedTunnel(t *testing.T) {
	tr := New(Callbacks{})
	nodes := tr.BuildMenu([]config.TunnelConfig{{ID: "a", Name: "a"}},
		map[string]ssh.StatusEvent{"a": {Status: config.StatusError, Error: "auth failed"}}, "")
	node := find(t, nodes, "! a")
	if node.Tooltip != "a: auth failed" {
		t.Fatalf("tooltip = %q", node.Tooltip)
	}
	if node.Children[0].Label != "Connect" {
		t.Fatalf("a failed tunnel offers %q", node.Children[0].Label)
	}
}

func TestAttachExpandsOnlyWithSeveralOptions(t *testing.T) {
	var attached []string
	options := map[string][]AttachOption{
		"one":  {{Protocol: "ssh", Label: "Terminal"}},
		"two":  {{Protocol: "ssh", Label: "Terminal"}, {Protocol: "rdp", Label: "Remote desktop"}},
		"none": nil,
	}
	tr := New(Callbacks{
		AttachOptions: func(id string) []AttachOption { return options[id] },
		Attach: func(id string, o AttachOption) error {
			attached = append(attached, id+":"+o.Protocol)
			return nil
		},
	})

	one := tr.attachNode("one")
	if len(one.Children) != 0 || one.OnClick == nil {
		t.Fatalf("single option must be a direct action: %+v", one)
	}
	one.OnClick()

	two := tr.attachNode("two")
	if strings.Join(labels(two.Children), "|") != "Terminal|Remote desktop" {
		t.Fatalf("children = %v", labels(two.Children))
	}
	two.Children[1].OnClick()

	if tr.attachNode("none") != nil {
		t.Fatal("no options must hide Attach")
	}
	if strings.Join(attached, ",") != "one:ssh,two:rdp" {
		t.Fatalf("attached = %v", attached)
	}
}

func TestUpdateItemAppearsWithVersion(t *testing.T) {
	called := false
	tr := New(Callbacks{OnUpdate: func() { called = true }})
	nodes := tr.BuildMenu(nil, nil, "1.2.0")
	find(t, nodes, "Update to v1.2.0").OnClick()
	if !called {
		t.Fatal("update item did not call OnUpdate")
	}
	find(t, nodes, "No tunnels configured")
}

func TestForwardAddress(t *testing.T) {
	cases := []struct {
		pf   config.PortForward
		want string
	}{
		{config.PortForward{LocalPort: 2239}, "127.0.0.1:2239"},
		{config.PortForward{Portless: true, Domain: "db.x", RemotePort: 5432}, "db.x.ssh-local:5432"},
		{config.PortForward{Portless: true, Domain: "web", RemotePort: 8080, ExposePort: 80}, "http://web.ssh-local"},
		{config.PortForward{Portless: true, Domain: "web", RemotePort: 8443, ExposePort: 443}, "https://web.ssh-local"},
	}
	for _, c := range cases {
		if got := ForwardAddress(c.pf); got != c.want {
			t.Errorf("ForwardAddress(%+v) = %q, want %q", c.pf, got, c.want)
		}
	}
}

type fakeSurface struct {
	menus    int
	icons    int
	tooltip  string
	nodes    []Node
	destroys int
}

func (f *fakeSurface) SetMenu(n []Node)      { f.menus++; f.nodes = n }
func (f *fakeSurface) SetIcon([]byte)        { f.icons++ }
func (f *fakeSurface) SetTooltip(tip string) { f.tooltip = tip }
func (f *fakeSurface) Destroy()              { f.destroys++ }

func TestStatusEventRedrawsSurface(t *testing.T) {
	tr := New(Callbacks{GetTunnels: func() []config.TunnelConfig {
		return []config.TunnelConfig{{ID: "a", Name: "a"}}
	}})
	s := &fakeSurface{}
	tr.Start(s)
	tr.HandleStatusEvent(ssh.StatusEvent{TunnelID: "a", Status: config.StatusError, Error: "x"})
	if s.tooltip != "SSH Tunnel Manager: 1 errors" {
		t.Fatalf("tooltip = %q", s.tooltip)
	}
	find(t, s.nodes, "! a")
	tr.Stop()
	tr.Stop()
	if s.destroys != 1 {
		t.Fatalf("destroys = %d", s.destroys)
	}
}
