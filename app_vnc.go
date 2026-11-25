package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"time"

	gossh "golang.org/x/crypto/ssh"

	"ssh-tunnel-manager/internal/config"
	"ssh-tunnel-manager/internal/keychain"
	"ssh-tunnel-manager/internal/ssh"
	"ssh-tunnel-manager/internal/vnc"
)

const (
	vncProbeTimeout   = 4 * time.Second
	vncConnectTimeout = 20 * time.Second
)

// Outcomes of OpenVNC that the viewer answers with a login form.
const (
	vncConnected     = "connected"
	vncLoginRequired = "login-required"
	vncLoginFailed   = "login-failed"
	// vncConfirmPublic asks the user to allow an unencrypted connection to an
	// address outside private networks.
	vncConfirmPublic = "confirm-public"
)

// vncHostAddress is where a host's own VNC server, such as macOS Screen
// Sharing, is reached: its loopback, through its SSH connection. A server on
// another port is opened through a forward with the vnc protocol.
func vncHostAddress() string {
	return net.JoinHostPort("127.0.0.1", strconv.Itoa(vnc.DefaultPort))
}

// VNCLogin is the login a viewer connects with. An empty Password means "use
// the chosen saved login", or the one assigned to the target, or none.
type VNCLogin struct {
	// Username is the macOS account for Apple Remote Desktop authentication.
	// Leave it empty for a server that asks only for a VNC password.
	Username     string `json:"username"`
	Password     string `json:"password"`
	CredentialID string `json:"credentialId"`
	// Save keeps a typed login in the credentials, or assigns the chosen saved
	// login to the target, once the server has accepted it.
	Save bool `json:"save"`
	// AllowPublic connects a remote desktop without SSH to an address outside
	// private networks this once; RememberPublic stops asking for it.
	AllowPublic    bool `json:"allowPublic"`
	RememberPublic bool `json:"rememberPublic"`
}

// VNCSavedLogin describes the login assigned to a target without its password.
type VNCSavedLogin struct {
	Username       string `json:"username"`
	HasPassword    bool   `json:"hasPassword"`
	CredentialID   string `json:"credentialId"`
	CredentialName string `json:"credentialName"`
}

// VNCConnection is the outcome of OpenVNC. Status is "connected" with a
// session, or asks for a login. Other failures are returned as errors.
type VNCConnection struct {
	Status string `json:"status"`
	// Message explains a rejected login, in the server's words when it gave any.
	Message string `json:"message,omitempty"`
	// NeedsUsername is true when the server accepts only Apple Remote Desktop
	// authentication, which takes the Mac's account name and password.
	NeedsUsername bool   `json:"needsUsername,omitempty"`
	SessionID     string `json:"sessionId,omitempty"`
	ProxyAddress  string `json:"proxyAddress,omitempty"`
	// Apple is true for macOS Screen Sharing, whose colour depth the viewer
	// can switch.
	Apple bool `json:"apple,omitempty"`
	// ReducedColor is true when the session shows 16-bit colour.
	ReducedColor bool `json:"reducedColor,omitempty"`
	// PublicAddress is the address outside private networks a "confirm-public"
	// outcome asks about.
	PublicAddress string `json:"publicAddress,omitempty"`
}

// probeVNC reports whether a VNC server answers on the host's port 5900. false
// with a nil error means the probe ran and found none. A non-nil error means it
// could not run, and must not be read as "no VNC".
func (a *App) probeVNC(tunnelID string) (bool, error) {
	if _, ok := a.store.GetTunnel(tunnelID); !ok {
		return false, &tunnelNotFoundError{tunnelID}
	}
	ctx, cancel := context.WithTimeout(context.Background(), vncProbeTimeout)
	defer cancel()
	conn, err := a.manager.DialRemote(ctx, tunnelID, vncHostAddress())
	if err != nil {
		if ssh.IsRemoteDialRefused(err) {
			return false, nil
		}
		return false, err
	}
	return vnc.Probe(ctx, conn)
}

// resolveVNCTarget returns the key under which a target's login is stored and
// the address it is reached at through the tunnel. An empty target means the
// host's own VNC server. Any other target must be a vnc forward configured on
// the host, so the frontend cannot open arbitrary connections.
func (a *App) resolveVNCTarget(tunnelID string, target ForwardTarget) (string, string, error) {
	if isRemoteDesktopID(tunnelID) {
		return a.resolveRemoteDesktop(tunnelID, target, config.ForwardProtocolVNC)
	}
	cfg, ok := a.store.GetTunnel(tunnelID)
	if !ok {
		return "", "", &tunnelNotFoundError{tunnelID}
	}
	if target.RemoteHost == "" {
		return tunnelID, vncHostAddress(), nil
	}
	if _, err := findForward(cfg, target, config.ForwardProtocolVNC); err != nil {
		return "", "", err
	}
	return forwardStateKey(tunnelID, target), target.address(), nil
}

// GetVNCLogin returns the login assigned to a target, without its password.
func (a *App) GetVNCLogin(tunnelID string, target ForwardTarget) (VNCSavedLogin, error) {
	key, _, err := a.resolveVNCTarget(tunnelID, target)
	if err != nil {
		return VNCSavedLogin{}, err
	}
	id, login, found, err := a.boundLogin(config.ForwardProtocolVNC, key)
	if err != nil || !found {
		return VNCSavedLogin{}, err
	}
	return VNCSavedLogin{
		Username:       login.Username,
		HasPassword:    login.Password != "",
		CredentialID:   id,
		CredentialName: a.credentialName(id),
	}, nil
}

// vncCredentials picks the login a connection uses: the typed one, the chosen
// saved one, the one assigned to the target, or none.
func (a *App) vncCredentials(key string, login VNCLogin) (vnc.Credentials, error) {
	if login.Password != "" {
		return vnc.Credentials{Username: login.Username, Password: login.Password}, nil
	}
	if login.CredentialID == "" {
		_, stored, found, err := a.boundLogin(config.ForwardProtocolVNC, key)
		if err != nil || !found {
			// No login: a server without authentication still connects.
			return vnc.Credentials{}, err
		}
		return vnc.Credentials{Username: stored.Username, Password: stored.Password}, nil
	}
	stored, err := a.storedLogin(config.ForwardProtocolVNC, key, login.CredentialID)
	if err != nil {
		return vnc.Credentials{}, err
	}
	return vnc.Credentials{Username: stored.Username, Password: stored.Password}, nil
}

// OpenVNC connects to a VNC server, the host's own, the one behind a forward,
// or a remote desktop reached without SSH, and authenticates in the app, so
// the password never reaches the webview. It then registers a bridge session
// for the web client. Through SSH the tunnel must be connected; the session
// uses its SSH connection.
func (a *App) OpenVNC(tunnelID string, target ForwardTarget, login VNCLogin) (*VNCConnection, error) {
	key, address, err := a.resolveVNCTarget(tunnelID, target)
	if err != nil {
		return nil, err
	}
	credentials, err := a.vncCredentials(key, login)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), vncConnectTimeout)
	defer cancel()
	if isRemoteDesktopID(tunnelID) {
		if confirm, err := a.confirmPublicVNC(ctx, key, address, login); confirm != nil || err != nil {
			return confirm, err
		}
	}
	dial := a.dialTarget(tunnelID, address)
	session, err := a.vncBridge.Open(ctx, vnc.Target{
		Dial: func(ctx context.Context) (net.Conn, error) {
			conn, err := dial(ctx)
			var rejected *gossh.OpenChannelError
			if (errors.As(err, &rejected) && rejected.Reason == gossh.ConnectionFailed) || isConnectionRefused(err) {
				return nil, fmt.Errorf("no VNC server accepts connections there: %w", err)
			}
			return conn, err
		},
		Credentials: credentials,
		FullColor:   a.prefs.Get().VNCFullColor[key],
	})
	if err != nil {
		return vncLoginOutcome(err, credentials, address)
	}
	// A server without authentication never checked the login, so it may be
	// mistyped and is not kept.
	// Saving may wait on the keychain, which must not hold up the session
	// whose token is already running.
	if login.Save && session.Authenticated {
		go a.keepVNCLogin(key, login)
	}
	return &VNCConnection{
		Status:       vncConnected,
		SessionID:    session.ID,
		ProxyAddress: session.ProxyAddress,
		Apple:        session.Apple,
		ReducedColor: session.ReducedColor,
	}, nil
}

// confirmPublicVNC asks before an unencrypted VNC connection without SSH goes
// to an address outside private networks, unless the user allowed it this
// time or for good. It returns nil when the connection may go ahead.
func (a *App) confirmPublicVNC(ctx context.Context, key, address string, login VNCLogin) (*VNCConnection, error) {
	if a.prefs.Get().VNCPublicConsent[key] {
		return nil, nil
	}
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	public, err := publicAddress(ctx, host)
	if err != nil || public == "" {
		return nil, err
	}
	if !login.AllowPublic {
		return &VNCConnection{Status: vncConfirmPublic, PublicAddress: public}, nil
	}
	if login.RememberPublic {
		a.setVNCPublicConsent(key, true)
	}
	return nil, nil
}

// SetVNCFullColor decides whether a target's macOS Screen Sharing sessions
// keep 32-bit colour. It applies from the next connection.
func (a *App) SetVNCFullColor(tunnelID string, target ForwardTarget, full bool) error {
	key, _, err := a.resolveVNCTarget(tunnelID, target)
	if err != nil {
		return err
	}
	a.rdpMu.Lock()
	defer a.rdpMu.Unlock()
	p := a.prefs.Get()
	p.VNCFullColor = withFlag(p.VNCFullColor, key, full)
	if err := a.prefs.Set(p); err != nil {
		return fmt.Errorf("saving the colour choice for %s: %w", key, err)
	}
	return nil
}

// withFlag returns a copy of flags with one key set or, for false, removed, so
// the map shared with the prefs store is never mutated.
func withFlag(flags map[string]bool, key string, on bool) map[string]bool {
	updated := make(map[string]bool, len(flags)+1)
	for k, v := range flags {
		updated[k] = v
	}
	if on {
		updated[key] = true
	} else {
		delete(updated, key)
	}
	return updated
}

// vncLoginOutcome turns a failed handshake into a request for a login when the
// login is what failed, and into an error otherwise.
func vncLoginOutcome(err error, credentials vnc.Credentials, address string) (*VNCConnection, error) {
	var required *vnc.CredentialsRequiredError
	if errors.As(err, &required) {
		message := ""
		if credentials.Password != "" {
			// A password alone does not do for Apple Remote Desktop.
			message = "This server needs the user name of an account on it as well as the password."
		}
		return &VNCConnection{Status: vncLoginRequired, Message: message, NeedsUsername: required.Username}, nil
	}
	var rejected *vnc.AuthError
	if errors.As(err, &rejected) {
		message := "The server rejected this login."
		if rejected.Reason != "" {
			message = "The server rejected this login: " + rejected.Reason
		}
		return &VNCConnection{Status: vncLoginFailed, Message: message}, nil
	}
	return nil, fmt.Errorf("connecting to the VNC server at %s: %w", address, err)
}

// keepVNCLogin saves a typed login that has just worked, or assigns the chosen
// saved login to the target. A failure is logged, because the session itself
// is open.
func (a *App) keepVNCLogin(key string, login VNCLogin) {
	if login.Password != "" {
		// VNC has no domain; one the assigned login has for another use stays.
		domain := ""
		if _, stored, found, err := a.boundLogin(config.ForwardProtocolVNC, key); err == nil && found {
			domain = stored.Domain
		}
		if err := a.saveTargetLogin(config.ForwardProtocolVNC, key,
			keychain.Login{Username: login.Username, Domain: domain, Password: login.Password}); err != nil {
			logBindingFailure(key, err)
		}
		return
	}
	if login.CredentialID == "" {
		return
	}
	a.credentialsMu.Lock()
	defer a.credentialsMu.Unlock()
	if err := a.setBinding(bindingKey(config.ForwardProtocolVNC, key), login.CredentialID); err != nil {
		logBindingFailure(key, err)
		return
	}
	if err := a.addCredentialProtocol(login.CredentialID, config.ForwardProtocolVNC); err != nil {
		logBindingFailure(key, err)
	}
	a.emit(eventSettingsChange)
}

// CloseVNC ends a bridge session.
func (a *App) CloseVNC(sessionID string) {
	if a.vncBridge != nil {
		a.vncBridge.CloseSession(sessionID)
	}
}
