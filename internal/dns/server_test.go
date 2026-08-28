package dns

import (
	"fmt"
	"net"
	"testing"

	mdns "github.com/miekg/dns"
)

func TestServerStartReportsUDPAddressInUse(t *testing.T) {
	blocker, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Close()

	server := NewServer(NewRegistry())
	err = server.start(blocker.LocalAddr().String())
	if !IsAddressInUse(err) {
		t.Fatalf("start error = %v, want address-in-use classification", err)
	}
	if server.Running() {
		t.Fatal("server should not be running after UDP bind conflict")
	}
}

func TestServerStartReportsTCPAddressInUseAndReleasesUDP(t *testing.T) {
	blocker, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Close()
	addr := blocker.Addr().String()

	server := NewServer(NewRegistry())
	err = server.start(addr)
	if !IsAddressInUse(err) {
		t.Fatalf("start error = %v, want address-in-use classification", err)
	}
	if server.Running() {
		t.Fatal("server should not be running after TCP bind conflict")
	}

	udp, err := net.ListenPacket("udp", addr)
	if err != nil {
		t.Fatalf("UDP listener leaked after TCP bind conflict: %v", err)
	}
	_ = udp.Close()
}

func TestServerStartAndStopOwnBothTransports(t *testing.T) {
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := probe.Addr().String()
	_ = probe.Close()

	server := NewServer(NewRegistry())
	if err := server.start(addr); err != nil {
		t.Fatalf("start: %v", err)
	}
	if !server.Running() {
		t.Fatal("server should report running after both transports start")
	}

	server.Stop()
	if server.Running() {
		t.Fatal("server should report stopped")
	}

	tcp, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("TCP address still held after Stop: %v", err)
	}
	_ = tcp.Close()
	udp, err := net.ListenPacket("udp", addr)
	if err != nil {
		t.Fatalf("UDP address still held after Stop: %v", err)
	}
	_ = udp.Close()
}

func TestRemoteRegistriesShareAddressesForSameServicePort(t *testing.T) {
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := probe.Addr().String()
	_ = probe.Close()

	registry := NewRegistry()
	server := NewServer(registry)
	if err := server.start(addr); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer server.Stop()

	first, err := newRemoteRegistry(addr)
	if err != nil {
		t.Fatalf("connect first registry: %v", err)
	}
	defer first.Close()
	second, err := newRemoteRegistry(addr)
	if err != nil {
		t.Fatalf("connect second registry: %v", err)
	}
	defer second.Close()

	// Include the DNS-owning app's local registry as well as two other app
	// instances. The exposed service port remains identical; the loopback IP
	// is what makes every listener address unique.
	clients := []ForwardRegistry{registry, first, second, first}
	seen := make(map[string]bool)
	for i, client := range clients {
		domain := fmt.Sprintf("mysql-%d", i+1)
		entry, err := client.Allocate(domain, 3306)
		if err != nil {
			t.Fatalf("allocate %s: %v", domain, err)
		}
		if entry.Port != 3306 {
			t.Fatalf("%s port = %d, want 3306", domain, entry.Port)
		}
		if seen[entry.IP.String()] {
			t.Fatalf("duplicate loopback address allocated: %s", entry.IP)
		}
		seen[entry.IP.String()] = true

		msg := new(mdns.Msg)
		msg.SetQuestion(domain+"."+TLD+".", mdns.TypeA)
		reply, _, err := (&mdns.Client{Net: "tcp"}).Exchange(msg, addr)
		if err != nil {
			t.Fatalf("resolve %s: %v", domain, err)
		}
		if len(reply.Answer) != 1 {
			t.Fatalf("resolve %s answers = %d, want 1", domain, len(reply.Answer))
		}
		a, ok := reply.Answer[0].(*mdns.A)
		if !ok || !a.A.Equal(entry.IP) {
			t.Fatalf("resolve %s = %v, want %s", domain, reply.Answer[0], entry.IP)
		}
	}
	if len(seen) != 4 {
		t.Fatalf("unique addresses = %d, want 4", len(seen))
	}
}

func TestRemoteRegistryRejectsDomainOwnedByAnotherInstance(t *testing.T) {
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := probe.Addr().String()
	_ = probe.Close()

	server := NewServer(NewRegistry())
	if err := server.start(addr); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer server.Stop()
	first, err := newRemoteRegistry(addr)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := newRemoteRegistry(addr)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()

	if _, err := first.Allocate("mysql", 3306); err != nil {
		t.Fatalf("first allocate: %v", err)
	}
	if _, err := second.Allocate("mysql", 3306); err == nil {
		t.Fatal("second owner should not be able to hijack the same domain")
	}
	if err := first.Close(); err != nil {
		t.Fatalf("close first owner: %v", err)
	}
	if _, err := second.Allocate("mysql", 3306); err != nil {
		t.Fatalf("domain should be reusable after owner closes: %v", err)
	}
}
