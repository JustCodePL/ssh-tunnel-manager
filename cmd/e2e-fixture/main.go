// Command e2e-fixture prepares a test home and runs the fake SSH server used by
// the end-to-end tests. It prints the home path and blocks.
package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"

	"ssh-tunnel-manager/internal/e2efixture"
)

func main() {
	if len(os.Args) != 3 {
		log.Fatal("usage: e2e-fixture <home> <control-port>")
	}
	home := os.Args[1]
	setup, err := e2efixture.PrepareHome(home)
	if err != nil {
		log.Fatal(err)
	}
	server, err := e2efixture.New(setup.FilesDir, setup.Signer.PublicKey())
	if err != nil {
		log.Fatal(err)
	}
	closed, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	closedPort := closed.Addr().(*net.TCPAddr).Port
	closed.Close()

	config := e2efixture.SSHConfig(server.Port, closedPort, setup.KeyPath)
	if err := os.WriteFile(filepath.Join(home, ".ssh", "config"), []byte(config), 0o600); err != nil {
		log.Fatal(err)
	}
	// The app only talks to servers it can verify.
	if err := os.WriteFile(filepath.Join(home, ".ssh", "known_hosts"), []byte(server.KnownHostsLine()+"\n"), 0o600); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("fixture ready home=%s ssh=%d\n", home, server.Port)
	log.Fatal(http.ListenAndServe("127.0.0.1:"+os.Args[2], e2efixture.ControlHandler(server)))
}
