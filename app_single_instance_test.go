package main

import "testing"

func TestSingleInstanceIDSeparatesTheSmokeTest(t *testing.T) {
	// Stable and beta builds share this lock; changing it would let both run.
	if got := singleInstanceID(false); got != "com.wails.ssh-tunnel-manager" {
		t.Fatalf("app lock = %q", got)
	}
	if got := singleInstanceID(true); got == singleInstanceID(false) {
		t.Fatalf("smoke test shares the app's lock %q", got)
	}
}
