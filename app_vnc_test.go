package main

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"ssh-tunnel-manager/internal/config"
	"ssh-tunnel-manager/internal/keychain"
	"ssh-tunnel-manager/internal/prefs"
	"ssh-tunnel-manager/internal/vnc"
	"ssh-tunnel-manager/internal/windows"
)

func vncForward(host string, remote, local int) config.PortForward {
	return config.PortForward{LocalPort: local, RemoteHost: host, RemotePort: remote}
}

func TestVNCTargetsMustBeVNCForwards(t *testing.T) {
	explicit := vncForward("127.0.0.1", 5901, 15901)
	explicit.Protocol = config.ForwardProtocolVNC
	app := newTestApp(t, demoTunnel("mac", "", vncForward("10.0.0.5", 5900, 15900), explicit, rdpForward("10.0.0.1", 13389)))

	key, address, err := app.resolveVNCTarget("mac", ForwardTarget{})
	if err != nil || key != "mac" || address != "127.0.0.1:5900" {
		t.Fatalf("host = %q %q %v", key, address, err)
	}
	key, address, err = app.resolveVNCTarget("mac", ForwardTarget{RemoteHost: "10.0.0.5", RemotePort: 5900})
	if err != nil || key != "mac>10.0.0.5:5900" || address != "10.0.0.5:5900" {
		t.Fatalf("inferred forward = %q %q %v", key, address, err)
	}
	if _, address, err = app.resolveVNCTarget("mac", ForwardTarget{RemoteHost: "127.0.0.1", RemotePort: 5901}); err != nil || address != "127.0.0.1:5901" {
		t.Fatalf("explicit forward = %q %v", address, err)
	}
	for _, target := range []ForwardTarget{{RemoteHost: "10.0.0.1", RemotePort: 3389}, {RemoteHost: "evil.example", RemotePort: 5900}} {
		if _, _, err := app.resolveVNCTarget("mac", target); err == nil {
			t.Errorf("%+v accepted", target)
		}
	}
	if _, _, err := app.resolveVNCTarget("missing", ForwardTarget{}); err == nil {
		t.Error("unknown tunnel accepted")
	}
}

func TestVNCLoginIsSavedOnlyWhenAsked(t *testing.T) {
	app := newTestApp(t, demoTunnel("mac", ""))
	host := ForwardTarget{}

	creds, err := app.vncCredentials("mac", VNCLogin{})
	if err != nil || creds != (vnc.Credentials{}) {
		t.Fatalf("no login = %+v %v", creds, err)
	}
	if got, err := app.GetVNCLogin("mac", host); err != nil || got.HasPassword {
		t.Fatalf("saved = %+v %v", got, err)
	}

	app.keepVNCLogin("mac", VNCLogin{Username: "artur", Password: "secret"})
	saved, err := app.GetVNCLogin("mac", host)
	if err != nil || saved.Username != "artur" || !saved.HasPassword || saved.CredentialName != "mac (VNC)" {
		t.Fatalf("saved = %+v %v", saved, err)
	}
	creds, err = app.vncCredentials("mac", VNCLogin{})
	if err != nil || creds.Username != "artur" || creds.Password != "secret" {
		t.Fatalf("assigned login = %+v %v", creds, err)
	}
	// A typed login wins over the stored one.
	if creds, _ = app.vncCredentials("mac", VNCLogin{Password: "typed"}); creds.Password != "typed" || creds.Username != "" {
		t.Fatalf("typed = %+v", creds)
	}

	// Saving again updates the same login.
	app.keepVNCLogin("mac", VNCLogin{Username: "artur", Password: "changed"})
	login, _, _ := keychain.GetLogin(saved.CredentialID)
	if login.Password != "changed" {
		t.Fatalf("password = %q", login.Password)
	}
	if views, _ := app.ListCredentials(); len(views) != 1 || strings.Join(views[0].Protocols, ",") != "vnc" {
		t.Fatalf("views = %+v", views)
	}
}

func TestVNCCanUseAndAssignASavedLogin(t *testing.T) {
	app := newTestApp(t, demoTunnel("mac", ""))
	shared, err := app.SaveCredential(CredentialInput{Name: "Mac account", Username: "artur", Password: "s3cret", Protocols: []string{"ssh"}})
	if err != nil {
		t.Fatal(err)
	}
	creds, err := app.vncCredentials("mac", VNCLogin{CredentialID: shared.ID})
	if err != nil || creds.Username != "artur" || creds.Password != "s3cret" {
		t.Fatalf("chosen = %+v %v", creds, err)
	}
	if _, err := app.vncCredentials("mac", VNCLogin{CredentialID: "missing"}); err == nil {
		t.Error("unknown login accepted")
	}

	app.keepVNCLogin("mac", VNCLogin{CredentialID: shared.ID})
	if id, _ := app.GetCredentialBinding("vnc", "mac", ForwardTarget{}); id != shared.ID {
		t.Fatalf("binding = %q", id)
	}
	// Only marked for SSH, the login is updated in place.
	app.keepVNCLogin("mac", VNCLogin{Username: "artur", Password: "s3cret"})
	if views, _ := app.ListCredentials(); len(views) != 1 || strings.Join(views[0].Protocols, ",") != "ssh,vnc" {
		t.Fatalf("views = %+v", views)
	}
	// Once the tunnel uses it for SSH, a new password for VNC is saved as a
	// new login, so the SSH login keeps working.
	if err := app.BindCredential("ssh", "mac", ForwardTarget{}, shared.ID); err != nil {
		t.Fatal(err)
	}
	app.keepVNCLogin("mac", VNCLogin{Username: "other", Password: "new"})
	views, _ := app.ListCredentials()
	if len(views) != 2 {
		t.Fatalf("views = %+v", views)
	}
	if stored, _, _ := keychain.GetLogin(shared.ID); stored.Username != "artur" || stored.Password != "s3cret" {
		t.Fatalf("shared login changed: %+v", stored)
	}
	if id, _ := app.GetCredentialBinding("vnc", "mac", ForwardTarget{}); id == shared.ID || id == "" {
		t.Fatalf("binding = %q", id)
	}
}

func TestVNCLoginOutcome(t *testing.T) {
	result, err := vncLoginOutcome(&vnc.CredentialsRequiredError{Username: true}, vnc.Credentials{}, "a:1")
	if err != nil || result.Status != vncLoginRequired || !result.NeedsUsername || result.Message != "" {
		t.Fatalf("required = %+v %v", result, err)
	}
	result, _ = vncLoginOutcome(&vnc.CredentialsRequiredError{Username: true}, vnc.Credentials{Password: "p"}, "a:1")
	if !strings.Contains(result.Message, "user name") {
		t.Fatalf("password only = %+v", result)
	}
	result, err = vncLoginOutcome(&vnc.AuthError{Reason: "Authentication failed"}, vnc.Credentials{Username: "u", Password: "p"}, "a:1")
	if err != nil || result.Status != vncLoginFailed || result.NeedsUsername || !strings.Contains(result.Message, "Authentication failed") {
		t.Fatalf("rejected = %+v %v", result, err)
	}
	result, err = vncLoginOutcome(&vnc.UnsupportedSecurityError{Types: []byte{19}}, vnc.Credentials{}, "a:1")
	if result != nil || err == nil || !strings.Contains(err.Error(), "VeNCrypt") {
		t.Fatalf("unsupported = %+v %v", result, err)
	}
	if _, err := vncLoginOutcome(vnc.ErrNotVNC, vnc.Credentials{}, "a:1"); !errors.Is(err, vnc.ErrNotVNC) {
		t.Fatalf("not VNC = %v", err)
	}
}

func TestScreenSharingIsOfferedLikeRemoteDesktop(t *testing.T) {
	app := newTestApp(t, demoTunnel("mac", "", vncForward("10.0.0.5", 5900, 15900)))
	offers := func() bool {
		return slices.ContainsFunc(app.GetAttachOptions("mac"), func(o AttachOption) bool { return o.Protocol == "vnc" })
	}
	if offers() {
		t.Error("offered without detection")
	}
	p := app.prefs.Get()
	p.HostTools = map[string]prefs.HostTools{"mac": {VNC: true}}
	if err := app.prefs.Set(p); err != nil {
		t.Fatal(err)
	}
	if !offers() {
		t.Error("detected, but not offered")
	}
	if err := app.SetTunnelSettings("mac", prefs.TunnelSettings{VNC: rdpModeOff}); err != nil {
		t.Fatal(err)
	}
	if offers() {
		t.Error("turned off, but offered")
	}
	if err := app.SetTunnelSettings("mac", prefs.TunnelSettings{VNC: "maybe"}); err == nil {
		t.Error("unknown mode accepted")
	}

	tray := app.trayAttachOptions("mac")
	last := tray[len(tray)-1]
	if last.Protocol != "vnc" || last.RemoteHost != "10.0.0.5" || !strings.HasPrefix(last.Label, "Screen sharing: ") {
		t.Fatalf("tray = %+v", tray)
	}

	cases := []struct {
		spec windows.Spec
		ok   bool
	}{
		{windows.Spec{Tool: windows.ToolVNC, TunnelID: "mac"}, true},
		{windows.Spec{Tool: windows.ToolVNC, TunnelID: "mac", RemoteHost: "10.0.0.5", RemotePort: 5900}, true},
		{windows.Spec{Tool: windows.ToolVNC, TunnelID: "mac", RemoteHost: "10.0.0.6", RemotePort: 5900}, false},
		{windows.Spec{Tool: windows.ToolRDP, TunnelID: "mac", RemoteHost: "10.0.0.5", RemotePort: 5900}, false},
	}
	for _, c := range cases {
		if err := app.validateToolSpec(c.spec); (err == nil) != c.ok {
			t.Errorf("%+v: err = %v", c.spec, err)
		}
	}
}

func TestVNCFullColorIsKeptPerTarget(t *testing.T) {
	app := newTestApp(t, demoTunnel("mac", "", vncForward("10.0.0.5", 5900, 15900)))
	forward := ForwardTarget{RemoteHost: "10.0.0.5", RemotePort: 5900}
	if err := app.SetVNCFullColor("mac", ForwardTarget{}, true); err != nil {
		t.Fatal(err)
	}
	if p := app.prefs.Get(); !p.VNCFullColor["mac"] || p.VNCFullColor["mac>10.0.0.5:5900"] {
		t.Fatalf("full colour = %v", p.VNCFullColor)
	}
	if err := app.SetVNCFullColor("mac", forward, true); err != nil {
		t.Fatal(err)
	}
	if err := app.SetVNCFullColor("mac", ForwardTarget{}, false); err != nil {
		t.Fatal(err)
	}
	if p := app.prefs.Get(); p.VNCFullColor["mac"] || !p.VNCFullColor["mac>10.0.0.5:5900"] {
		t.Fatalf("full colour = %v", p.VNCFullColor)
	}
	if err := app.SetVNCFullColor("mac", ForwardTarget{RemoteHost: "evil.example", RemotePort: 5900}, true); err == nil {
		t.Error("unknown target accepted")
	}
}

func TestVNCLoginFollowsRenameAndDelete(t *testing.T) {
	forward := vncForward("10.0.0.5", 5900, 15900)
	app := newTestApp(t, demoTunnel("old", "", forward))
	app.keepVNCLogin("old", VNCLogin{Username: "a", Password: "p"})
	app.keepVNCLogin(forwardStateKey("old", ForwardTarget{RemoteHost: "10.0.0.5", RemotePort: 5900}), VNCLogin{Password: "q"})

	if err := app.SetVNCFullColor("old", ForwardTarget{RemoteHost: "10.0.0.5", RemotePort: 5900}, true); err != nil {
		t.Fatal(err)
	}

	app.moveRDPState("old", "new", []config.PortForward{forward})
	p := app.prefs.Get()
	if p.CredentialBindings["vnc:new"] == "" || p.CredentialBindings["vnc:new>10.0.0.5:5900"] == "" || len(p.CredentialBindings) != 2 {
		t.Fatalf("bindings = %v", p.CredentialBindings)
	}
	if !p.VNCFullColor["new>10.0.0.5:5900"] || len(p.VNCFullColor) != 1 {
		t.Fatalf("full colour = %v", p.VNCFullColor)
	}
	app.removeRDPState("new", []config.PortForward{forward})
	if p := app.prefs.Get(); len(p.CredentialBindings) != 0 || len(p.VNCFullColor) != 0 {
		t.Fatalf("after delete: bindings %v, full colour %v", p.CredentialBindings, p.VNCFullColor)
	}
}
