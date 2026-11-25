package main

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"strconv"
	"strings"
	"testing"

	"ssh-tunnel-manager/internal/config"
	"ssh-tunnel-manager/internal/prefs"
	"ssh-tunnel-manager/internal/rdp"
	"ssh-tunnel-manager/internal/vnc"
	"ssh-tunnel-manager/internal/windows"
)

func desktop(name, protocol, address string, port int) config.RemoteDesktop {
	return config.RemoteDesktop{Name: name, Protocol: protocol, Address: address, Port: port}
}

func TestRemoteDesktopNamesAreSharedWithHosts(t *testing.T) {
	app := newTestApp(t, demoTunnel("jump", ""))
	if err := app.AddRemoteDesktop(desktop("jump", "rdp", "10.0.0.5", 0)); err == nil {
		t.Error("a remote desktop took a host's name")
	}
	if err := app.AddRemoteDesktop(desktop("laptop", "rdp", " 10.0.0.5 ", 0)); err != nil {
		t.Fatal(err)
	}
	got := app.GetRemoteDesktops()
	if len(got) != 1 || got[0].Port != 3389 || got[0].Address != "10.0.0.5" || got[0].ID != "rd:laptop" {
		t.Fatalf("desktops = %+v", got)
	}
	if err := app.AddTunnel(demoTunnel("laptop", "")); err == nil {
		t.Error("a host took a remote desktop's name")
	}
	if err := app.AddTunnel(demoTunnel("rd:laptop", "")); err == nil {
		t.Error("a host name with the remote desktop prefix was accepted")
	}
	if err := app.AddRemoteDesktop(desktop("mac", "vnc", "[fd00::5]", 0)); err != nil {
		t.Fatal(err)
	}
	if d, _ := app.store.GetRemoteDesktop("mac"); d.Port != 5900 || d.Address != "fd00::5" {
		t.Fatalf("mac = %+v", d)
	}
}

func TestRemoteDesktopRenameMovesAndDeleteClearsState(t *testing.T) {
	app := newTestApp(t)
	if err := app.AddRemoteDesktop(desktop("laptop", "rdp", "10.0.0.5", 3389)); err != nil {
		t.Fatal(err)
	}
	if err := app.SaveRDPLogin("rd:laptop", ForwardTarget{}, RDPLogin{Username: "me", Password: "secret"}); err != nil {
		t.Fatal(err)
	}
	if err := app.TrustRDPCertificate("rd:laptop", ForwardTarget{}, "abc"); err != nil {
		t.Fatal(err)
	}
	if err := app.SetTunnelSettings("rd:laptop", prefs.TunnelSettings{Clipboard: prefs.ClipboardOff}); err != nil {
		t.Fatal(err)
	}
	app.setVNCPublicConsent("rd:laptop", true)
	saved, _ := app.GetRDPLogin("rd:laptop", ForwardTarget{})
	if saved.CredentialName != "laptop" {
		t.Fatalf("login name = %q", saved.CredentialName)
	}

	renamed := desktop("work laptop", "rdp", "10.0.0.5", 3389)
	renamed.Group = "Office"
	if err := app.UpdateRemoteDesktop("laptop", renamed); err != nil {
		t.Fatal(err)
	}
	p := app.prefs.Get()
	if p.RDPCertificates["rd:work laptop"] != "abc" || p.RDPCertificates["rd:laptop"] != "" {
		t.Fatalf("certificates = %v", p.RDPCertificates)
	}
	if p.CredentialBindings["rdp:rd:work laptop"] != saved.CredentialID || p.CredentialBindings["rdp:rd:laptop"] != "" {
		t.Fatalf("bindings = %v", p.CredentialBindings)
	}
	if p.Tunnels["rd:work laptop"].Clipboard != prefs.ClipboardOff || !p.VNCPublicConsent["rd:work laptop"] {
		t.Fatalf("settings = %v, consent = %v", p.Tunnels, p.VNCPublicConsent)
	}
	if app.GetEffectiveSettings()["rd:work laptop"].Clipboard != prefs.ClipboardOff {
		t.Fatal("effective settings miss the remote desktop")
	}

	// A new protocol drops the login assigned for the old one.
	if err := app.BindCredential("rdp", "rd:work laptop", ForwardTarget{}, saved.CredentialID); err != nil {
		t.Fatal(err)
	}
	asVNC := renamed
	asVNC.Protocol = "vnc"
	if err := app.UpdateRemoteDesktop("work laptop", asVNC); err != nil {
		t.Fatal(err)
	}
	if id := app.prefs.Get().CredentialBindings["rdp:rd:work laptop"]; id != "" {
		t.Fatalf("old protocol binding = %q", id)
	}

	// A new address drops the consent given for the old one.
	moved := asVNC
	moved.Address = "203.0.113.7"
	if err := app.UpdateRemoteDesktop("work laptop", moved); err != nil {
		t.Fatal(err)
	}
	if app.prefs.Get().VNCPublicConsent["rd:work laptop"] {
		t.Fatal("consent survived an address change")
	}

	if err := app.DeleteRemoteDesktop("work laptop"); err != nil {
		t.Fatal(err)
	}
	p = app.prefs.Get()
	if len(p.RDPCertificates) != 0 || len(p.CredentialBindings) != 0 || len(p.Tunnels) != 0 {
		t.Fatalf("left behind: %v %v %v", p.RDPCertificates, p.CredentialBindings, p.Tunnels)
	}
	// The saved login itself stays.
	if views, _ := app.ListCredentials(); len(views) != 1 {
		t.Fatalf("credentials = %+v", views)
	}
}

func TestDuplicateRemoteDesktopPicksAFreeName(t *testing.T) {
	app := newTestApp(t, demoTunnel("pc copy", ""))
	if err := app.AddRemoteDesktop(desktop("pc", "vnc", "10.0.0.6", 5901)); err != nil {
		t.Fatal(err)
	}
	copied, err := app.DuplicateRemoteDesktop("pc")
	if err != nil {
		t.Fatal(err)
	}
	if copied.Name != "pc copy 2" || copied.Port != 5901 || copied.Protocol != "vnc" {
		t.Fatalf("copy = %+v", copied)
	}
}

func TestRemoteDesktopTargetsAndTools(t *testing.T) {
	app := newTestApp(t)
	if err := app.AddRemoteDesktop(desktop("pc", "rdp", "10.0.0.5", 3390)); err != nil {
		t.Fatal(err)
	}
	key, address, err := app.resolveRDPTarget("rd:pc", ForwardTarget{})
	if err != nil || key != "rd:pc" || address != "10.0.0.5:3390" {
		t.Fatalf("rdp = %q %q %v", key, address, err)
	}
	if _, _, err := app.resolveVNCTarget("rd:pc", ForwardTarget{}); err == nil {
		t.Error("an RDP desktop opened as VNC")
	}
	if _, _, err := app.resolveRDPTarget("rd:pc", ForwardTarget{RemoteHost: "evil", RemotePort: 1}); err == nil {
		t.Error("a forward on a remote desktop was accepted")
	}
	if _, _, err := app.resolveRDPTarget("rd:missing", ForwardTarget{}); err == nil {
		t.Error("unknown remote desktop accepted")
	}
	if err := app.validateToolSpec(windows.Spec{Tool: windows.ToolRDP, TunnelID: "rd:pc"}); err != nil {
		t.Fatal(err)
	}
	for _, spec := range []windows.Spec{
		{Tool: windows.ToolTerminal, TunnelID: "rd:pc"},
		{Tool: windows.ToolVNC, TunnelID: "rd:pc"},
		{Tool: windows.ToolFiles, TunnelID: "rd:pc"},
	} {
		if err := app.validateToolSpec(spec); err == nil {
			t.Errorf("%+v accepted", spec)
		}
	}
	if app.toolNeedsConnection(windows.Spec{Tool: windows.ToolRDP, TunnelID: "rd:pc"}) {
		t.Error("a remote desktop waits for an SSH connection")
	}
	if got := toolTitle(windows.Spec{Tool: windows.ToolRDP, TunnelID: "rd:pc"}); got != "Remote desktop: pc" {
		t.Errorf("title = %q", got)
	}
	if err := app.BindCredential("ssh", "rd:pc", ForwardTarget{}, ""); err == nil {
		t.Error("an SSH login was assigned to a remote desktop")
	}
}

func TestFirstRDPCertificateWaitsForConfirmationOnlyWithoutSSH(t *testing.T) {
	app := newTestApp(t, demoTunnel("host", ""))
	if err := app.AddRemoteDesktop(desktop("pc", "rdp", "10.0.0.5", 3389)); err != nil {
		t.Fatal(err)
	}
	if err := app.checkRDPCertificate("rd:pc", "aa"); !errors.Is(err, rdp.ErrCertificateUnconfirmed) {
		t.Fatalf("first use = %v", err)
	}
	if pinned := app.prefs.Get().RDPCertificates["rd:pc"]; pinned != "" {
		t.Fatalf("pinned before confirmation: %q", pinned)
	}
	if err := app.TrustRDPCertificate("rd:pc", ForwardTarget{}, "aa"); err != nil {
		t.Fatal(err)
	}
	if err := app.checkRDPCertificate("rd:pc", "aa"); err != nil {
		t.Fatalf("confirmed = %v", err)
	}
	if err := app.checkRDPCertificate("rd:pc", "bb"); !errors.Is(err, rdp.ErrCertificateChanged) {
		t.Fatalf("changed = %v", err)
	}
	// Through SSH the first certificate is still pinned silently.
	if err := app.checkRDPCertificate("host", "cc"); err != nil || app.prefs.Get().RDPCertificates["host"] != "cc" {
		t.Fatalf("ssh first use = %v", err)
	}
}

func TestCheckRemoteDesktops(t *testing.T) {
	app := newTestApp(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	open := listener.Addr().(*net.TCPAddr).Port
	closedListener, _ := net.Listen("tcp", "127.0.0.1:0")
	closed := closedListener.Addr().(*net.TCPAddr).Port
	closedListener.Close()
	if err := app.AddRemoteDesktop(desktop("up", "rdp", "127.0.0.1", open)); err != nil {
		t.Fatal(err)
	}
	if err := app.AddRemoteDesktop(desktop("down", "vnc", "127.0.0.1", closed)); err != nil {
		t.Fatal(err)
	}
	got := app.CheckRemoteDesktops([]string{"rd:up", "rd:down", "rd:missing"})
	if len(got) != 2 || !got["rd:up"] || got["rd:down"] {
		t.Fatalf("check = %v", got)
	}
}

func TestIsPrivateIP(t *testing.T) {
	for address, private := range map[string]bool{
		"10.1.2.3": true, "172.16.0.1": true, "172.31.255.255": true, "192.168.1.20": true,
		"100.64.0.1": true, "100.127.255.255": true, "fd12::1": true, "127.0.0.1": true, "::1": true,
		"::ffff:192.168.1.1": true,
		"172.32.0.1": false, "100.128.0.1": false, "8.8.8.8": false, "203.0.113.7": false,
		"fc00::1": false, "2001:db8::1": false,
	} {
		if got := isPrivateIP(net.ParseIP(address)); got != private {
			t.Errorf("%s: private = %v", address, got)
		}
	}
	if public, err := publicAddress(context.Background(), "192.168.1.5"); err != nil || public != "" {
		t.Errorf("private literal = %q %v", public, err)
	}
	if public, err := publicAddress(context.Background(), "203.0.113.7"); err != nil || public != "203.0.113.7" {
		t.Errorf("public literal = %q %v", public, err)
	}
}

// serveNoAuthRFB is a VNC server without authentication that stops after the
// security result, where the bridge hands the connection to the viewer.
func serveNoAuthRFB(t *testing.T) (int, <-chan struct{}) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	served := make(chan struct{}, 4)
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				_, _ = io.WriteString(conn, "RFB 003.008\n")
				version := make([]byte, 12)
				if _, err := io.ReadFull(conn, version); err != nil {
					return
				}
				_, _ = conn.Write([]byte{1, 1})
				choice := make([]byte, 1)
				if _, err := io.ReadFull(conn, choice); err != nil {
					return
				}
				_ = binary.Write(conn, binary.BigEndian, uint32(0))
				served <- struct{}{}
				_, _ = io.Copy(io.Discard, conn)
			}()
		}
	}()
	return listener.Addr().(*net.TCPAddr).Port, served
}

func TestOpenVNCConnectsDirectly(t *testing.T) {
	app := newTestApp(t)
	app.vncBridge = vnc.NewBridge(false)
	t.Cleanup(app.vncBridge.Close)
	port, served := serveNoAuthRFB(t)
	if err := app.AddRemoteDesktop(desktop("mac", "vnc", "127.0.0.1", port)); err != nil {
		t.Fatal(err)
	}
	result, err := app.OpenVNC("rd:mac", ForwardTarget{}, VNCLogin{})
	if err != nil || result.Status != vncConnected || result.SessionID == "" {
		t.Fatalf("open = %+v %v", result, err)
	}
	<-served
	app.CloseVNC(result.SessionID)

	closedListener, _ := net.Listen("tcp", "127.0.0.1:0")
	closed := closedListener.Addr().(*net.TCPAddr).Port
	closedListener.Close()
	if err := app.AddRemoteDesktop(desktop("off", "vnc", "127.0.0.1", closed)); err != nil {
		t.Fatal(err)
	}
	if _, err := app.OpenVNC("rd:off", ForwardTarget{}, VNCLogin{}); err == nil || !strings.Contains(err.Error(), "no VNC server accepts connections") {
		t.Fatalf("closed port = %v", err)
	}
}

func TestOpenVNCAsksBeforeAPublicAddress(t *testing.T) {
	app := newTestApp(t)
	app.vncBridge = vnc.NewBridge(false)
	t.Cleanup(app.vncBridge.Close)
	// TEST-NET-3 is outside private networks and never answers, so only the
	// question is tested here.
	if err := app.AddRemoteDesktop(desktop("far", "vnc", "203.0.113.7", 5900)); err != nil {
		t.Fatal(err)
	}
	result, err := app.OpenVNC("rd:far", ForwardTarget{}, VNCLogin{})
	if err != nil || result.Status != vncConfirmPublic || result.PublicAddress != "203.0.113.7" {
		t.Fatalf("open = %+v %v", result, err)
	}
	app.setVNCPublicConsent("rd:far", true)
	if confirm, err := app.confirmPublicVNC(context.Background(), "rd:far", "203.0.113.7:5900", VNCLogin{}); confirm != nil || err != nil {
		t.Fatalf("after consent = %+v %v", confirm, err)
	}
	app.setVNCPublicConsent("rd:far", false)
	if confirm, _ := app.confirmPublicVNC(context.Background(), "rd:far", "203.0.113.7:5900", VNCLogin{AllowPublic: true, RememberPublic: true}); confirm != nil {
		t.Fatalf("allowed = %+v", confirm)
	}
	if !app.prefs.Get().VNCPublicConsent["rd:far"] {
		t.Fatal("consent not remembered")
	}
	// A private address never asks.
	if confirm, _ := app.confirmPublicVNC(context.Background(), "rd:near", net.JoinHostPort("192.168.1.9", strconv.Itoa(5900)), VNCLogin{}); confirm != nil {
		t.Fatalf("private = %+v", confirm)
	}
}

func TestDialTargetGoesDirectForRemoteDesktops(t *testing.T) {
	app := newTestApp(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	conn, err := app.dialTarget("rd:x", listener.Addr().String())(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	conn.Close()
}

func TestExportAndImportRemoteDesktops(t *testing.T) {
	app := newTestApp(t, demoTunnel("jump", ""))
	if err := app.AddRemoteDesktop(desktop("pc", "rdp", "10.0.0.5", 3389)); err != nil {
		t.Fatal(err)
	}
	if err := app.AddRemoteDesktop(desktop("mac", "vnc", "10.0.0.6", 5900)); err != nil {
		t.Fatal(err)
	}
	path := t.TempDir() + "/export"
	if err := app.ExportTunnels(path, []string{"jump", "rd:pc"}); err != nil {
		t.Fatal(err)
	}
	other := newTestApp(t, demoTunnel("pc", ""))
	preview, err := other.ImportPreviewRemoteDesktops(path)
	if err != nil || len(preview) != 1 || preview[0].Name != "pc" {
		t.Fatalf("preview = %+v %v", preview, err)
	}
	hosts, _ := other.ImportPreview(path)
	if len(hosts) != 1 || hosts[0].Name != "jump" {
		t.Fatalf("hosts = %+v", hosts)
	}
	result, err := other.ImportRemoteDesktops(path, []string{"pc"})
	if err != nil || result.Imported != 0 || len(result.Skipped) != 1 {
		t.Fatalf("conflict = %+v %v", result, err)
	}

	onlyDesktops := t.TempDir() + "/desktops"
	if err := app.ExportTunnels(onlyDesktops, []string{"rd:mac"}); err != nil {
		t.Fatal(err)
	}
	third := newTestApp(t)
	result, err = third.ImportRemoteDesktops(onlyDesktops, []string{"mac"})
	if err != nil || result.Imported != 1 {
		t.Fatalf("import = %+v %v", result, err)
	}
	if d, ok := third.store.GetRemoteDesktop("mac"); !ok || d.Port != 5900 || d.Protocol != "vnc" {
		t.Fatalf("imported = %+v", d)
	}
}
