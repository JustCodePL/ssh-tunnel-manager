package main

import (
	"slices"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"

	"ssh-tunnel-manager/internal/config"
	"ssh-tunnel-manager/internal/keychain"
	"ssh-tunnel-manager/internal/prefs"
	"ssh-tunnel-manager/internal/windows"
)

func newTestApp(t *testing.T, tunnels ...config.TunnelConfig) *App {
	t.Helper()
	keyring.MockInit()
	dir := t.TempDir()
	prefsStore, err := prefs.NewStoreWithPath(filepath.Join(dir, "prefs.json"))
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", dir)
	store, err := config.NewStore()
	if err != nil {
		t.Fatal(err)
	}
	app := NewApp(store, prefsStore, false)
	for _, tunnel := range tunnels {
		if err := app.AddTunnel(tunnel); err != nil {
			t.Fatal(err)
		}
	}
	return app
}

func demoTunnel(name, group string, forwards ...config.PortForward) config.TunnelConfig {
	return config.TunnelConfig{Name: name, Host: "127.0.0.1", Port: 22, User: "demo", Group: group, PortForwards: forwards}
}

func rdpForward(host string, local int) config.PortForward {
	return config.PortForward{LocalPort: local, RemoteHost: host, RemotePort: 3389}
}

func TestDefaultsMatchTheAgreedDesign(t *testing.T) {
	app := newTestApp(t)
	s := app.GetAppSettings()
	if s.Theme != prefs.ThemeTerminal {
		t.Errorf("theme = %q", s.Theme)
	}
	if s.WindowGrouping != string(windows.GroupToolTarget) || s.AfterDisconnect != prefs.AfterDisconnectResume || s.HideToolsWithMain {
		t.Errorf("window defaults = %+v", s)
	}
	if s.Refresh["docker"] != 0 || s.Refresh["htop"] != 3 || s.Refresh["disk"] != 5 || s.Refresh["stats"] != 3 {
		t.Errorf("refresh = %v", s.Refresh)
	}
	if s.Row["attach"] != prefs.RowInRow || s.Row["forwards"] != prefs.RowInRow || s.Row["stats"] != prefs.RowInPanel {
		t.Errorf("row = %v", s.Row)
	}
}

func TestSetAppSettingsValidates(t *testing.T) {
	app := newTestApp(t)
	good := app.GetAppSettings()
	cases := map[string]func(*AppSettings){
		"theme":    func(s *AppSettings) { s.Theme = "neon" },
		"grouping": func(s *AppSettings) { s.WindowGrouping = "random" },
		"after":    func(s *AppSettings) { s.AfterDisconnect = "explode" },
		"monitor":  func(s *AppSettings) { s.Refresh = map[string]int{"gpu": 3} },
		"negative": func(s *AppSettings) { s.Refresh = map[string]int{"htop": -1} },
		"module":   func(s *AppSettings) { s.Row = map[string]string{"nope": "row"} },
		"place":    func(s *AppSettings) { s.Row = map[string]string{"files": "sideways"} },
	}
	for name, change := range cases {
		bad := good
		change(&bad)
		if err := app.SetAppSettings(bad); err == nil {
			t.Errorf("%s: invalid settings were accepted", name)
		}
	}
	good.Theme = prefs.ThemeGraphite
	if err := app.SetAppSettings(good); err != nil {
		t.Fatal(err)
	}
	if app.GetAppSettings().Theme != prefs.ThemeGraphite {
		t.Fatal("the theme was not saved")
	}
}

func TestTunnelOverridesWinAndInheritTheRest(t *testing.T) {
	app := newTestApp(t, demoTunnel("a", ""))
	s := app.GetAppSettings()
	s.Refresh = map[string]int{"htop": 10}
	s.AfterDisconnect = prefs.AfterDisconnectKeep
	if err := app.SetAppSettings(s); err != nil {
		t.Fatal(err)
	}
	if err := app.SetTunnelSettings("a", prefs.TunnelSettings{
		Refresh: map[string]int{"disk": 60}, AfterDisconnect: prefs.AfterDisconnectClose, Clipboard: prefs.ClipboardOff,
		Row: map[string]string{"files": prefs.RowInRow},
	}); err != nil {
		t.Fatal(err)
	}
	got := app.GetEffectiveSettings()["a"]
	if got.Refresh["htop"] != 10 || got.Refresh["disk"] != 60 || got.Refresh["docker"] != 0 {
		t.Errorf("refresh = %v", got.Refresh)
	}
	if got.AfterDisconnect != prefs.AfterDisconnectClose || got.Clipboard != prefs.ClipboardOff {
		t.Errorf("effective = %+v", got)
	}
	if got.Row["files"] != prefs.RowInRow || got.Row["attach"] != prefs.RowInRow {
		t.Errorf("row = %v", got.Row)
	}
	if err := app.SetTunnelSettings("a", prefs.TunnelSettings{Clipboard: "sideways"}); err == nil {
		t.Error("an unknown clipboard mode was accepted")
	}
	if err := app.SetTunnelSettings("missing", prefs.TunnelSettings{}); err == nil {
		t.Error("settings for a missing tunnel were accepted")
	}
}

func TestApplyRowToGroupCopiesOnce(t *testing.T) {
	app := newTestApp(t, demoTunnel("a", "G"), demoTunnel("b", "G"), demoTunnel("c", "Other"))
	if err := app.SetTunnelSettings("a", prefs.TunnelSettings{Row: map[string]string{"logs": prefs.RowInRow}}); err != nil {
		t.Fatal(err)
	}
	count, err := app.ApplyRowToGroup("a")
	if err != nil || count != 1 {
		t.Fatalf("count=%d err=%v", count, err)
	}
	if app.GetEffectiveSettings()["b"].Row["logs"] != prefs.RowInRow {
		t.Error("the group member did not get the row")
	}
	if app.GetEffectiveSettings()["c"].Row["logs"] == prefs.RowInRow {
		t.Error("another group changed")
	}
	if err := app.SetTunnelSettings("a", prefs.TunnelSettings{}); err != nil {
		t.Fatal(err)
	}
	if app.GetEffectiveSettings()["b"].Row["logs"] != prefs.RowInRow {
		t.Error("a later change followed the copy")
	}
	lone := newTestApp(t, demoTunnel("x", ""))
	if _, err := lone.ApplyRowToGroup("x"); err == nil {
		t.Error("a tunnel without a group has nothing to apply to")
	}
}

func TestCredentialsKeepSecretsInTheKeychain(t *testing.T) {
	app := newTestApp(t)
	if _, err := app.SaveCredential(CredentialInput{Name: "", Password: "x", Protocols: []string{"rdp"}}); err == nil {
		t.Error("a login without a name was accepted")
	}
	if _, err := app.SaveCredential(CredentialInput{Name: "x", Protocols: []string{"rdp"}}); err == nil {
		t.Error("a new login without a password was accepted")
	}
	if _, err := app.SaveCredential(CredentialInput{Name: "x", Password: "p"}); err == nil {
		t.Error("a login without a protocol was accepted")
	}
	if _, err := app.SaveCredential(CredentialInput{Name: "x", Password: "p", Protocols: []string{"ftp"}}); err == nil {
		t.Error("an unknown protocol was accepted")
	}

	saved, err := app.SaveCredential(CredentialInput{
		Name: "Office", Username: "admin", Domain: "CORP", Password: "s3cret", Protocols: []string{"rdp", "vnc"},
	})
	if err != nil {
		t.Fatal(err)
	}
	login, found, err := keychain.GetLogin(saved.ID)
	if err != nil || !found || login.Password != "s3cret" || login.Username != "admin" || login.Domain != "CORP" {
		t.Fatalf("keychain login = %+v found=%v err=%v", login, found, err)
	}

	// Only names and protocols reach the prefs file.
	data := readFile(t, filepath.Join(filepath.Dir(prefsPath(t, app)), "prefs.json"))
	for _, secret := range []string{"s3cret", "admin", "CORP"} {
		if strings.Contains(data, secret) {
			t.Errorf("prefs.json contains %q", secret)
		}
	}
	if !strings.Contains(data, "Office") {
		t.Error("prefs.json should list the login's name")
	}

	// An empty password keeps the stored one.
	if _, err := app.SaveCredential(CredentialInput{ID: saved.ID, Name: "Office 2", Username: "root", Protocols: []string{"rdp"}}); err != nil {
		t.Fatal(err)
	}
	login, _, _ = keychain.GetLogin(saved.ID)
	if login.Password != "s3cret" || login.Username != "root" {
		t.Fatalf("after edit: %+v", login)
	}

	views, err := app.ListCredentials()
	if err != nil || len(views) != 1 || views[0].Name != "Office 2" || !views[0].HasPassword {
		t.Fatalf("views = %+v err=%v", views, err)
	}
	if _, err := app.SaveCredential(CredentialInput{ID: "missing", Name: "x", Protocols: []string{"rdp"}}); err == nil {
		t.Error("editing a missing login was accepted")
	}
}

func TestEachServerBehindABastionHasItsOwnLogin(t *testing.T) {
	app := newTestApp(t, demoTunnel("nas", "", rdpForward("10.0.0.1", 13389), rdpForward("10.0.0.2", 13390)))
	a := ForwardTarget{RemoteHost: "10.0.0.1", RemotePort: 3389}
	b := ForwardTarget{RemoteHost: "10.0.0.2", RemotePort: 3389}

	if err := app.SaveRDPLogin("nas", a, RDPLogin{Username: "first", Password: "one"}); err != nil {
		t.Fatal(err)
	}
	if err := app.SaveRDPLogin("nas", b, RDPLogin{Username: "second", Password: "two"}); err != nil {
		t.Fatal(err)
	}
	gotA, err := app.GetRDPLogin("nas", a)
	if err != nil || gotA.Username != "first" || !gotA.HasPassword {
		t.Fatalf("a = %+v err=%v", gotA, err)
	}
	gotB, _ := app.GetRDPLogin("nas", b)
	if gotB.Username != "second" {
		t.Fatalf("b = %+v", gotB)
	}
	if gotA.CredentialID == gotB.CredentialID {
		t.Fatal("two targets share an automatically saved login")
	}

	// One saved login can serve both.
	if err := app.BindCredential("rdp", "nas", b, gotA.CredentialID); err != nil {
		t.Fatal(err)
	}
	gotB, _ = app.GetRDPLogin("nas", b)
	if gotB.Username != "first" {
		t.Fatalf("after binding b = %+v", gotB)
	}
	views, _ := app.ListCredentials()
	var used int
	for _, view := range views {
		if view.ID == gotA.CredentialID {
			used = view.UsedBy
		}
	}
	if used != 2 {
		t.Fatalf("used by = %d", used)
	}

	// A working login updates the assigned credential instead of adding another.
	if err := app.SaveRDPLogin("nas", a, RDPLogin{Username: "first", Password: "changed"}); err != nil {
		t.Fatal(err)
	}
	login, _, _ := keychain.GetLogin(gotA.CredentialID)
	if login.Password != "changed" {
		t.Fatalf("password = %q", login.Password)
	}

	// Deleting a login clears its assignments.
	if err := app.DeleteCredential(gotA.CredentialID); err != nil {
		t.Fatal(err)
	}
	if got, _ := app.GetRDPLogin("nas", a); got.HasPassword {
		t.Fatalf("a still resolves: %+v", got)
	}
	if _, found, _ := keychain.GetLogin(gotA.CredentialID); found {
		t.Fatal("the secret stayed in the keychain")
	}

	if err := app.BindCredential("rdp", "nas", ForwardTarget{RemoteHost: "10.9.9.9", RemotePort: 3389}, ""); err == nil {
		t.Error("binding a target that is not a forward was accepted")
	}
	if err := app.BindCredential("rdp", "nas", a, "missing"); err == nil {
		t.Error("binding a missing login was accepted")
	}
	if err := app.BindCredential("vnc", "nas", a, ""); err == nil {
		t.Error("binding for a protocol without a client was accepted")
	}
}

func TestRenameAndDeleteFollowSettings(t *testing.T) {
	app := newTestApp(t, demoTunnel("old", "", rdpForward("10.0.0.1", 13389)))
	target := ForwardTarget{RemoteHost: "10.0.0.1", RemotePort: 3389}
	if err := app.SaveRDPLogin("old", target, RDPLogin{Username: "u", Password: "p"}); err != nil {
		t.Fatal(err)
	}
	if err := app.SetTunnelSettings("old", prefs.TunnelSettings{Clipboard: prefs.ClipboardOff}); err != nil {
		t.Fatal(err)
	}
	app.moveRDPState("old", "new", []config.PortForward{rdpForward("10.0.0.1", 13389)})
	p := app.prefs.Get()
	if p.Tunnels["new"].Clipboard != prefs.ClipboardOff {
		t.Error("tunnel settings did not follow the rename")
	}
	if _, ok := p.Tunnels["old"]; ok {
		t.Error("old tunnel settings stayed")
	}
	if p.CredentialBindings[bindingKey("rdp", "new>10.0.0.1:3389")] == "" {
		t.Errorf("binding did not follow the rename: %v", p.CredentialBindings)
	}

	app.removeRDPState("new", []config.PortForward{rdpForward("10.0.0.1", 13389)})
	p = app.prefs.Get()
	if len(p.CredentialBindings) != 0 || len(p.Tunnels) != 0 {
		t.Errorf("state stayed after delete: %+v %+v", p.CredentialBindings, p.Tunnels)
	}
	if len(p.Credentials) != 1 {
		t.Error("deleting a tunnel must not delete the reusable login")
	}
}

func TestOpenToolValidatesTargets(t *testing.T) {
	ssh := config.PortForward{LocalPort: 12222, RemoteHost: "10.0.0.3", RemotePort: 22}
	app := newTestApp(t, demoTunnel("nas", "", rdpForward("10.0.0.1", 13389), ssh))
	cases := []struct {
		spec windows.Spec
		ok   bool
	}{
		{windows.Spec{Tool: windows.ToolSettings}, true},
		{windows.Spec{Tool: windows.ToolTerminal, TunnelID: "nas"}, true},
		{windows.Spec{Tool: windows.ToolTerminal, TunnelID: "missing"}, false},
		{windows.Spec{Tool: "unknown", TunnelID: "nas"}, false},
		{windows.Spec{Tool: windows.ToolRDP, TunnelID: "nas", RemoteHost: "10.0.0.1", RemotePort: 3389}, true},
		{windows.Spec{Tool: windows.ToolRDP, TunnelID: "nas", RemoteHost: "10.0.0.3", RemotePort: 22}, false},
		{windows.Spec{Tool: windows.ToolTerminal, TunnelID: "nas", RemoteHost: "10.0.0.3", RemotePort: 22}, true},
		{windows.Spec{Tool: windows.ToolFiles, TunnelID: "nas", RemoteHost: "evil.example", RemotePort: 22}, false},
	}
	for _, c := range cases {
		if err := app.validateToolSpec(c.spec); (err == nil) != c.ok {
			t.Errorf("%+v: err = %v", c.spec, err)
		}
	}
}

func TestToolSessionsEndWithTheirTab(t *testing.T) {
	app := newTestApp(t, demoTunnel("a", ""))
	result, err := app.OpenTool(windows.Spec{Tool: windows.ToolLogs, TunnelID: "a"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.External || !strings.Contains(result.URL, "external=1") {
		t.Errorf("without native windows the caller must open the URL: %+v", result)
	}
	state, err := app.GetToolWindow(result.Window)
	if err != nil || len(state.Tabs) != 1 {
		t.Fatalf("state = %+v err=%v", state, err)
	}
	app.CloseTab(result.Window, result.Tab)
	if _, err := app.GetToolWindow(result.Window); err == nil {
		t.Error("the window should be gone with its last tab")
	}
}

func TestCloseModeClosesTunnelTabs(t *testing.T) {
	app := newTestApp(t, demoTunnel("a", ""), demoTunnel("b", ""))
	if err := app.SetTunnelSettings("a", prefs.TunnelSettings{AfterDisconnect: prefs.AfterDisconnectClose}); err != nil {
		t.Fatal(err)
	}
	open := func(id string) string {
		result, err := app.OpenTool(windows.Spec{Tool: windows.ToolLogs, TunnelID: id})
		if err != nil {
			t.Fatal(err)
		}
		return result.Window
	}
	a, b := open("a"), open("b")
	app.handleToolsAfterDisconnect("a", config.StatusConnecting)
	if _, err := app.GetToolWindow(a); err != nil {
		t.Error("only a lost connection may close tabs")
	}
	app.handleToolsAfterDisconnect("a", config.StatusDisconnected)
	if _, err := app.GetToolWindow(a); err == nil {
		t.Error("close mode left the tab open")
	}
	app.handleToolsAfterDisconnect("b", config.StatusDisconnected)
	if _, err := app.GetToolWindow(b); err != nil {
		t.Error("resume mode must keep the tab")
	}
}

func TestAttachOptionsFollowDetection(t *testing.T) {
	app := newTestApp(t, demoTunnel("a", "", rdpForward("10.0.0.1", 13389)))
	if got := app.GetAttachOptions("a"); len(got) != 1 || got[0].Label != "Terminal" {
		t.Fatalf("options = %+v", got)
	}
	p := app.prefs.Get()
	p.HostTools = map[string]prefs.HostTools{"a": {RDP: true}}
	if err := app.prefs.Set(p); err != nil {
		t.Fatal(err)
	}
	if got := app.GetAttachOptions("a"); len(got) != 2 || got[1].Protocol != "rdp" {
		t.Fatalf("options = %+v", got)
	}
	tray := app.trayAttachOptions("a")
	if len(tray) != 3 || tray[2].RemoteHost != "10.0.0.1" || !strings.Contains(tray[2].Label, "Remote desktop") {
		t.Fatalf("tray options = %+v", tray)
	}
}

func prefsPath(t *testing.T, app *App) string {
	t.Helper()
	return app.prefs.Path()
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestSSHLoginFollowsItsTunnelAndPinsTheHostKey(t *testing.T) {
	app := newTestApp(t, demoTunnel("old", ""))
	if login, err := app.sshLogin("old"); err != nil || login != nil {
		t.Fatalf("a tunnel without a login got %+v, err=%v", login, err)
	}
	saved, err := app.SaveCredential(CredentialInput{Name: "Shared", Username: "admin", Password: "s3cret", Protocols: []string{"ssh"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := app.BindCredential("ssh", "old", ForwardTarget{}, saved.ID); err != nil {
		t.Fatal(err)
	}
	login, err := app.sshLogin("old")
	if err != nil || login == nil || login.Username != "admin" || login.Password != "s3cret" {
		t.Fatalf("login = %+v err=%v", login, err)
	}

	for address, fingerprint := range map[string]string{"a:22": "SHA256:first", "jump:22": "SHA256:jump"} {
		if err := app.pinSSHHostKey("old", address, fingerprint); err != nil {
			t.Fatal(err)
		}
	}
	if got := app.GetSSHHostKeys("old"); len(got) != 2 || got[0].Address != "a:22" || got[0].Fingerprint != "SHA256:first" || got[1].Address != "jump:22" {
		t.Errorf("pinned = %+v", got)
	}
	data := readFile(t, filepath.Join(filepath.Dir(prefsPath(t, app)), "prefs.json"))
	if strings.Contains(data, "s3cret") {
		t.Error("prefs.json contains the password")
	}

	app.moveRDPState("old", "new", nil)
	if id, err := app.GetCredentialBinding("ssh", "old", ForwardTarget{}); err == nil && id != "" {
		t.Error("the login stayed on the old ID")
	}
	if p := app.prefs.Get(); p.CredentialBindings[bindingKey("ssh", "new")] != saved.ID || p.SSHHostKeys["new"]["a:22"] != "SHA256:first" || p.SSHHostKeys["old"] != nil {
		t.Errorf("rename: %+v %+v", p.CredentialBindings, p.SSHHostKeys)
	}

	if err := app.ForgetSSHHostKeys("new"); err != nil {
		t.Fatal(err)
	}
	if got := app.GetSSHHostKeys("new"); len(got) != 0 {
		t.Fatalf("after forgetting: %+v", got)
	}
	if err := app.pinSSHHostKey("new", "a:22", "SHA256:other"); err != nil {
		t.Fatal(err)
	}

	app.removeRDPState("new", nil)
	if p := app.prefs.Get(); len(p.CredentialBindings) != 0 || len(p.SSHHostKeys) != 0 || len(p.Credentials) != 1 {
		t.Errorf("after delete: %+v %+v %+v", p.CredentialBindings, p.SSHHostKeys, p.Credentials)
	}
}

func TestSSHLoginWithoutASecretFallsBackToKeys(t *testing.T) {
	app := newTestApp(t, demoTunnel("host", ""))
	saved, err := app.SaveCredential(CredentialInput{Name: "Shared", Username: "admin", Password: "p", Protocols: []string{"ssh"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := app.BindCredential("ssh", "host", ForwardTarget{}, saved.ID); err != nil {
		t.Fatal(err)
	}
	if err := keychain.DeleteLogin(saved.ID); err != nil {
		t.Fatal(err)
	}
	if login, err := app.sshLogin("host"); err != nil || login != nil {
		t.Fatalf("login = %+v err=%v", login, err)
	}
}

// One login opens both: a remote desktop can use a login saved for SSH without
// assigning it, and assigning it marks it as used for RDP.
func TestRemoteDesktopCanUseASavedSSHLogin(t *testing.T) {
	app := newTestApp(t, demoTunnel("host", "", rdpForward("10.0.0.1", 13389)))
	target := ForwardTarget{RemoteHost: "10.0.0.1", RemotePort: 3389}
	saved, err := app.SaveCredential(CredentialInput{Name: "Shared", Username: "admin", Password: "s3cret", Protocols: []string{"ssh"}})
	if err != nil {
		t.Fatal(err)
	}
	key := rdpForwardKey("host", target)

	if _, err := app.storedRDPLogin(key, ""); err == nil {
		t.Error("a target without an assigned login found one")
	}
	login, err := app.storedRDPLogin(key, saved.ID)
	if err != nil || login.Password != "s3cret" || login.Username != "admin" {
		t.Fatalf("chosen login = %+v err=%v", login, err)
	}
	if id, _ := app.GetCredentialBinding("rdp", "host", target); id != "" {
		t.Error("using a login once must not assign it")
	}
	if _, err := app.storedRDPLogin(key, "missing"); err == nil {
		t.Error("an unknown login was accepted")
	}

	if err := app.BindCredential("rdp", "host", target, saved.ID); err != nil {
		t.Fatal(err)
	}
	views, err := app.ListCredentials()
	if err != nil || len(views) != 1 || strings.Join(views[0].Protocols, ",") != "ssh,rdp" {
		t.Fatalf("views = %+v err=%v", views, err)
	}
	if _, err := app.storedRDPLogin(key, ""); err != nil {
		t.Errorf("assigned login: %v", err)
	}

	// Assigning it again adds nothing.
	if err := app.BindCredential("rdp", "host", target, saved.ID); err != nil {
		t.Fatal(err)
	}
	views, _ = app.ListCredentials()
	if strings.Join(views[0].Protocols, ",") != "ssh,rdp" {
		t.Errorf("protocols = %v", views[0].Protocols)
	}
}

func TestGroupsCanExistBeforeTheirFirstTunnel(t *testing.T) {
	app := newTestApp(t, demoTunnel("host", "Old"))
	if err := app.AddGroup("  "); err == nil {
		t.Error("a group without a name was accepted")
	}
	for _, name := range []string{"Empty", " Empty ", "Old"} {
		if err := app.AddGroup(name); err != nil {
			t.Fatal(err)
		}
	}
	if got := strings.Join(app.GetGroups(), ","); got != "Empty,Old" {
		t.Fatalf("groups = %q", got)
	}
	if err := app.RenameGroup("Old", "New"); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(app.GetGroups(), ","); got != "Empty,New" {
		t.Errorf("after rename = %q", got)
	}
	if cfg, _ := app.store.GetTunnel("host"); cfg.Group != "New" {
		t.Errorf("tunnel group = %q", cfg.Group)
	}
	if err := app.RemoveGroup("Empty"); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(app.GetGroups(), ","); got != "New" {
		t.Errorf("after remove = %q", got)
	}
}

func TestRemoteDesktopModeOverridesDetection(t *testing.T) {
	app := newTestApp(t, demoTunnel("host", ""))
	offers := func() bool {
		return slices.ContainsFunc(app.GetAttachOptions("host"), func(o AttachOption) bool { return o.Protocol == "rdp" })
	}
	if offers() {
		t.Error("offered without detection or an override")
	}
	if err := app.SetTunnelSettings("host", prefs.TunnelSettings{RDP: rdpModeOn}); err != nil {
		t.Fatal(err)
	}
	if !offers() {
		t.Error("turned on, but not offered")
	}
	p := app.prefs.Get()
	p.HostTools = map[string]prefs.HostTools{"host": {RDP: true}}
	if err := app.prefs.Set(p); err != nil {
		t.Fatal(err)
	}
	if err := app.SetTunnelSettings("host", prefs.TunnelSettings{RDP: rdpModeOff}); err != nil {
		t.Fatal(err)
	}
	if offers() {
		t.Error("turned off, but still offered after detection")
	}
	if err := app.SetTunnelSettings("host", prefs.TunnelSettings{}); err != nil {
		t.Fatal(err)
	}
	if !offers() {
		t.Error("without an override it should follow detection")
	}
	if err := app.SetTunnelSettings("host", prefs.TunnelSettings{RDP: "maybe"}); err == nil {
		t.Error("an unknown mode was accepted")
	}
}
