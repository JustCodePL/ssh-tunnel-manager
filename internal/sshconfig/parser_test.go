package sshconfig

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func parseString(s string) ([]HostEntry, error) {
	return Parse(bufio.NewScanner(strings.NewReader(s)))
}

func TestParseBasicHost(t *testing.T) {
	input := `Host my-server
    HostName bastion.example.com
    User deploy
    Port 2222
    IdentityFile /home/user/.ssh/id_ed25519
    LocalForward 8080 127.0.0.1:80
`
	entries, err := parseString(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	e := entries[0]
	if e.Alias != "my-server" {
		t.Errorf("Alias = %q, want %q", e.Alias, "my-server")
	}
	if e.HostName != "bastion.example.com" {
		t.Errorf("HostName = %q, want %q", e.HostName, "bastion.example.com")
	}
	if e.User != "deploy" {
		t.Errorf("User = %q, want %q", e.User, "deploy")
	}
	if e.Port != 2222 {
		t.Errorf("Port = %d, want %d", e.Port, 2222)
	}
	if e.IdentityFile != "/home/user/.ssh/id_ed25519" {
		t.Errorf("IdentityFile = %q, want absolute path", e.IdentityFile)
	}
	if len(e.PortForwards) != 1 {
		t.Fatalf("expected 1 port forward, got %d", len(e.PortForwards))
	}
	pf := e.PortForwards[0]
	if pf.LocalPort != 8080 || pf.RemoteHost != "127.0.0.1" || pf.RemotePort != 80 {
		t.Errorf("PortForward = %+v, want {8080, 127.0.0.1, 80}", pf)
	}
}

func TestParseMultipleHosts(t *testing.T) {
	input := `Host alpha
    HostName alpha.example.com
    User root

Host beta
    HostName beta.example.com
    User admin
    LocalForward 3306 db:3306
`
	entries, err := parseString(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Alias != "alpha" {
		t.Errorf("first alias = %q, want %q", entries[0].Alias, "alpha")
	}
	if entries[1].Alias != "beta" {
		t.Errorf("second alias = %q, want %q", entries[1].Alias, "beta")
	}
	if len(entries[1].PortForwards) != 1 {
		t.Fatalf("expected 1 port forward for beta, got %d", len(entries[1].PortForwards))
	}
	pf := entries[1].PortForwards[0]
	if pf.RemoteHost != "db" {
		t.Errorf("remote host = %q, want %q", pf.RemoteHost, "db")
	}
}

func TestParseWildcardSkipped(t *testing.T) {
	input := `Host *
    ServerAliveInterval 60

Host *.example.com
    User shared

Host real-server
    HostName real.example.com
    User admin
`
	entries, err := parseString(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry (wildcards skipped), got %d", len(entries))
	}
	if entries[0].Alias != "real-server" {
		t.Errorf("Alias = %q, want %q", entries[0].Alias, "real-server")
	}
}

func TestParseMatchBlockSkipped(t *testing.T) {
	input := `Host before-match
    HostName before.example.com

Match host *.internal
    ProxyJump gateway

Host after-match
    HostName after.example.com
`
	entries, err := parseString(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries (Match skipped), got %d", len(entries))
	}
	if entries[0].Alias != "before-match" {
		t.Errorf("first = %q, want %q", entries[0].Alias, "before-match")
	}
	if entries[1].Alias != "after-match" {
		t.Errorf("second = %q, want %q", entries[1].Alias, "after-match")
	}
}

func TestParseMultipleLocalForwards(t *testing.T) {
	input := `Host multi
    HostName multi.example.com
    LocalForward 8080 127.0.0.1:80
    LocalForward 9090 db:3306
`
	entries, err := parseString(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries[0].PortForwards) != 2 {
		t.Fatalf("expected 2 port forwards, got %d", len(entries[0].PortForwards))
	}
	if entries[0].PortForwards[1].LocalPort != 9090 {
		t.Errorf("second forward local port = %d, want 9090", entries[0].PortForwards[1].LocalPort)
	}
}

func TestParseMetadata(t *testing.T) {
	input := `# stm:group=production
# stm:autoconnect=true
Host prod-db
    HostName prod.example.com
    User deploy
`
	entries, err := parseString(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Group != "production" {
		t.Errorf("Group = %q, want %q", entries[0].Group, "production")
	}
	if !entries[0].AutoConnect {
		t.Error("AutoConnect = false, want true")
	}
}

func TestParseMetadataNotCarriedToNextHost(t *testing.T) {
	input := `# stm:group=groupA
Host first
    HostName first.example.com

Host second
    HostName second.example.com
`
	entries, err := parseString(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2, got %d", len(entries))
	}
	if entries[0].Group != "groupA" {
		t.Errorf("first group = %q, want %q", entries[0].Group, "groupA")
	}
	if entries[1].Group != "" {
		t.Errorf("second group = %q, want empty", entries[1].Group)
	}
}

func TestParseEmptyFile(t *testing.T) {
	entries, err := parseString("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected 0 entries, got %d", len(entries))
	}
}

func TestParseDefaultPort(t *testing.T) {
	input := `Host simple
    HostName simple.example.com
`
	entries, err := parseString(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if entries[0].Port != 22 {
		t.Errorf("Port = %d, want 22 (default)", entries[0].Port)
	}
}

func TestParseHostNameAbsent(t *testing.T) {
	input := `Host my-alias
    User admin
`
	entries, err := parseString(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if entries[0].HostName != "my-alias" {
		t.Errorf("HostName = %q, want %q (should default to alias)", entries[0].HostName, "my-alias")
	}
}

func TestRenderHostBlock(t *testing.T) {
	e := HostEntry{
		Alias:        "my-server",
		HostName:     "bastion.example.com",
		User:         "deploy",
		Port:         2222,
		IdentityFile: "/home/user/.ssh/id_ed25519",
		Group:        "production",
		AutoConnect:  true,
		PortForwards: []PortForwardEntry{
			{LocalPort: 8080, RemoteHost: "127.0.0.1", RemotePort: 80},
		},
	}
	got := RenderHostBlock(e)

	checks := []string{
		"# stm:group=production",
		"# stm:autoconnect=true",
		"Host my-server",
		"    HostName bastion.example.com",
		"    User deploy",
		"    Port 2222",
		"    LocalForward 8080 127.0.0.1:80",
	}
	for _, want := range checks {
		if !strings.Contains(got, want) {
			t.Errorf("rendered block missing %q:\n%s", want, got)
		}
	}
}

func TestRenderHostBlockMinimal(t *testing.T) {
	e := HostEntry{
		Alias:    "simple",
		HostName: "simple",
		Port:     22,
	}
	got := RenderHostBlock(e)
	if !strings.Contains(got, "Host simple") {
		t.Errorf("missing Host line:\n%s", got)
	}
	if strings.Contains(got, "HostName") {
		t.Errorf("should not include HostName when HostName == Alias:\n%s", got)
	}
	if strings.Contains(got, "Port") {
		t.Errorf("should not include Port when 22:\n%s", got)
	}
	if strings.Contains(got, "# stm:") {
		t.Errorf("should not include stm metadata:\n%s", got)
	}
}

func TestParsePortForwardDescription(t *testing.T) {
	input := `Host described
    HostName described.example.com
    LocalForward 8080 127.0.0.1:80 # web server
    LocalForward 3306 db:3306 # database
    LocalForward 9090 127.0.0.1:9090
`
	entries, err := parseString(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	pfs := entries[0].PortForwards
	if len(pfs) != 3 {
		t.Fatalf("expected 3 port forwards, got %d", len(pfs))
	}
	if pfs[0].Description != "web server" {
		t.Errorf("pf[0].Description = %q, want %q", pfs[0].Description, "web server")
	}
	if pfs[1].Description != "database" {
		t.Errorf("pf[1].Description = %q, want %q", pfs[1].Description, "database")
	}
	if pfs[2].Description != "" {
		t.Errorf("pf[2].Description = %q, want empty", pfs[2].Description)
	}
}

func TestRenderPortForwardDescription(t *testing.T) {
	e := HostEntry{
		Alias:    "desc-test",
		HostName: "desc.example.com",
		Port:     22,
		PortForwards: []PortForwardEntry{
			{LocalPort: 8080, RemoteHost: "127.0.0.1", RemotePort: 80, Description: "web server"},
			{LocalPort: 3306, RemoteHost: "db", RemotePort: 3306},
		},
	}
	got := RenderHostBlock(e)
	if !strings.Contains(got, "LocalForward 8080 127.0.0.1:80 # web server") {
		t.Errorf("expected description comment in rendered block:\n%s", got)
	}
	if strings.Contains(got, "3306:3306 #") {
		t.Errorf("should not have comment when description is empty:\n%s", got)
	}
}

func TestParseProxyCommand(t *testing.T) {
	input := `Host ec2-instance
    HostName i-0b305fe8db0d23da6
    User ec2-user
    ProxyCommand aws ssm start-session --target %h --document-name AWS-StartSSHSession --parameters portNumber=%p
`
	entries, err := parseString(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	want := "aws ssm start-session --target %h --document-name AWS-StartSSHSession --parameters portNumber=%p"
	if entries[0].ProxyCommand != want {
		t.Errorf("ProxyCommand = %q, want %q", entries[0].ProxyCommand, want)
	}
}

func TestParseProxyJump(t *testing.T) {
	input := `Host internal
    HostName 10.0.1.50
    User admin
    ProxyJump bastion.example.com
`
	entries, err := parseString(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].ProxyJump != "bastion.example.com" {
		t.Errorf("ProxyJump = %q, want %q", entries[0].ProxyJump, "bastion.example.com")
	}
}

func TestRenderProxyJump(t *testing.T) {
	e := HostEntry{
		Alias:     "internal",
		HostName:  "10.0.1.50",
		Port:      22,
		ProxyJump: "bastion.example.com",
	}
	got := RenderHostBlock(e)
	if !strings.Contains(got, "    ProxyJump bastion.example.com") {
		t.Errorf("expected ProxyJump in rendered block:\n%s", got)
	}
}

func TestRenderNoProxy(t *testing.T) {
	e := HostEntry{
		Alias:    "simple",
		HostName: "simple.example.com",
		Port:     22,
	}
	got := RenderHostBlock(e)
	if strings.Contains(got, "ProxyCommand") {
		t.Errorf("should not include ProxyCommand when empty:\n%s", got)
	}
	if strings.Contains(got, "ProxyJump") {
		t.Errorf("should not include ProxyJump when empty:\n%s", got)
	}
}

func TestRoundTripProxyCommand(t *testing.T) {
	original := HostEntry{
		Alias:        "proxy-test",
		HostName:     "i-abc123",
		User:         "ec2-user",
		Port:         22,
		ProxyCommand: "aws ssm start-session --target %h --document-name AWS-StartSSHSession",
	}
	rendered := RenderHostBlock(original)
	parsed, err := parseString(rendered)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(parsed) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(parsed))
	}
	if parsed[0].ProxyCommand != original.ProxyCommand {
		t.Errorf("ProxyCommand = %q, want %q", parsed[0].ProxyCommand, original.ProxyCommand)
	}
}

func TestRoundTripParseRender(t *testing.T) {
	original := HostEntry{
		Alias:       "webserver",
		HostName:    "web.example.com",
		User:        "www",
		Port:        22,
		Group:       "web",
		AutoConnect: true,
		PortForwards: []PortForwardEntry{
			{LocalPort: 8080, RemoteHost: "127.0.0.1", RemotePort: 80, Description: "web"},
			{LocalPort: 8443, RemoteHost: "127.0.0.1", RemotePort: 443},
		},
	}

	rendered := RenderHostBlock(original)
	parsed, err := parseString(rendered)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(parsed) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(parsed))
	}
	got := parsed[0]

	if got.Alias != original.Alias {
		t.Errorf("Alias = %q, want %q", got.Alias, original.Alias)
	}
	if got.HostName != original.HostName {
		t.Errorf("HostName = %q, want %q", got.HostName, original.HostName)
	}
	if got.User != original.User {
		t.Errorf("User = %q, want %q", got.User, original.User)
	}
	if got.Group != original.Group {
		t.Errorf("Group = %q, want %q", got.Group, original.Group)
	}
	if got.AutoConnect != original.AutoConnect {
		t.Errorf("AutoConnect = %v, want %v", got.AutoConnect, original.AutoConnect)
	}
	if len(got.PortForwards) != len(original.PortForwards) {
		t.Fatalf("PortForwards count = %d, want %d", len(got.PortForwards), len(original.PortForwards))
	}
	if got.PortForwards[0].Description != "web" {
		t.Errorf("PortForwards[0].Description = %q, want %q", got.PortForwards[0].Description, "web")
	}
	if got.PortForwards[1].Description != "" {
		t.Errorf("PortForwards[1].Description = %q, want empty", got.PortForwards[1].Description)
	}
}

func TestParseFileWithInclude(t *testing.T) {
	dir := t.TempDir()
	confDir := filepath.Join(dir, "config.d")
	if err := os.MkdirAll(confDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Write included file
	included := filepath.Join(confDir, "work.conf")
	if err := os.WriteFile(included, []byte(`Host work-server
    HostName work.example.com
    User deploy
`), 0o644); err != nil {
		t.Fatalf("write included: %v", err)
	}

	// Write main config with Include directive
	mainConfig := filepath.Join(dir, "config")
	if err := os.WriteFile(mainConfig, []byte(`Host main-server
    HostName main.example.com
    User admin

Include `+confDir+`/*.conf
`), 0o644); err != nil {
		t.Fatalf("write main: %v", err)
	}

	entries, err := ParseFile(mainConfig)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	// First entry from main config
	if entries[0].Alias != "main-server" {
		t.Errorf("entries[0].Alias = %q, want %q", entries[0].Alias, "main-server")
	}
	absMain, _ := filepath.Abs(mainConfig)
	if entries[0].SourceFile != absMain {
		t.Errorf("entries[0].SourceFile = %q, want %q", entries[0].SourceFile, absMain)
	}

	// Second entry from included file
	if entries[1].Alias != "work-server" {
		t.Errorf("entries[1].Alias = %q, want %q", entries[1].Alias, "work-server")
	}
	absIncluded, _ := filepath.Abs(included)
	if entries[1].SourceFile != absIncluded {
		t.Errorf("entries[1].SourceFile = %q, want %q", entries[1].SourceFile, absIncluded)
	}
}

func TestParseFileSourceFileSet(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	if err := os.WriteFile(path, []byte(`Host simple
    HostName simple.example.com
`), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	entries, err := ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	absPath, _ := filepath.Abs(path)
	if entries[0].SourceFile != absPath {
		t.Errorf("SourceFile = %q, want %q", entries[0].SourceFile, absPath)
	}
}
