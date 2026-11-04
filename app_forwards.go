package main

import (
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"

	"ssh-tunnel-manager/internal/config"
)

// ForwardTarget names a port forward by its remote end. The zero value means
// the host itself rather than a forward.
type ForwardTarget struct {
	RemoteHost string `json:"remoteHost"`
	RemotePort int    `json:"remotePort"`
}

func (t ForwardTarget) address() string {
	return net.JoinHostPort(t.RemoteHost, strconv.Itoa(t.RemotePort))
}

// findForward returns the host's forward to target, which must have the given
// protocol. Actions on a forward go through this check, so the frontend can only
// reach machines the user configured.
func findForward(cfg config.TunnelConfig, target ForwardTarget, protocol string) (config.PortForward, error) {
	for _, pf := range cfg.PortForwards {
		if pf.RemoteHost != target.RemoteHost || pf.RemotePort != target.RemotePort {
			continue
		}
		if pf.EffectiveProtocol() != protocol {
			continue
		}
		return pf, nil
	}
	return config.PortForward{}, fmt.Errorf("%s has no %s forward to %s", cfg.Name, protocol, target.address())
}

// forwardSSHConfig describes the SSH connection to a forward's target. It reuses
// the tunnel's user and key unless the forward names another user.
func forwardSSHConfig(cfg config.TunnelConfig, pf config.PortForward) config.TunnelConfig {
	target := cfg
	target.Host = pf.RemoteHost
	target.Port = pf.RemotePort
	target.ProxyCommand = ""
	target.ProxyJump = ""
	target.PortForwards = nil
	if pf.SSHUser != "" {
		target.User = pf.SSHUser
	}
	return target
}

// validateForwardProtocols rejects values that would corrupt the SSH config when
// written as comments.
func validateForwardProtocols(forwards []config.PortForward) error {
	for _, pf := range forwards {
		switch pf.Protocol {
		case "", config.ForwardProtocolRDP, config.ForwardProtocolSSH, config.ForwardProtocolNone:
		default:
			return fmt.Errorf("unknown forward protocol %q", pf.Protocol)
		}
		if strings.ContainsAny(pf.SSHUser, " \t\r\n") {
			return fmt.Errorf("SSH user of a forward cannot contain whitespace")
		}
	}
	return nil
}

func (a *App) sshForward(tunnelID string, target ForwardTarget) (config.TunnelConfig, error) {
	cfg, ok := a.store.GetTunnel(tunnelID)
	if !ok {
		return config.TunnelConfig{}, &tunnelNotFoundError{tunnelID}
	}
	pf, err := findForward(cfg, target, config.ForwardProtocolSSH)
	if err != nil {
		return config.TunnelConfig{}, err
	}
	return forwardSSHConfig(cfg, pf), nil
}

// OpenForwardSFTP opens a file browser session on an SSH forward's target,
// through the tunnel.
func (a *App) OpenForwardSFTP(tunnelID string, target ForwardTarget) (*SFTPOpenResult, error) {
	cfg, err := a.sshForward(tunnelID, target)
	if err != nil {
		return nil, err
	}
	sessionID, home, err := a.sftpMgr.OpenSessionVia(cfg, a.manager.RemoteDialerFor(tunnelID))
	if err != nil {
		return nil, fmt.Errorf("opening SFTP on %s: %w", target.address(), err)
	}
	return &SFTPOpenResult{SessionID: sessionID, Home: home}, nil
}

// OpenForwardTerminal opens a shell on an SSH forward's target, through the
// tunnel.
func (a *App) OpenForwardTerminal(tunnelID string, target ForwardTarget) (string, error) {
	cfg, err := a.sshForward(tunnelID, target)
	if err != nil {
		return "", err
	}
	sessionID, err := a.termMgr.OpenSessionVia(cfg, a.manager.RemoteDialerFor(tunnelID),
		func(sessionID, data string) {
			if a.ctx != nil {
				a.emitToSession(sessionID, "terminal:output", map[string]any{"sessionId": sessionID, "data": data})
			}
		},
		func(sessionID string) {
			if a.ctx != nil {
				a.emitToSession(sessionID, "terminal:closed", map[string]any{"sessionId": sessionID})
			}
		},
	)
	if err != nil {
		return "", fmt.Errorf("opening terminal on %s: %w", target.address(), err)
	}
	return sessionID, nil
}

var portlessSuffixPattern = regexp.MustCompile(`^[a-z0-9-]+(\.[a-z0-9-]+)*$`)

// validatePortlessSuffix rejects a suffix that is no valid part of a domain, or
// could not be written back to the SSH config as one comment line.
func validatePortlessSuffix(suffix string) error {
	if suffix != "" && !portlessSuffixPattern.MatchString(suffix) {
		return fmt.Errorf("invalid portless suffix %q: use lowercase letters, digits, dashes and dots", suffix)
	}
	return nil
}
