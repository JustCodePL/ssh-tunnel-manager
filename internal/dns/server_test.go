package dns

import (
	"net"
	"testing"
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
