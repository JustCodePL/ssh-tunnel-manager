// Package e2efixture is a fake SSH server for the end-to-end tests. It answers
// the app's monitoring commands with canned output, serves SFTP from a real
// directory, offers a small echo shell and accepts forwards, so the real backend
// runs without a real server behind it.
package e2efixture

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/binary"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// Server is a running fixture.
type Server struct {
	config   *ssh.ServerConfig
	listener net.Listener
	Port     int
	Root     string

	mu    sync.Mutex
	conns map[*ssh.ServerConn]struct{}
}

// New starts a server that accepts the given public key.
func New(root string, authorized ssh.PublicKey) (*Server, error) {
	_, hostKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generating host key: %w", err)
	}
	signer, err := ssh.NewSignerFromKey(hostKey)
	if err != nil {
		return nil, fmt.Errorf("creating host signer: %w", err)
	}
	config := &ssh.ServerConfig{
		PublicKeyCallback: func(meta ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			if meta.User() != PasswordUser && bytes.Equal(key.Marshal(), authorized.Marshal()) {
				return nil, nil
			}
			return nil, fmt.Errorf("unknown key")
		},
		PasswordCallback: func(meta ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
			if meta.User() == PasswordUser && string(password) == Password {
				return nil, nil
			}
			return nil, fmt.Errorf("wrong password")
		},
	}
	config.AddHostKey(signer)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("listening: %w", err)
	}
	s := &Server{
		config:   config,
		listener: listener,
		Port:     listener.Addr().(*net.TCPAddr).Port,
		Root:     root,
		conns:    make(map[*ssh.ServerConn]struct{}),
	}
	go s.acceptLoop()
	return s, nil
}

// DropConnections closes every SSH connection, like a network failure.
func (s *Server) DropConnections() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for conn := range s.conns {
		conn.Close()
		n++
	}
	return n
}

func (s *Server) acceptLoop() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		go s.serve(conn)
	}
}

func (s *Server) serve(raw net.Conn) {
	conn, chans, reqs, err := ssh.NewServerConn(raw, s.config)
	if err != nil {
		raw.Close()
		return
	}
	s.mu.Lock()
	s.conns[conn] = struct{}{}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.conns, conn)
		s.mu.Unlock()
		conn.Close()
	}()
	go ssh.DiscardRequests(reqs)
	for newChannel := range chans {
		switch newChannel.ChannelType() {
		case "session":
			go s.session(newChannel)
		case "direct-tcpip":
			go s.directTCPIP(newChannel)
		default:
			_ = newChannel.Reject(ssh.UnknownChannelType, "unsupported")
		}
	}
}

func (s *Server) session(newChannel ssh.NewChannel) {
	channel, requests, err := newChannel.Accept()
	if err != nil {
		return
	}
	defer channel.Close()
	for req := range requests {
		switch req.Type {
		case "pty-req", "env", "window-change":
			if req.WantReply {
				_ = req.Reply(true, nil)
			}
		case "shell":
			_ = req.Reply(true, nil)
			runShell(channel)
			return
		case "exec":
			var payload struct{ Command string }
			_ = ssh.Unmarshal(req.Payload, &payload)
			_ = req.Reply(true, nil)
			output, status := respond(payload.Command)
			_, _ = io.WriteString(channel, output)
			_, _ = channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{status}))
			return
		case "subsystem":
			var payload struct{ Name string }
			_ = ssh.Unmarshal(req.Payload, &payload)
			if payload.Name != "sftp" {
				_ = req.Reply(false, nil)
				continue
			}
			_ = req.Reply(true, nil)
			server, err := sftp.NewServer(channel, sftp.WithServerWorkingDirectory(s.Root))
			if err == nil {
				_ = server.Serve()
				server.Close()
			}
			_, _ = channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{0}))
			return
		default:
			if req.WantReply {
				_ = req.Reply(false, nil)
			}
		}
	}
}

// runShell is a line-based shell that echoes its input.
func runShell(channel ssh.Channel) {
	_, _ = io.WriteString(channel, "fixture$ ")
	var line []byte
	buf := make([]byte, 1024)
	for {
		n, err := channel.Read(buf)
		if err != nil {
			return
		}
		for _, b := range buf[:n] {
			switch b {
			case '\r', '\n':
				command := strings.TrimSpace(string(line))
				line = line[:0]
				_, _ = io.WriteString(channel, "\r\n")
				if command == "exit" {
					_, _ = channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{0}))
					return
				}
				if command != "" {
					_, _ = io.WriteString(channel, "ran: "+command+"\r\n")
				}
				_, _ = io.WriteString(channel, "fixture$ ")
			default:
				line = append(line, b)
				_, _ = channel.Write([]byte{b})
			}
		}
	}
}

func (s *Server) directTCPIP(newChannel ssh.NewChannel) {
	var target struct {
		Host       string
		Port       uint32
		OriginHost string
		OriginPort uint32
	}
	if err := ssh.Unmarshal(newChannel.ExtraData(), &target); err != nil {
		_ = newChannel.Reject(ssh.ConnectionFailed, "bad target")
		return
	}
	channel, requests, err := newChannel.Accept()
	if err != nil {
		return
	}
	defer channel.Close()
	go ssh.DiscardRequests(requests)

	if target.Port == 3389 {
		answerX224(channel)
		return
	}
	// Anything else behaves as an echo service.
	_, _ = io.Copy(channel, channel)
}

// answerX224 replies to the app's RDP probe with a connection confirm, then ends
// the connection: the fixture detects as an RDP server without being one.
func answerX224(channel ssh.Channel) {
	header := make([]byte, 4)
	if _, err := io.ReadFull(channel, header); err != nil {
		return
	}
	length := int(binary.BigEndian.Uint16(header[2:4]))
	if length > 4 {
		if _, err := io.ReadFull(channel, make([]byte, length-4)); err != nil {
			return
		}
	}
	_, _ = channel.Write([]byte{0x03, 0x00, 0x00, 0x0b, 0x06, 0xd0, 0x00, 0x00, 0x12, 0x34, 0x00})
}

const procStat = `cpu  100 0 100 800 0 0 0 0 0 0
cpu0 50 0 50 400 0 0 0 0 0 0
cpu1 50 0 50 400 0 0 0 0 0 0
`

const procMeminfo = `MemTotal:        8000000 kB
MemFree:         3000000 kB
MemAvailable:    5000000 kB
SwapTotal:       2000000 kB
SwapFree:        2000000 kB
`

const psOutput = `    PID USER     %CPU %MEM COMMAND
   1842 postgres  8.4  2.9 postgres
   2310 www-data  4.1  0.3 nginx
   3055 deploy    2.3  5.2 node
    912 redis     0.7  0.1 redis-server
`

const dfOutput = `Filesystem     1-blocks      Used Available Capacity Mounted on
/dev/sda1   100000000000 40000000000 60000000000      40% /
/dev/sdb1   500000000000 450000000000 50000000000      90% /data
`

const dockerPS = `{"ID":"aaa111","Names":"db","Image":"postgres:16","Status":"Up 9 days (healthy)","State":"running","Ports":"5432/tcp"}
{"ID":"bbb222","Names":"web","Image":"nginx:1.27","Status":"Up 9 days","State":"running","Ports":"0.0.0.0:8080->80/tcp"}
{"ID":"ccc333","Names":"worker","Image":"app:1","Status":"Exited (0) 2 days ago","State":"exited","Ports":""}
`

const dockerStats = `{"ID":"aaa111","Name":"db","CPUPerc":"1.80%","MemPerc":"9.40%","MemUsage":"724MiB / 7.7GiB","NetIO":"1.2GB / 980MB","BlockIO":"310MB / 2.1GB","PIDs":"14"}
{"ID":"bbb222","Name":"web","CPUPerc":"0.10%","MemPerc":"0.60%","MemUsage":"46MiB / 7.7GiB","NetIO":"2.4GB / 8.1GB","BlockIO":"12MB / 0B","PIDs":"5"}
`

const dockerInspect = `{"Id":"aaa111","Name":"/db","Created":"2026-10-25T10:00:00Z","Path":"postgres","Args":[],"RestartCount":0,"State":{"Status":"running"},"Config":{"Image":"postgres:16","Env":["POSTGRES_PASSWORD=do-not-show","PGDATA=/data"]},"NetworkSettings":{"Networks":{"backend":{}},"Ports":{"5432/tcp":null}},"Mounts":[{"Source":"/srv/db","Destination":"/data","Mode":"rw"}]}
`

// respond returns the canned output for a command the app runs.
func respond(command string) (string, uint32) {
	switch {
	case strings.Contains(command, "command -v docker"):
		return "docker\nhtop\nLinux\n", 0
	case strings.Contains(command, "command -v"):
		return "yes\n", 0
	case strings.Contains(command, "ps -eo"):
		return procStat + "@@@\n" + procStat + "@@@\n" + procMeminfo + "@@@\n0.42 0.38 0.31 1/100 1234\n@@@\n1234567.00 100.00\n@@@\n" + psOutput, 0
	case strings.Contains(command, "/proc/stat"):
		return procStat + "---\n" + procMeminfo, 0
	case strings.HasPrefix(command, "df "):
		return dfOutput, 0
	case strings.HasPrefix(command, "docker ps"):
		return dockerPS, 0
	case strings.HasPrefix(command, "docker stats"):
		return dockerStats, 0
	case strings.HasPrefix(command, "docker inspect"):
		return dockerInspect, 0
	}
	return "", 127
}

// Setup describes a prepared test home directory.
type Setup struct {
	Home     string
	KeyPath  string
	FilesDir string
	Signer   ssh.Signer
}

// PrepareHome creates a home directory with an SSH key and sample remote files.
func PrepareHome(home string) (*Setup, error) {
	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		return nil, fmt.Errorf("creating .ssh: %w", err)
	}
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generating key: %w", err)
	}
	block, err := ssh.MarshalPrivateKey(private, "")
	if err != nil {
		return nil, fmt.Errorf("encoding key: %w", err)
	}
	keyPath := filepath.Join(sshDir, "id_fixture")
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(block), 0o600); err != nil {
		return nil, fmt.Errorf("writing key: %w", err)
	}
	signer, err := ssh.NewSignerFromKey(private)
	if err != nil {
		return nil, fmt.Errorf("creating signer: %w", err)
	}
	files := filepath.Join(home, "remote")
	for path, content := range map[string]string{
		"app/main.go":        "package main\n",
		"logs/app.log":       "started\n",
		"README.md":          "# Demo\n",
		"deploy.sh":          "#!/bin/sh\necho deploy\n",
		"docker-compose.yml": "services: {}\n",
	} {
		full := filepath.Join(files, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return nil, fmt.Errorf("creating %s: %w", path, err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			return nil, fmt.Errorf("writing %s: %w", path, err)
		}
	}
	return &Setup{Home: home, KeyPath: keyPath, FilesDir: files, Signer: signer}, nil
}

// PasswordUser can only log in to the fake server with Password, which proves a
// connection used a saved password and not the scenario's key.
const (
	PasswordUser = "password-user"
	Password     = "fixture-password"
)

// SSHConfig returns the SSH config of the test scenario for a server port. The
// "denied" tunnel's user gets through to a port nothing listens on.
func SSHConfig(port, closedPort int, keyPath string) string {
	host := func(meta []string, name string, serverPort int, forwards ...string) string {
		var b strings.Builder
		for _, line := range meta {
			b.WriteString("# stm:" + line + "\n")
		}
		fmt.Fprintf(&b, "Host %s\n    HostName 127.0.0.1\n    Port %d\n    User demo\n    IdentityFile %s\n", name, serverPort, keyPath)
		for _, forward := range forwards {
			b.WriteString(forward)
		}
		b.WriteString("\n")
		return b.String()
	}
	return host([]string{"group=Production"}, "prod-web", port,
		"    LocalForward 15432 127.0.0.1:5432\n",
		"    # stm:forward-protocol=rdp\n    LocalForward 13389 10.0.0.1:3389\n",
		"    # stm:forward-protocol=rdp\n    LocalForward 13390 10.0.0.2:3389\n",
		"    # stm:forward-protocol=ssh\n    LocalForward 12222 10.0.0.3:22\n",
	) + host([]string{"group=Production"}, "prod-db", port) +
		host([]string{"group=Staging"}, "stage-a", port) +
		host(nil, "broken-host", closedPort) +
		host(nil, "laptop", port)
}

// ControlHandler serves the test controls.
func ControlHandler(s *Server) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/drop", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, "%d", s.DropConnections())
	})
	mux.HandleFunc("/report", func(w http.ResponseWriter, r *http.Request) {
		fmt.Println("report:", r.URL.Query().Get("m"))
		w.Header().Set("Access-Control-Allow-Origin", "*")
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "ok")
	})
	return mux
}
