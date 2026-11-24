package main

import "testing"

func TestSingleInstanceIDSeparatesTheSmokeTest(t *testing.T) {
	// Stable and beta builds share this lock; changing it would let both run.
	if got := singleInstanceID(false); got != "com.wails.ssh-tunnel-manager" {
		t.Fatalf("app lock = %q", got)
	}
	// build.yml looks for this name to know the smoke test has its own lock.
	if got := singleInstanceID(true); got != "com.wails.ssh-tunnel-manager.smoke-test" {
		t.Fatalf("smoke test lock = %q", got)
	}
}

func TestSecondInstanceExitCodeFailsOnlyTheSmokeTest(t *testing.T) {
	if got := secondInstanceExitCode(false); got != 0 {
		t.Fatalf("second app launch exits %d", got)
	}
	if got := secondInstanceExitCode(true); got == 0 {
		t.Fatal("a smoke test that hands over to another instance exits 0")
	}
}
