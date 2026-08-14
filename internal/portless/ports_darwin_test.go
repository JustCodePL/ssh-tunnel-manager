//go:build darwin

package portless

import "testing"

func TestListenerPort(t *testing.T) {
	tests := []struct {
		name           string
		publicPort     int
		wantListener   int
		wantRedirected bool
	}{
		{name: "invalid zero", publicPort: 0, wantListener: 0, wantRedirected: false},
		{name: "HTTP", publicPort: 80, wantListener: 10080, wantRedirected: true},
		{name: "HTTPS", publicPort: 443, wantListener: 10443, wantRedirected: true},
		{name: "last privileged", publicPort: 1023, wantListener: 11023, wantRedirected: true},
		{name: "first unprivileged", publicPort: 1024, wantListener: 1024, wantRedirected: false},
		{name: "high port", publicPort: 8080, wantListener: 8080, wantRedirected: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ListenerPort(tc.publicPort); got != tc.wantListener {
				t.Fatalf("ListenerPort(%d) = %d, want %d", tc.publicPort, got, tc.wantListener)
			}
			if got := RequiresPrivilegedPortRedirect(tc.publicPort); got != tc.wantRedirected {
				t.Fatalf("RequiresPrivilegedPortRedirect(%d) = %v, want %v", tc.publicPort, got, tc.wantRedirected)
			}
		})
	}
}
