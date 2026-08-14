//go:build darwin

package main

import (
	"testing"

	"ssh-tunnel-manager/internal/config"
)

func TestTunnelRequiresPrivilegedPortRedirect(t *testing.T) {
	tests := []struct {
		name string
		cfg  config.TunnelConfig
		want bool
	}{
		{
			name: "portless remote port 80",
			cfg: config.TunnelConfig{PortForwards: []config.PortForward{
				{Portless: true, RemotePort: 80},
			}},
			want: true,
		},
		{
			name: "portless expose port overrides high remote port",
			cfg: config.TunnelConfig{PortForwards: []config.PortForward{
				{Portless: true, RemotePort: 8080, ExposePort: 80},
			}},
			want: true,
		},
		{
			name: "high port",
			cfg: config.TunnelConfig{PortForwards: []config.PortForward{
				{Portless: true, RemotePort: 8080},
			}},
			want: false,
		},
		{
			name: "plain forward does not use PF",
			cfg: config.TunnelConfig{PortForwards: []config.PortForward{
				{Portless: false, LocalPort: 80, RemotePort: 8080, ExposePort: 80},
			}},
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tunnelRequiresPrivilegedPortRedirect(tc.cfg); got != tc.want {
				t.Fatalf("tunnelRequiresPrivilegedPortRedirect() = %v, want %v", got, tc.want)
			}
		})
	}
}
