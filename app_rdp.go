package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"time"

	"ssh-tunnel-manager/internal/config"
	"ssh-tunnel-manager/internal/keychain"
	"ssh-tunnel-manager/internal/prefs"
	"ssh-tunnel-manager/internal/rdp"
	"ssh-tunnel-manager/internal/ssh"
)

const (
	rdpProbeTimeout            = 4 * time.Second
	eventRDPCertificateChanged = "rdp:certificate-changed"
)

// rdpRemoteAddress is where a host's RDP server is reached: its loopback,
// through its SSH connection. This also covers servers bound to all interfaces.
// Only the configured port is used; other ports are never scanned.
func rdpRemoteAddress(cfg config.TunnelConfig) string {
	port := cfg.RDPPort
	if port == 0 {
		port = rdp.DefaultPort
	}
	return net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
}

// validateRDPPort rejects a port that could not be written back to the config.
func validateRDPPort(port int) error {
	if port < 0 || port > 65535 {
		return fmt.Errorf("RDP port must be between 1 and 65535, got %d", port)
	}
	return nil
}

// RDPLogin is a login entered by the user. An empty Password means "use the
// stored login".
type RDPLogin struct {
	Username string `json:"username"`
	Domain   string `json:"domain"`
	Password string `json:"password"`
	// CredentialID uses a saved login for this session, whatever the target has
	// assigned. It applies when Password is empty.
	CredentialID string `json:"credentialId"`
}

// RDPSavedLogin describes the stored login without exposing the password.
type RDPSavedLogin struct {
	Username       string `json:"username"`
	Domain         string `json:"domain"`
	HasPassword    bool   `json:"hasPassword"`
	CredentialID   string `json:"credentialId"`
	CredentialName string `json:"credentialName"`
}

// RDPSession is what the web client needs to open a session. It includes the
// password, because the client performs NLA in the webview.
type RDPSession struct {
	ID           string `json:"id"`
	StateKey     string `json:"stateKey"`
	ProxyAddress string `json:"proxyAddress"`
	Token        string `json:"token"`
	Destination  string `json:"destination"`
	Username     string `json:"username"`
	Domain       string `json:"domain"`
	Password     string `json:"password"`
}

// RDPCertificateChange tells the frontend that a server's certificate differs
// from the pinned one, so the user can decide whether to trust it.
type RDPCertificateChange struct {
	StateKey string `json:"stateKey"`
	Expected string `json:"expected"`
	Actual   string `json:"actual"`
	// FirstUse is true for the first connection to a remote desktop reached
	// without SSH, whose certificate the user confirms before it is trusted.
	FirstUse bool `json:"firstUse"`
}

// probeRDP reports whether an RDP server answers on the host. false with a nil
// error means the probe ran and found none. A non-nil error means it could not
// run, and must not be read as "no RDP".
func (a *App) probeRDP(tunnelID string) (bool, error) {
	cfg, ok := a.store.GetTunnel(tunnelID)
	if !ok {
		return false, &tunnelNotFoundError{tunnelID}
	}
	ctx, cancel := context.WithTimeout(context.Background(), rdpProbeTimeout)
	defer cancel()
	conn, err := a.manager.DialRemote(ctx, tunnelID, rdpRemoteAddress(cfg))
	if err != nil {
		if ssh.IsRemoteDialRefused(err) {
			return false, nil
		}
		return false, err
	}
	return rdp.Probe(ctx, conn)
}

// resolveRDPTarget returns the key under which a target's login and certificate
// are stored, and the address it is reached at through the tunnel. An empty
// target means the host's own RDP server. Any other target must be a forward
// configured on the host, so the frontend cannot open arbitrary connections.
func (a *App) resolveRDPTarget(tunnelID string, target ForwardTarget) (string, string, error) {
	if isRemoteDesktopID(tunnelID) {
		return a.resolveRemoteDesktop(tunnelID, target, config.ForwardProtocolRDP)
	}
	cfg, ok := a.store.GetTunnel(tunnelID)
	if !ok {
		return "", "", &tunnelNotFoundError{tunnelID}
	}
	if target.RemoteHost == "" {
		return tunnelID, rdpRemoteAddress(cfg), nil
	}
	if _, err := findForward(cfg, target, config.ForwardProtocolRDP); err != nil {
		return "", "", err
	}
	return forwardStateKey(tunnelID, target), target.address(), nil
}

// forwardStateKey is the storage key of a forward target's login and certificate.
func forwardStateKey(tunnelID string, target ForwardTarget) string {
	return tunnelID + ">" + target.address()
}

// GetRDPLogin returns the login assigned to a target, without its password.
func (a *App) GetRDPLogin(tunnelID string, target ForwardTarget) (RDPSavedLogin, error) {
	key, _, err := a.resolveRDPTarget(tunnelID, target)
	if err != nil {
		return RDPSavedLogin{}, err
	}
	id, login, found, err := a.boundLogin(config.ForwardProtocolRDP, key)
	if err != nil {
		return RDPSavedLogin{}, err
	}
	if !found {
		return RDPSavedLogin{}, nil
	}
	return RDPSavedLogin{
		Username:       login.Username,
		Domain:         login.Domain,
		HasPassword:    login.Password != "",
		CredentialID:   id,
		CredentialName: a.credentialName(id),
	}, nil
}

// SaveRDPLogin stores a login that has just worked, so a mistyped password is
// never stored. It updates the login assigned to the target, or saves a new one
// named after the target and assigns it.
func (a *App) SaveRDPLogin(tunnelID string, target ForwardTarget, login RDPLogin) error {
	key, _, err := a.resolveRDPTarget(tunnelID, target)
	if err != nil {
		return err
	}
	return a.saveTargetLogin(config.ForwardProtocolRDP, key,
		keychain.Login{Username: login.Username, Domain: login.Domain, Password: login.Password})
}

// ForgetRDPLogin removes the assignment of a login to a target. The login itself
// stays in the credentials store.
func (a *App) ForgetRDPLogin(tunnelID string, target ForwardTarget) error {
	key, _, err := a.resolveRDPTarget(tunnelID, target)
	if err != nil {
		return err
	}
	a.credentialsMu.Lock()
	defer a.credentialsMu.Unlock()
	return a.setBinding(bindingKey(config.ForwardProtocolRDP, key), "")
}

// OpenRDP registers a bridge session to an RDP server: the host's own, the
// one behind a forward, or a remote desktop reached without SSH. Through SSH
// the tunnel must be connected, and the session uses its SSH connection, so a
// forward's target is reached from the host.
func (a *App) OpenRDP(tunnelID string, target ForwardTarget, login RDPLogin) (*RDPSession, error) {
	key, address, err := a.resolveRDPTarget(tunnelID, target)
	if err != nil {
		return nil, err
	}
	if login.Password == "" {
		stored, err := a.storedLogin(config.ForwardProtocolRDP, key, login.CredentialID)
		if err != nil {
			return nil, err
		}
		login = RDPLogin{Username: stored.Username, Domain: stored.Domain, Password: stored.Password}
	}

	session, err := a.rdpBridge.Open(rdp.Target{
		Address: address,
		Dial:    a.dialTarget(tunnelID, address),
		CheckCertificate: func(fingerprint string) error {
			return a.checkRDPCertificate(key, fingerprint)
		},
	})
	if err != nil {
		return nil, fmt.Errorf("opening RDP session for %s: %w", key, err)
	}
	return &RDPSession{
		ID:           session.ID,
		StateKey:     key,
		ProxyAddress: session.ProxyAddress,
		Token:        session.Token,
		Destination:  address,
		Username:     login.Username,
		Domain:       login.Domain,
		Password:     login.Password,
	}, nil
}

// CloseRDP ends a bridge session.
func (a *App) CloseRDP(sessionID string) {
	a.rdpBridge.CloseSession(sessionID)
}

// TrustRDPCertificate pins a new certificate after the user accepted a change.
func (a *App) TrustRDPCertificate(tunnelID string, target ForwardTarget, fingerprint string) error {
	key, _, err := a.resolveRDPTarget(tunnelID, target)
	if err != nil {
		return err
	}
	a.rdpMu.Lock()
	defer a.rdpMu.Unlock()
	return a.pinRDPCertificate(key, fingerprint)
}

// checkRDPCertificate trusts a host's certificate on first use, and refuses a
// different one afterwards until the user accepts it. A remote desktop reached
// without SSH has no SSH host key behind it, so even its first certificate
// waits for the user to compare the fingerprint.
func (a *App) checkRDPCertificate(stateKey, fingerprint string) error {
	a.rdpMu.Lock()
	defer a.rdpMu.Unlock()
	pinned := a.prefs.Get().RDPCertificates[stateKey]
	switch {
	case pinned == fingerprint:
		return nil
	case pinned == "" && isRemoteDesktopID(stateKey):
		slog.Info("RDP certificate awaits confirmation", "target", stateKey, "sha256", fingerprint)
		if a.ctx != nil {
			a.emit(eventRDPCertificateChanged, RDPCertificateChange{StateKey: stateKey, Actual: fingerprint, FirstUse: true})
		}
		return rdp.ErrCertificateUnconfirmed
	case pinned == "":
		slog.Info("pinning RDP certificate on first use", "target", stateKey, "sha256", fingerprint)
		return a.pinRDPCertificate(stateKey, fingerprint)
	}
	slog.Warn("RDP certificate changed", "target", stateKey, "expected", pinned, "actual", fingerprint)
	if a.ctx != nil {
		a.emit(eventRDPCertificateChanged, RDPCertificateChange{
			StateKey: stateKey,
			Expected: pinned,
			Actual:   fingerprint,
		})
	}
	return rdp.ErrCertificateChanged
}

// pinRDPCertificate stores a fingerprint. The caller holds rdpMu.
func (a *App) pinRDPCertificate(tunnelID, fingerprint string) error {
	p := a.prefs.Get()
	p.RDPCertificates = withRDPCertificate(p.RDPCertificates, tunnelID, fingerprint)
	if err := a.prefs.Set(p); err != nil {
		return fmt.Errorf("saving RDP certificate for %s: %w", tunnelID, err)
	}
	return nil
}

// withRDPCertificate returns a copy of certificates with one entry changed, so
// the map shared with the prefs store is never mutated. An empty fingerprint
// removes the entry.
func withRDPCertificate(certificates map[string]string, tunnelID, fingerprint string) map[string]string {
	updated := make(map[string]string, len(certificates)+1)
	for id, value := range certificates {
		updated[id] = value
	}
	if fingerprint == "" {
		delete(updated, tunnelID)
	} else {
		updated[tunnelID] = fingerprint
	}
	return updated
}

// moveRDPState carries RDP logins, pinned certificates and the detection result
// over to a host's new ID after a rename, so renaming neither loses a login nor
// resets certificate trust.
func (a *App) moveRDPState(oldID, newID string, forwards []config.PortForward) {
	moves := map[string]string{oldID: newID}
	for _, pf := range forwards {
		target := ForwardTarget{RemoteHost: pf.RemoteHost, RemotePort: pf.RemotePort}
		moves[forwardStateKey(oldID, target)] = forwardStateKey(newID, target)
	}
	a.rdpMu.Lock()
	defer a.rdpMu.Unlock()
	p := a.prefs.Get()
	for from, to := range moves {
		if pinned, ok := p.RDPCertificates[from]; ok {
			p.RDPCertificates = withRDPCertificate(withRDPCertificate(p.RDPCertificates, from, ""), to, pinned)
		}
		if p.VNCFullColor[from] {
			p.VNCFullColor = withFlag(withFlag(p.VNCFullColor, from, false), to, true)
		}
		if p.VNCPublicConsent[from] {
			p.VNCPublicConsent = withFlag(withFlag(p.VNCPublicConsent, from, false), to, true)
		}
	}
	if tools, ok := p.HostTools[oldID]; ok {
		p.HostTools = cloneHostTools(p.HostTools)
		delete(p.HostTools, oldID)
		p.HostTools[newID] = tools
	}
	if pinned, ok := p.SSHHostKeys[oldID]; ok {
		p.SSHHostKeys = withSSHHostKeys(withSSHHostKeys(p.SSHHostKeys, oldID, nil), newID, pinned)
	}
	p.CredentialBindings = moveCredentialBindings(p.CredentialBindings, moves)
	if settings, ok := p.Tunnels[oldID]; ok {
		tunnels := make(map[string]prefs.TunnelSettings, len(p.Tunnels))
		for id, value := range p.Tunnels {
			tunnels[id] = value
		}
		delete(tunnels, oldID)
		tunnels[newID] = settings
		p.Tunnels = tunnels
	}
	if err := a.prefs.Set(p); err != nil {
		slog.Warn("moving RDP state failed", "target", newID, "error", err)
	}
}

// removeRDPState forgets the RDP logins and pinned certificates of a deleted
// host and of its forwards.
func (a *App) removeRDPState(tunnelID string, forwards []config.PortForward) {
	keys := []string{tunnelID}
	for _, pf := range forwards {
		keys = append(keys, forwardStateKey(tunnelID, ForwardTarget{RemoteHost: pf.RemoteHost, RemotePort: pf.RemotePort}))
	}
	for _, key := range keys {
		if err := keychain.DeleteRDPCredentials(key); err != nil {
			slog.Warn("removing RDP login failed", "target", key, "error", err)
		}
	}
	a.rdpMu.Lock()
	defer a.rdpMu.Unlock()
	p := a.prefs.Get()
	changed := false
	for _, key := range keys {
		if _, ok := p.RDPCertificates[key]; ok {
			p.RDPCertificates = withRDPCertificate(p.RDPCertificates, key, "")
			changed = true
		}
		if p.VNCFullColor[key] {
			p.VNCFullColor = withFlag(p.VNCFullColor, key, false)
			changed = true
		}
		if p.VNCPublicConsent[key] {
			p.VNCPublicConsent = withFlag(p.VNCPublicConsent, key, false)
			changed = true
		}
	}
	if _, ok := p.SSHHostKeys[tunnelID]; ok {
		p.SSHHostKeys = withSSHHostKeys(p.SSHHostKeys, tunnelID, nil)
		changed = true
	}
	if updated, removed := removeCredentialBindings(p.CredentialBindings, keys); removed {
		p.CredentialBindings = updated
		changed = true
	}
	if _, ok := p.Tunnels[tunnelID]; ok {
		tunnels := make(map[string]prefs.TunnelSettings, len(p.Tunnels))
		for id, value := range p.Tunnels {
			if id != tunnelID {
				tunnels[id] = value
			}
		}
		p.Tunnels = tunnels
		changed = true
	}
	if !changed {
		return
	}
	if err := a.prefs.Set(p); err != nil {
		slog.Warn("removing RDP certificates failed", "tunnel", tunnelID, "error", err)
	}
}
