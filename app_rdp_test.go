package main

import "testing"

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
