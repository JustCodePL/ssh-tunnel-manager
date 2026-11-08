package ssh

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/pkg/sftp"
)

func removeSession(t *testing.T) *SFTPSession {
	t.Helper()
	tun, _ := keyTunnel(t, trustAnyHostKey)
	clientConfig, err := tun.buildSSHConfig()
	if err != nil {
		t.Fatal(err)
	}
	client, err := tun.dial(context.Background(), clientConfig)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close() })
	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sftpClient.Close() })
	return &SFTPSession{client: client, sftpc: sftpClient}
}

func TestRemoveDeletesALinkAndKeepsItsTarget(t *testing.T) {
	session := removeSession(t)
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	kept := filepath.Join(target, "kept.txt")
	if err := os.WriteFile(kept, []byte("kept"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("cannot create a symbolic link: %v", err)
	}

	if err := session.Remove(link); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Errorf("the link is still there: %v", err)
	}
	if _, err := os.Stat(kept); err != nil {
		t.Errorf("the target's contents are gone: %v", err)
	}
}

func TestRemoveDeletesADanglingLink(t *testing.T) {
	session := removeSession(t)
	link := filepath.Join(t.TempDir(), "dangling")
	if err := os.Symlink(filepath.Join(filepath.Dir(link), "missing"), link); err != nil {
		t.Skipf("cannot create a symbolic link: %v", err)
	}
	if err := session.Remove(link); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Errorf("the link is still there: %v", err)
	}
}

func TestRemoveStillDeletesADirectoryWithItsContents(t *testing.T) {
	session := removeSession(t)
	dir := filepath.Join(t.TempDir(), "dir")
	if err := os.MkdirAll(filepath.Join(dir, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "nested", "file.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := session.Remove(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(dir); !os.IsNotExist(err) {
		t.Errorf("the directory is still there: %v", err)
	}
}
