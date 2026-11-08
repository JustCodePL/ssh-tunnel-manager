package ssh

import (
	"errors"
	"io/fs"
	"log/slog"
	"net"
	"strings"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// HostKeyFunc decides whether to trust the key a server presents, before any
// authentication. Every SSH connection needs one: address is the host and port
// being dialled, which for a jump host or a connection inside a tunnel is that
// hop's own address.
type HostKeyFunc func(tunnelID, address string, key ssh.PublicKey) error

// KnownHostsVerdict is what a known_hosts file says about a server's key.
type KnownHostsVerdict int

const (
	// KnownHostsUnknown means the file has no key of this type for the server.
	KnownHostsUnknown KnownHostsVerdict = iota
	// KnownHostsMatch means the file lists this key for the server.
	KnownHostsMatch
	// KnownHostsMismatch means the file lists another key of this type.
	KnownHostsMismatch
	// KnownHostsRevoked means the file marks this key as revoked.
	KnownHostsRevoked
)

// CheckKnownHosts looks a server's key up in a known_hosts file. A file that is
// missing or cannot be parsed says nothing, so the caller treats the server as
// unknown and asks the user. A known key of another type also counts as unknown,
// as in OpenSSH: the server may simply have offered a different type this time.
func CheckKnownHosts(path, address string, key ssh.PublicKey) KnownHostsVerdict {
	callback, err := knownhosts.New(path)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			slog.Warn("cannot read known_hosts", "path", path, "error", err)
		}
		return KnownHostsUnknown
	}
	// The address decides the lookup. The remote address must only be well
	// formed, and connections through a proxy or a tunnel do not have a real one.
	err = callback(address, &net.TCPAddr{IP: net.IPv4zero}, key)
	if err == nil {
		return KnownHostsMatch
	}
	var revoked *knownhosts.RevokedError
	if errors.As(err, &revoked) {
		return KnownHostsRevoked
	}
	var keyErr *knownhosts.KeyError
	if !errors.As(err, &keyErr) {
		slog.Warn("cannot check known_hosts", "path", path, "address", address, "error", err)
		return KnownHostsUnknown
	}
	for _, want := range keyErr.Want {
		if want.Key.Type() == key.Type() {
			return KnownHostsMismatch
		}
	}
	return KnownHostsUnknown
}

// isHostKeyRefusal returns true if the host key check stopped the connection.
// Retrying cannot help: the server has to change, or the user has to decide.
func isHostKeyRefusal(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "host key changed") || strings.Contains(msg, "was not trusted")
}

// hostKeyCallback adapts a HostKeyFunc to one tunnel's connections.
func hostKeyCallback(tunnelID string, check HostKeyFunc) ssh.HostKeyCallback {
	return func(address string, _ net.Addr, key ssh.PublicKey) error {
		return check(tunnelID, address, key)
	}
}
