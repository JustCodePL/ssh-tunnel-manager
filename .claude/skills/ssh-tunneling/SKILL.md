---
name: ssh-tunneling
description: >
  SSH tunnel and port forwarding patterns in Go using golang.org/x/crypto/ssh.
  Use when implementing SSH connections, key authentication, local port
  forwarding, keep-alive, reconnection logic, or parsing SSH config files.
---

# SSH Tunneling in Go

## Connection Pattern
```go
config := &ssh.ClientConfig{
    User: user,
    Auth: []ssh.AuthMethod{ssh.PublicKeys(signer)},
    HostKeyCallback: ssh.InsecureIgnoreHostKey(), // TODO: known_hosts
    Timeout: 10 * time.Second,
}
client, err := ssh.Dial("tcp", fmt.Sprintf("%s:%d", host, port), config)
```

## Key Loading
```go
key, err := os.ReadFile(privateKeyPath)
// Without passphrase:
signer, err := ssh.ParsePrivateKey(key)
// With passphrase:
signer, err := ssh.ParsePrivateKeyWithPassphrase(key, []byte(passphrase))
```

Supported formats: RSA, Ed25519, ECDSA (OpenSSH and PEM).

## Local Port Forwarding
```go
listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", localPort))
// Accept loop in goroutine:
for {
    local, err := listener.Accept()
    remote, err := client.Dial("tcp", fmt.Sprintf("%s:%d", remoteHost, remotePort))
    go io.Copy(local, remote)
    go io.Copy(remote, local)
}
```

## Keep-Alive
```go
go func() {
    ticker := time.NewTicker(interval)
    for {
        select {
        case <-ticker.C:
            _, _, err := client.SendRequest("keepalive@openssh.com", true, nil)
            if err != nil { /* trigger reconnect */ }
        case <-ctx.Done():
            return
        }
    }
}()
```

## SSH Config Parsing

LocalForward format: `LocalForward local_port remote_host:remote_port`
Parse with: split by whitespace, handle comments starting with `#`.
Metadata in comments: `# Label: PostgreSQL`, `# Color: #FF6B6B`, `# Group: Production`

## Reconnect with Exponential Backoff

delays: 5s → 10s → 20s → 40s → max 5 retries (configurable)
Use `context.Context` to cancel reconnect on user disconnect.