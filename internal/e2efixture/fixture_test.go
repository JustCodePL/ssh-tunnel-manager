package e2efixture

import (
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"

	"ssh-tunnel-manager/internal/sysstats"
)

func TestCannedOutputParses(t *testing.T) {
	output, status := respond("docker ps -a --format '{{json .}}'")
	if status != 0 {
		t.Fatalf("status = %d", status)
	}
	containers := sysstats.ParseDockerPs(output)
	if len(containers) != 3 || containers[0].Name != "db" {
		t.Fatalf("containers = %+v", containers)
	}
	df, _ := respond("df -P -B1")
	if mounts := sysstats.ParseDf(df); len(mounts) != 2 || mounts[1].MountPoint != "/data" {
		t.Fatalf("mounts = %+v", mounts)
	}
	if _, status := respond("rm -rf /"); status == 0 {
		t.Fatal("an unknown command must fail")
	}
}

func TestServerRunsCommandsAndDrops(t *testing.T) {
	setup, err := PrepareHome(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	server, err := New(setup.FilesDir, setup.Signer.PublicKey())
	if err != nil {
		t.Fatal(err)
	}
	client, err := ssh.Dial("tcp", strings.Replace(server.listener.Addr().String(), "localhost", "127.0.0.1", 1), &ssh.ClientConfig{
		User:            "demo",
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(setup.Signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	})
	if err != nil {
		t.Fatal(err)
	}
	session, err := client.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	out, err := session.Output("command -v docker >/dev/null")
	if err != nil || !strings.Contains(string(out), "docker") {
		t.Fatalf("out=%q err=%v", out, err)
	}
	if server.DropConnections() != 1 {
		t.Fatal("expected one connection to drop")
	}
	if err := client.Wait(); err == nil {
		t.Fatal("the client should see the connection end")
	}
}
