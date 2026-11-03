package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"ssh-tunnel-manager/internal/keychain"
	"ssh-tunnel-manager/internal/rdp"
	"ssh-tunnel-manager/internal/ssh"
)

const (
	rdpProbeTimeout            = 4 * time.Second
	eventRDPCertificateChanged = "rdp:certificate-changed"
)

// rdpRemoteAddress is where RDP is reached: the host's loopback, through its
// SSH connection. This also covers servers bound to all interfaces.
var rdpRemoteAddress = net.JoinHostPort("127.0.0.1", fmt.Sprint(rdp.DefaultPort))

// RDPLogin is a login entered by the user. An empty Password means "use the
// stored login".
type RDPLogin struct {
	Username string `json:"username"`
	Domain   string `json:"domain"`
	Password string `json:"password"`
}

// RDPSavedLogin describes the stored login without exposing the password.
type RDPSavedLogin struct {
	Username    string `json:"username"`
	Domain      string `json:"domain"`
	HasPassword bool   `json:"hasPassword"`
}

// RDPSession is what the web client needs to open a session. It includes the
// password, because the client performs NLA in the webview.
type RDPSession struct {
	ID           string `json:"id"`
	ProxyAddress string `json:"proxyAddress"`
	Token        string `json:"token"`
	Destination  string `json:"destination"`
	Username     string `json:"username"`
	Domain       string `json:"domain"`
	Password     string `json:"password"`
}

// RDPCertificateChange tells the frontend that a host's certificate differs
// from the pinned one, so the user can decide whether to trust it.
type RDPCertificateChange struct {
	TunnelID string `json:"tunnelId"`
	Expected string `json:"expected"`
	Actual   string `json:"actual"`
}

// probeRDP reports whether an RDP server answers on the host. false with a nil
// error means the probe ran and found none. A non-nil error means it could not
// run, and must not be read as "no RDP".
func (a *App) probeRDP(tunnelID string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), rdpProbeTimeout)
	defer cancel()
	conn, err := a.manager.DialRemote(ctx, tunnelID, rdpRemoteAddress)
	if err != nil {
		if ssh.IsRemoteDialRefused(err) {
			return false, nil
		}
		return false, err
	}
	return rdp.Probe(ctx, conn)
}

// GetRDPLogin returns the stored login for a host, without its password.
func (a *App) GetRDPLogin(tunnelID string) (RDPSavedLogin, error) {
	credentials, found, err := keychain.GetRDPCredentials(tunnelID)
	if err != nil {
		return RDPSavedLogin{}, err
	}
	if !found {
		return RDPSavedLogin{}, nil
	}
	return RDPSavedLogin{Username: credentials.Username, Domain: credentials.Domain, HasPassword: credentials.Password != ""}, nil
}

// SaveRDPLogin stores a login in the OS keychain. The frontend calls this once
// the login has worked, so a mistyped password is never stored.
func (a *App) SaveRDPLogin(tunnelID string, login RDPLogin) error {
	return keychain.SetRDPCredentials(tunnelID, keychain.RDPCredentials{
		Username: login.Username,
		Domain:   login.Domain,
		Password: login.Password,
	})
}

// ForgetRDPLogin removes the stored login for a host.
func (a *App) ForgetRDPLogin(tunnelID string) error {
	return keychain.DeleteRDPCredentials(tunnelID)
}

// OpenRDP registers a bridge session to the host's RDP server. The tunnel must
// be connected, and the session uses its SSH connection.
func (a *App) OpenRDP(tunnelID string, login RDPLogin) (*RDPSession, error) {
	if _, ok := a.store.GetTunnel(tunnelID); !ok {
		return nil, &tunnelNotFoundError{tunnelID}
	}
	if login.Password == "" {
		stored, found, err := keychain.GetRDPCredentials(tunnelID)
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, errors.New("no stored RDP login for this host")
		}
		login = RDPLogin{Username: stored.Username, Domain: stored.Domain, Password: stored.Password}
	}

	session, err := a.rdpBridge.Open(rdp.Target{
		Address: rdpRemoteAddress,
		Dial: func(ctx context.Context) (net.Conn, error) {
			return a.manager.DialRemote(ctx, tunnelID, rdpRemoteAddress)
		},
		CheckCertificate: func(fingerprint string) error {
			return a.checkRDPCertificate(tunnelID, fingerprint)
		},
	})
	if err != nil {
		return nil, fmt.Errorf("opening RDP session for %s: %w", tunnelID, err)
	}
	return &RDPSession{
		ID:           session.ID,
		ProxyAddress: session.ProxyAddress,
		Token:        session.Token,
		Destination:  rdpRemoteAddress,
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
func (a *App) TrustRDPCertificate(tunnelID, fingerprint string) error {
	a.rdpMu.Lock()
	defer a.rdpMu.Unlock()
	return a.pinRDPCertificate(tunnelID, fingerprint)
}

// checkRDPCertificate trusts a host's certificate on first use, and refuses a
// different one afterwards until the user accepts it.
func (a *App) checkRDPCertificate(tunnelID, fingerprint string) error {
	a.rdpMu.Lock()
	defer a.rdpMu.Unlock()
	pinned := a.prefs.Get().RDPCertificates[tunnelID]
	switch pinned {
	case fingerprint:
		return nil
	case "":
		slog.Info("pinning RDP certificate on first use", "tunnel", tunnelID, "sha256", fingerprint)
		return a.pinRDPCertificate(tunnelID, fingerprint)
	}
	slog.Warn("RDP certificate changed", "tunnel", tunnelID, "expected", pinned, "actual", fingerprint)
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, eventRDPCertificateChanged, RDPCertificateChange{
			TunnelID: tunnelID,
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

// moveRDPState carries a host's RDP login, pinned certificate and detection
// result over to its new ID after a rename, so renaming neither loses the login
// nor resets certificate trust.
func (a *App) moveRDPState(oldID, newID string) {
	if credentials, found, err := keychain.GetRDPCredentials(oldID); err != nil {
		slog.Warn("reading RDP login during rename failed", "tunnel", oldID, "error", err)
	} else if found {
		if err := keychain.SetRDPCredentials(newID, credentials); err != nil {
			slog.Warn("moving RDP login failed", "tunnel", newID, "error", err)
		} else if err := keychain.DeleteRDPCredentials(oldID); err != nil {
			slog.Warn("removing old RDP login failed", "tunnel", oldID, "error", err)
		}
	}

	a.rdpMu.Lock()
	defer a.rdpMu.Unlock()
	p := a.prefs.Get()
	if pinned, ok := p.RDPCertificates[oldID]; ok {
		p.RDPCertificates = withRDPCertificate(withRDPCertificate(p.RDPCertificates, oldID, ""), newID, pinned)
	}
	if tools, ok := p.HostTools[oldID]; ok {
		p.HostTools = cloneHostTools(p.HostTools)
		delete(p.HostTools, oldID)
		p.HostTools[newID] = tools
	}
	if err := a.prefs.Set(p); err != nil {
		slog.Warn("moving RDP state failed", "tunnel", newID, "error", err)
	}
}

// removeRDPState forgets a deleted host's RDP login and pinned certificate.
func (a *App) removeRDPState(tunnelID string) {
	if err := keychain.DeleteRDPCredentials(tunnelID); err != nil {
		slog.Warn("removing RDP login failed", "tunnel", tunnelID, "error", err)
	}
	a.rdpMu.Lock()
	defer a.rdpMu.Unlock()
	p := a.prefs.Get()
	if _, ok := p.RDPCertificates[tunnelID]; !ok {
		return
	}
	p.RDPCertificates = withRDPCertificate(p.RDPCertificates, tunnelID, "")
	if err := a.prefs.Set(p); err != nil {
		slog.Warn("removing RDP certificate failed", "tunnel", tunnelID, "error", err)
	}
}
