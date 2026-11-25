package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"ssh-tunnel-manager/internal/config"
	"ssh-tunnel-manager/internal/sshconfig"
	"ssh-tunnel-manager/internal/windows"
)

// remoteDesktopCheckTimeout bounds the reachability check of one remote desktop.
const remoteDesktopCheckTimeout = 2 * time.Second

// Main window tabs.
const (
	mainTabTunnels = "tunnels"
	mainTabClients = "clients"
	mainTabWeb     = "web"
)

// isRemoteDesktopID reports whether an ID names a remote desktop reached
// without SSH rather than an SSH host.
func isRemoteDesktopID(id string) bool {
	return strings.HasPrefix(id, config.RemoteDesktopIDPrefix)
}

type remoteDesktopNotFoundError struct{ name string }

func (e *remoteDesktopNotFoundError) Error() string {
	return "remote desktop not found: " + e.name
}

// remoteDesktop returns the remote desktop an ID names.
func (a *App) remoteDesktop(id string) (config.RemoteDesktop, error) {
	name := strings.TrimPrefix(id, config.RemoteDesktopIDPrefix)
	d, ok := a.store.GetRemoteDesktop(name)
	if !ok {
		return config.RemoteDesktop{}, &remoteDesktopNotFoundError{name}
	}
	return d, nil
}

// remoteDesktopAddress is where a remote desktop is dialed.
func remoteDesktopAddress(d config.RemoteDesktop) string {
	return net.JoinHostPort(d.Address, strconv.Itoa(d.Port))
}

// resolveRemoteDesktop returns the state key and address of a remote desktop
// opened with the given protocol. A remote desktop has no forwards.
func (a *App) resolveRemoteDesktop(id string, target ForwardTarget, protocol string) (string, string, error) {
	if target.RemoteHost != "" || target.RemotePort != 0 {
		return "", "", errors.New("a remote desktop reached without SSH has no forwards")
	}
	d, err := a.remoteDesktop(id)
	if err != nil {
		return "", "", err
	}
	if d.Protocol != protocol {
		return "", "", fmt.Errorf("%s is a %s desktop, not %s", d.Name, strings.ToUpper(d.Protocol), strings.ToUpper(protocol))
	}
	return d.ID, remoteDesktopAddress(d), nil
}

// dialTarget returns how a session reaches its server: through the tunnel's
// SSH connection, or directly for a remote desktop without SSH.
func (a *App) dialTarget(tunnelID, address string) func(ctx context.Context) (net.Conn, error) {
	if isRemoteDesktopID(tunnelID) {
		return func(ctx context.Context) (net.Conn, error) {
			var dialer net.Dialer
			return dialer.DialContext(ctx, "tcp", address)
		}
	}
	return func(ctx context.Context) (net.Conn, error) {
		return a.manager.DialRemote(ctx, tunnelID, address)
	}
}

// isConnectionRefused reports a direct connection that reached the machine
// but found nothing listening.
func isConnectionRefused(err error) bool {
	return errors.Is(err, syscall.ECONNREFUSED)
}

// GetRemoteDesktops returns the remote desktops reached without SSH.
func (a *App) GetRemoteDesktops() []config.RemoteDesktop {
	return a.store.GetRemoteDesktops()
}

func cleanRemoteDesktop(d config.RemoteDesktop) (config.RemoteDesktop, error) {
	d.Address = strings.TrimSpace(d.Address)
	// An IPv6 address may come in brackets, as in a URL.
	d.Address = strings.TrimSuffix(strings.TrimPrefix(d.Address, "["), "]")
	d.Group = strings.TrimSpace(d.Group)
	if d.Port == 0 {
		d.Port = sshconfig.DefaultRemoteDesktopPort(d.Protocol)
	}
	return d, config.ValidateRemoteDesktop(d)
}

// AddRemoteDesktop saves a new remote desktop reached without SSH. Its file
// is the chosen config file, or the main one.
func (a *App) AddRemoteDesktop(d config.RemoteDesktop) error {
	d, err := cleanRemoteDesktop(d)
	if err != nil {
		return err
	}
	if err := a.store.AddRemoteDesktop(d); err != nil {
		return err
	}
	a.remoteDesktopsChanged()
	return nil
}

// UpdateRemoteDesktop saves an edited remote desktop. A rename carries its
// login, certificate, consents and settings over to the new name.
func (a *App) UpdateRemoteDesktop(oldName string, d config.RemoteDesktop) error {
	before, ok := a.store.GetRemoteDesktop(oldName)
	if !ok {
		return &remoteDesktopNotFoundError{oldName}
	}
	d, err := cleanRemoteDesktop(d)
	if err != nil {
		return err
	}
	if err := a.store.UpdateRemoteDesktop(oldName, d); err != nil {
		return err
	}
	oldID, newID := config.RemoteDesktopID(oldName), config.RemoteDesktopID(d.Name)
	if before.Address != d.Address {
		// The consent was given for the old address.
		a.setVNCPublicConsent(oldID, false)
	}
	if before.Protocol != d.Protocol {
		// A login assigned for the other protocol no longer applies.
		a.credentialsMu.Lock()
		if err := a.setBinding(bindingKey(before.Protocol, oldID), ""); err != nil {
			slog.Warn("removing the login of the old protocol failed", "target", oldID, "error", err)
		}
		a.credentialsMu.Unlock()
	}
	if oldID != newID {
		a.moveRDPState(oldID, newID, nil)
		a.windowReg.RenameTunnel(oldID, newID, windows.Grouping(a.prefs.Get().WindowGrouping))
	}
	a.remoteDesktopsChanged()
	return nil
}

// DeleteRemoteDesktop removes a remote desktop, closes its viewers and forgets
// what was stored for it. A saved login it used stays in the saved logins.
func (a *App) DeleteRemoteDesktop(name string) error {
	id := config.RemoteDesktopID(name)
	if err := a.store.DeleteRemoteDesktop(name); err != nil {
		return err
	}
	for _, ref := range a.windowReg.TabsOfTunnel(id) {
		a.CloseTab(ref.WindowName, ref.TabID)
	}
	a.removeRDPState(id, nil)
	a.remoteDesktopsChanged()
	return nil
}

// DuplicateRemoteDesktop saves a copy of a remote desktop under a free name,
// in the same file. Logins and consents are not copied.
func (a *App) DuplicateRemoteDesktop(name string) (config.RemoteDesktop, error) {
	d, ok := a.store.GetRemoteDesktop(name)
	if !ok {
		return config.RemoteDesktop{}, &remoteDesktopNotFoundError{name}
	}
	taken := make(map[string]bool)
	for _, t := range a.store.GetTunnels() {
		taken[t.Name] = true
	}
	for _, other := range a.store.GetRemoteDesktops() {
		taken[other.Name] = true
	}
	d.Name = name + " copy"
	for n := 2; taken[d.Name]; n++ {
		d.Name = fmt.Sprintf("%s copy %d", name, n)
	}
	if err := a.store.AddRemoteDesktop(d); err != nil {
		return config.RemoteDesktop{}, err
	}
	a.remoteDesktopsChanged()
	copied, _ := a.store.GetRemoteDesktop(d.Name)
	return copied, nil
}

// CheckRemoteDesktops tries a TCP connection to each remote desktop, by ID, and
// reports which answered. It does not speak RDP or VNC, so the server keeps
// no trace of a session.
func (a *App) CheckRemoteDesktops(ids []string) map[string]bool {
	result := make(map[string]bool, len(ids))
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, id := range ids {
		d, err := a.remoteDesktop(id)
		if err != nil {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			reachable := checkTCP(remoteDesktopAddress(d), remoteDesktopCheckTimeout)
			mu.Lock()
			result[id] = reachable
			mu.Unlock()
		}()
	}
	wg.Wait()
	return result
}

// checkTCP reports whether something accepts TCP connections at address.
func checkTCP(address string, timeout time.Duration) bool {
	conn, err := net.DialTimeout("tcp", address, timeout)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// OpenRemoteDesktop opens the viewer of a remote desktop reached without SSH.
func (a *App) OpenRemoteDesktop(id string) error {
	d, err := a.remoteDesktop(id)
	if err != nil {
		return err
	}
	tool := windows.ToolRDP
	if d.Protocol == config.ForwardProtocolVNC {
		tool = windows.ToolVNC
	}
	_, err = a.OpenTool(windows.Spec{Tool: tool, TunnelID: d.ID})
	return err
}

// GetMainTab returns the main window's last tab.
func (a *App) GetMainTab() string {
	switch tab := a.prefs.Get().MainTab; tab {
	case mainTabClients, mainTabWeb:
		return tab
	}
	return mainTabTunnels
}

// SetMainTab remembers the main window's tab.
func (a *App) SetMainTab(tab string) error {
	switch tab {
	case mainTabTunnels, mainTabClients, mainTabWeb:
	default:
		return fmt.Errorf("unknown tab %q", tab)
	}
	p := a.prefs.Get()
	if p.MainTab == tab {
		return nil
	}
	p.MainTab = tab
	return a.prefs.Set(p)
}

func (a *App) remoteDesktopsChanged() {
	a.tray.RefreshMenu()
	a.emitTunnelsChanged()
}

// privateNetworks are where an unencrypted VNC connection goes without asking:
// private IPv4 ranges, the carrier-grade NAT range used by Tailscale, unique
// local IPv6 addresses and loopback.
var privateNetworks = func() []*net.IPNet {
	var nets []*net.IPNet
	for _, cidr := range []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "100.64.0.0/10", "fd00::/8", "127.0.0.0/8", "::1/128"} {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			panic(err)
		}
		nets = append(nets, network)
	}
	return nets
}()

// isPrivateIP reports whether an address is in one of privateNetworks.
func isPrivateIP(ip net.IP) bool {
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}
	for _, network := range privateNetworks {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

// publicAddress returns an address outside private networks that host
// resolves to, or "" when every address is private.
func publicAddress(ctx context.Context, host string) (string, error) {
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return "", fmt.Errorf("resolving %s: %w", host, err)
	}
	for _, addr := range addrs {
		if !isPrivateIP(addr.IP) {
			return addr.IP.String(), nil
		}
	}
	return "", nil
}

// setVNCPublicConsent stores or forgets the consent to unencrypted VNC to a
// public address for one remote desktop.
func (a *App) setVNCPublicConsent(key string, allowed bool) {
	a.rdpMu.Lock()
	defer a.rdpMu.Unlock()
	p := a.prefs.Get()
	if p.VNCPublicConsent[key] == allowed {
		return
	}
	p.VNCPublicConsent = withFlag(p.VNCPublicConsent, key, allowed)
	if err := a.prefs.Set(p); err != nil {
		slog.Warn("saving VNC consent failed", "target", key, "error", err)
	}
}
