package dns

import "testing"

func TestRegistry_AllocateReleaseLookup(t *testing.T) {
	r := NewRegistry()

	e1, err := r.Allocate("db.foo", 5432)
	if err != nil {
		t.Fatalf("first allocate: %v", err)
	}
	if e1.IP.String() != "127.0.1.1" {
		t.Errorf("first IP = %s, want 127.0.1.1", e1.IP)
	}

	e2, err := r.Allocate("db.bar", 5432)
	if err != nil {
		t.Fatalf("second allocate: %v", err)
	}
	if e2.IP.String() != "127.0.1.2" {
		t.Errorf("second IP = %s, want 127.0.1.2", e2.IP)
	}

	// Repeat allocate is idempotent
	again, err := r.Allocate("db.foo", 5432)
	if err != nil {
		t.Fatalf("idempotent allocate: %v", err)
	}
	if !again.IP.Equal(e1.IP) {
		t.Errorf("repeat allocate changed IP: %s -> %s", e1.IP, again.IP)
	}

	got, ok := r.Lookup("db.foo")
	if !ok || !got.IP.Equal(e1.IP) {
		t.Errorf("lookup miss or wrong IP: %v, %s", ok, got.IP)
	}

	// Release frees the IP for reuse
	r.Release("db.foo")
	if _, ok := r.Lookup("db.foo"); ok {
		t.Error("Lookup returned entry after Release")
	}

	e3, err := r.Allocate("db.baz", 5432)
	if err != nil {
		t.Fatalf("post-release allocate: %v", err)
	}
	if e3.IP.String() != "127.0.1.1" {
		t.Errorf("post-release IP = %s, want recycled 127.0.1.1", e3.IP)
	}
}

func TestRegistry_LookupFQDN(t *testing.T) {
	r := NewRegistry()
	if _, err := r.Allocate("db.foo", 5432); err != nil {
		t.Fatalf("allocate: %v", err)
	}

	cases := []struct {
		fqdn string
		want bool
	}{
		{"db.foo.ssh-local.", true},
		{"DB.FOO.SSH-LOCAL.", true},
		{"db.foo.ssh-local", true},
		{"db.foo.local.", false},
		{"unknown.ssh-local.", false},
	}
	for _, tc := range cases {
		_, ok := r.LookupFQDN(tc.fqdn)
		if ok != tc.want {
			t.Errorf("LookupFQDN(%q) = %v, want %v", tc.fqdn, ok, tc.want)
		}
	}
}

func TestRegistry_BlockSkipsBlockedIPs(t *testing.T) {
	r := NewRegistry()

	first, err := r.Allocate("a", 1)
	if err != nil {
		t.Fatalf("allocate a: %v", err)
	}
	if first.IP.String() != "127.0.1.1" {
		t.Errorf("first IP = %s, want 127.0.1.1", first.IP)
	}
	// Simulate a bind failure on this IP: release the domain and block
	// the address. Next allocation must skip 127.0.1.1.
	r.Release("a")
	r.Block(first.IP)

	second, err := r.Allocate("a", 1)
	if err != nil {
		t.Fatalf("re-allocate a: %v", err)
	}
	if second.IP.String() != "127.0.1.2" {
		t.Errorf("after Block, got %s, want 127.0.1.2", second.IP)
	}
}

func TestRegistry_Exhaustion(t *testing.T) {
	r := NewRegistry()
	for i := 0; i < 254; i++ {
		if _, err := r.Allocate(makeName(i), 1); err != nil {
			t.Fatalf("allocate %d: %v", i, err)
		}
	}
	if _, err := r.Allocate("overflow", 1); err == nil {
		t.Error("expected error on 255th allocate, got nil")
	}
}

func makeName(i int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz"
	a := letters[i/26%26]
	b := letters[i%26]
	return string([]byte{a, b})
}
