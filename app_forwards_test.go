package main

import (
	"testing"

	"ssh-tunnel-manager/internal/config"
)

func TestFindForwardRequiresConfiguredForwardWithProtocol(t *testing.T) {
	cfg := config.TunnelConfig{Name: "bastion", PortForwards: []config.PortForward{
		{RemoteHost: "10.0.0.5", RemotePort: 3389},
		{RemoteHost: "10.0.0.6", RemotePort: 3389, Protocol: config.ForwardProtocolNone},
		{RemoteHost: "10.0.0.7", RemotePort: 8080, Protocol: config.ForwardProtocolRDP},
		{RemoteHost: "10.0.0.8", RemotePort: 22},
	}}
	cases := []struct {
		host     string
		port     int
		protocol string
		found    bool
	}{
		{"10.0.0.5", 3389, config.ForwardProtocolRDP, true},
		{"10.0.0.7", 8080, config.ForwardProtocolRDP, true},
		{"10.0.0.8", 22, config.ForwardProtocolSSH, true},
		{"10.0.0.6", 3389, config.ForwardProtocolRDP, false},
		{"10.0.0.5", 3389, config.ForwardProtocolSSH, false},
		{"10.9.9.9", 3389, config.ForwardProtocolRDP, false},
		{"10.0.0.5", 3390, config.ForwardProtocolRDP, false},
	}
	for _, c := range cases {
		_, err := findForward(cfg, ForwardTarget{RemoteHost: c.host, RemotePort: c.port}, c.protocol)
		if (err == nil) != c.found {
			t.Errorf("%s:%d %s: err = %v", c.host, c.port, c.protocol, err)
		}
	}
}

func TestForwardSSHConfig(t *testing.T) {
	cfg := config.TunnelConfig{Host: "bastion", Port: 22, User: "jump", KeyPath: "/key", ProxyJump: "other", ProxyCommand: "cmd",
		PortForwards: []config.PortForward{{RemoteHost: "10.0.0.8", RemotePort: 2222}}}

	got := forwardSSHConfig(cfg, config.PortForward{RemoteHost: "10.0.0.8", RemotePort: 2222})
	if got.Host != "10.0.0.8" || got.Port != 2222 || got.User != "jump" || got.KeyPath != "/key" {
		t.Fatalf("got %+v", got)
	}
	if got.ProxyJump != "" || got.ProxyCommand != "" || got.PortForwards != nil {
		t.Fatalf("the tunnel's route leaked into the target: %+v", got)
	}
	if got := forwardSSHConfig(cfg, config.PortForward{RemoteHost: "h", RemotePort: 22, SSHUser: "inner"}); got.User != "inner" {
		t.Fatalf("user = %q", got.User)
	}
}

func TestValidateForwardProtocols(t *testing.T) {
	for _, pf := range []config.PortForward{{}, {Protocol: "rdp"}, {Protocol: "ssh", SSHUser: "inner"}, {Protocol: "none"}} {
		if err := validateForwardProtocols([]config.PortForward{pf}); err != nil {
			t.Errorf("%+v: %v", pf, err)
		}
	}
	for _, pf := range []config.PortForward{{Protocol: "vnc"}, {SSHUser: "a b"}, {SSHUser: "a\nHost evil"}} {
		if err := validateForwardProtocols([]config.PortForward{pf}); err == nil {
			t.Errorf("%+v should be rejected", pf)
		}
	}
}

func TestRDPForwardKeyDiffersFromHostKey(t *testing.T) {
	first := rdpForwardKey("bastion", ForwardTarget{RemoteHost: "10.0.0.5", RemotePort: 3389})
	second := rdpForwardKey("bastion", ForwardTarget{RemoteHost: "10.0.0.6", RemotePort: 3389})
	if first == "bastion" || first == second {
		t.Fatalf("keys must not collide: %q %q", first, second)
	}
}
