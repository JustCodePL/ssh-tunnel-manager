package main

import (
	"testing"

	"ssh-tunnel-manager/internal/config"
)

func TestWithRDPCertificateCopiesTheMap(t *testing.T) {
	original := map[string]string{"a": "1"}

	updated := withRDPCertificate(original, "b", "2")
	if len(original) != 1 {
		t.Fatalf("original was mutated: %v", original)
	}
	if updated["a"] != "1" || updated["b"] != "2" {
		t.Fatalf("updated = %v", updated)
	}

	removed := withRDPCertificate(updated, "a", "")
	if _, ok := removed["a"]; ok {
		t.Fatalf("entry was not removed: %v", removed)
	}
	if updated["a"] != "1" {
		t.Fatalf("updated was mutated: %v", updated)
	}

	if got := withRDPCertificate(nil, "a", "1"); got["a"] != "1" {
		t.Fatalf("from nil map: %v", got)
	}
}

func TestRDPRemoteAddress(t *testing.T) {
	if got := rdpRemoteAddress(config.TunnelConfig{}); got != "127.0.0.1:3389" {
		t.Fatalf("default = %q", got)
	}
	if got := rdpRemoteAddress(config.TunnelConfig{RDPPort: 3390}); got != "127.0.0.1:3390" {
		t.Fatalf("custom = %q", got)
	}
}

func TestValidateRDPPort(t *testing.T) {
	for _, port := range []int{0, 1, 3389, 65535} {
		if err := validateRDPPort(port); err != nil {
			t.Errorf("port %d: %v", port, err)
		}
	}
	for _, port := range []int{-1, 65536} {
		if err := validateRDPPort(port); err == nil {
			t.Errorf("port %d should be rejected", port)
		}
	}
}
