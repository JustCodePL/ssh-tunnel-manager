package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRemoteDesktopCRUD(t *testing.T) {
	path := tempSSHConfigPath(t)
	extra := filepath.Join(filepath.Dir(path), "work.conf")
	writeSSHConfig(t, path, "Include work.conf\n\nHost jump\n    HostName jump.example\n")
	writeSSHConfig(t, extra, "Host office\n    HostName office.example\n")
	s, err := NewStoreWithPath(path)
	if err != nil {
		t.Fatal(err)
	}

	laptop := RemoteDesktop{Name: "laptop", Address: "10.0.0.5", Port: 3389, Protocol: "rdp", Group: "Office"}
	if err := s.AddRemoteDesktop(laptop); err != nil {
		t.Fatal(err)
	}
	mac := RemoteDesktop{Name: "mac", Address: "mac.local", Port: 5900, Protocol: "vnc", SourceFile: extra}
	if err := s.AddRemoteDesktop(mac); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"jump", "office", "laptop"} {
		if err := s.AddRemoteDesktop(RemoteDesktop{Name: name, Address: "x", Port: 1, Protocol: "rdp"}); err == nil {
			t.Errorf("duplicate name %q accepted", name)
		}
	}
	if err := s.AddTunnel(TunnelConfig{ID: "mac", Name: "mac", Host: "h"}); err == nil {
		t.Error("a host took a remote desktop's name")
	}

	// A fresh store reads both back, each from its own file.
	reloaded, err := NewStoreWithPath(path)
	if err != nil {
		t.Fatal(err)
	}
	got := reloaded.GetRemoteDesktops()
	if len(got) != 2 || len(reloaded.GetTunnels()) != 2 {
		t.Fatalf("desktops = %+v, tunnels = %+v", got, reloaded.GetTunnels())
	}
	byName := map[string]RemoteDesktop{}
	for _, d := range got {
		byName[d.Name] = d
	}
	if byName["laptop"].SourceFile != path || byName["mac"].SourceFile != extra || byName["laptop"].ID != "rd:laptop" || byName["laptop"].Group != "Office" {
		t.Fatalf("desktops = %+v", byName)
	}

	// Renaming rewrites the entry in its own file.
	renamed := byName["mac"]
	renamed.Name = "studio"
	renamed.SourceFile = path // ignored: an entry stays in its file
	if err := reloaded.UpdateRemoteDesktop("mac", renamed); err != nil {
		t.Fatal(err)
	}
	if err := reloaded.UpdateRemoteDesktop("studio", RemoteDesktop{Name: "jump", Address: "x", Port: 1, Protocol: "vnc"}); err == nil {
		t.Error("rename onto a host accepted")
	}
	data, _ := os.ReadFile(extra)
	if !strings.Contains(string(data), "# stm:rd=studio address=mac.local port=5900 protocol=vnc") || strings.Contains(string(data), "rd=mac ") {
		t.Fatalf("work.conf = %s", data)
	}
	if d, ok := reloaded.GetRemoteDesktop("studio"); !ok || d.ID != "rd:studio" || d.SourceFile != extra {
		t.Fatalf("studio = %+v %v", d, ok)
	}

	// Deleting the last entry of a file removes its section; hosts stay.
	if err := reloaded.DeleteRemoteDesktop("studio"); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(extra)
	if string(data) != "Host office\n    HostName office.example\n" {
		t.Fatalf("work.conf after delete = %q", data)
	}
	if err := reloaded.DeleteRemoteDesktop("studio"); err == nil {
		t.Error("deleting a missing entry succeeded")
	}
}

func TestRemoteDesktopKeepsEntriesAddedByHand(t *testing.T) {
	path := tempSSHConfigPath(t)
	s, err := NewStoreWithPath(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AddRemoteDesktop(RemoteDesktop{Name: "a", Address: "a.lan", Port: 3389, Protocol: "rdp"}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	edited := strings.Replace(string(data), "# stm:remote-desktops-end", "# stm:rd=hand address=h.lan protocol=vnc\n# stm:remote-desktops-end", 1)
	writeSSHConfig(t, path, edited)
	if err := s.AddRemoteDesktop(RemoteDesktop{Name: "b", Address: "b.lan", Port: 3389, Protocol: "rdp"}); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(path)
	if !strings.Contains(string(data), "rd=hand") || !strings.Contains(string(data), "rd=b ") {
		t.Fatalf("config = %s", data)
	}
}

func TestValidateRemoteDesktop(t *testing.T) {
	good := RemoteDesktop{Name: "work laptop", Address: "fd00::5", Port: 3389, Protocol: "rdp", Group: "Office team"}
	if err := ValidateRemoteDesktop(good); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*RemoteDesktop){
		func(d *RemoteDesktop) { d.Name = "" },
		func(d *RemoteDesktop) { d.Name = " padded" },
		func(d *RemoteDesktop) { d.Name = "rd:x" },
		func(d *RemoteDesktop) { d.Name = "a\nHost evil" },
		func(d *RemoteDesktop) { d.Address = "" },
		func(d *RemoteDesktop) { d.Address = "a b" },
		func(d *RemoteDesktop) { d.Address = "laptop.lan:3390" },
		func(d *RemoteDesktop) { d.Protocol = "ssh" },
		func(d *RemoteDesktop) { d.Port = 0 },
		func(d *RemoteDesktop) { d.Port = 70000 },
		func(d *RemoteDesktop) { d.Group = "x\r" },
	} {
		d := good
		change(&d)
		if err := ValidateRemoteDesktop(d); err == nil {
			t.Errorf("%+v accepted", d)
		}
	}
}

func TestLoadSkipsDuplicateAndInvalidDesktops(t *testing.T) {
	path := tempSSHConfigPath(t)
	writeSSHConfig(t, path, "# stm:remote-desktops-begin\n# stm:rd=pc address=a.lan protocol=rdp\n# stm:rd=pc address=b.lan protocol=vnc\n# stm:rd=jump address=c.lan protocol=rdp\n# stm:rd=v6 address=[fd00::5] protocol=vnc\n# stm:rd=bad address=host:3390 protocol=rdp\n# stm:remote-desktops-end\n\nHost jump\n")
	s, err := NewStoreWithPath(path)
	if err != nil {
		t.Fatal(err)
	}
	got := s.GetRemoteDesktops()
	if len(got) != 2 || got[0].Name != "pc" || got[0].Address != "a.lan" || got[1].Address != "fd00::5" {
		t.Fatalf("desktops = %+v", got)
	}
	// A name already in the file is never overwritten by an add.
	writeSSHConfig(t, path, "# stm:remote-desktops-begin\n# stm:rd=hand address=h.lan protocol=rdp\n# stm:remote-desktops-end\n")
	if err := s.AddRemoteDesktop(RemoteDesktop{Name: "hand", Address: "x", Port: 1, Protocol: "rdp"}); err == nil {
		t.Fatal("an entry added by hand was overwritten")
	}
}
