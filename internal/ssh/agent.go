package ssh

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"runtime"
	"sync/atomic"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

// AgentDialer opens a new connection to an SSH agent. Each request to the
// agent uses its own connection and closes it when the request ends.
type AgentDialer func() (net.Conn, error)

const (
	// agentListTimeout bounds dialing the agent, and then separately listing
	// its keys. An agent answers this at once; one that accepts connections
	// but never answers (for example the launchd ssh-agent once it runs out of
	// file descriptors) must not block the connection forever.
	agentListTimeout = 5 * time.Second
	// agentSignTimeout bounds a signature. It is longer because an agent may
	// ask the user to confirm the use of a key (ssh-add -c, 1Password).
	agentSignTimeout = 60 * time.Second
)

// dialSystemAgent connects to the user's SSH agent: SSH_AUTH_SOCK, or on macOS
// the launchd agent socket when the app was started without it, or on Windows
// the OpenSSH agent pipe.
func dialSystemAgent() (net.Conn, error) {
	if runtime.GOOS == "windows" {
		return net.DialTimeout("pipe", `\\.\pipe\openssh-ssh-agent`, agentListTimeout)
	}
	sock := os.Getenv("SSH_AUTH_SOCK")
	if sock == "" && runtime.GOOS == "darwin" {
		// macOS GUI apps may not inherit SSH_AUTH_SOCK from the shell.
		// Try the launchd-managed agent socket.
		sock = findMacOSAgentSocket()
	}
	if sock == "" {
		return nil, errors.New("SSH_AUTH_SOCK not set")
	}
	return net.DialTimeout("unix", sock, agentListTimeout)
}

// errAgentUnreachable marks a failure to connect to the agent, which is normal
// when no agent runs.
var errAgentUnreachable = errors.New("cannot connect to the SSH agent")

// agentKeys returns a signer for each key of the agent that dial reaches, or
// none when dial is nil. An agent that cannot be reached or does not answer in
// time is skipped, so the other login methods still run.
func agentKeys(dial AgentDialer) []ssh.Signer {
	if dial == nil {
		return nil
	}
	signers, err := agentSigners(dial, agentListTimeout, agentSignTimeout)
	switch {
	case errors.Is(err, errAgentUnreachable):
		slog.Debug("SSH agent not available", "error", err)
	case err != nil:
		slog.Warn("SSH agent did not list its keys", "error", err)
	}
	return signers
}

// agentSigners lists the agent's keys within listTimeout and returns a signer
// for each, whose signatures are bounded by signTimeout.
func agentSigners(dial AgentDialer, listTimeout, signTimeout time.Duration) ([]ssh.Signer, error) {
	var keys []*agent.Key
	err := withAgent(dial, listTimeout, func(client agent.ExtendedAgent) error {
		var err error
		keys, err = client.List()
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("listing the SSH agent's keys: %w", err)
	}
	signers := make([]ssh.Signer, 0, len(keys))
	for _, key := range keys {
		signers = append(signers, &agentSigner{dial: dial, key: key, timeout: signTimeout})
	}
	return signers, nil
}

// withAgent runs fn with a client on a new agent connection. The connection is
// closed when fn returns or timeout passes, whichever comes first; closing it
// ends a request the agent never answers.
func withAgent(dial AgentDialer, timeout time.Duration, fn func(agent.ExtendedAgent) error) error {
	conn, err := dial()
	if err != nil {
		return fmt.Errorf("%w: %w", errAgentUnreachable, err)
	}
	var timedOut atomic.Bool
	timer := time.AfterFunc(timeout, func() {
		timedOut.Store(true)
		_ = conn.Close()
	})
	defer func() {
		timer.Stop()
		_ = conn.Close()
	}()
	if err := fn(agent.NewClient(conn)); err != nil {
		if timedOut.Load() {
			return fmt.Errorf("the SSH agent did not answer within %s: %w", timeout, err)
		}
		return err
	}
	return nil
}

// agentSigner signs with one of the agent's keys, on a new connection for each
// signature.
type agentSigner struct {
	dial    AgentDialer
	key     *agent.Key
	timeout time.Duration
}

var _ ssh.AlgorithmSigner = (*agentSigner)(nil)

func (s *agentSigner) PublicKey() ssh.PublicKey { return s.key }

func (s *agentSigner) Sign(rand io.Reader, data []byte) (*ssh.Signature, error) {
	return s.SignWithAlgorithm(rand, data, "")
}

// SignWithAlgorithm asks the agent for the signature through the agent
// package's own signer, which maps the algorithm to the agent's flags.
func (s *agentSigner) SignWithAlgorithm(rand io.Reader, data []byte, algorithm string) (*ssh.Signature, error) {
	var sig *ssh.Signature
	err := withAgent(s.dial, s.timeout, func(client agent.ExtendedAgent) error {
		signers, err := client.Signers()
		if err != nil {
			return err
		}
		want := s.key.Marshal()
		for _, signer := range signers {
			if !bytes.Equal(signer.PublicKey().Marshal(), want) {
				continue
			}
			// The agent package's signers are AlgorithmSigners.
			algorithmSigner, ok := signer.(ssh.AlgorithmSigner)
			if !ok {
				return fmt.Errorf("the agent's signer for %s cannot choose an algorithm", s.key.Type())
			}
			sig, err = algorithmSigner.SignWithAlgorithm(rand, data, algorithm)
			return err
		}
		return errors.New("the SSH agent no longer holds the key")
	})
	if err != nil {
		return nil, fmt.Errorf("signing with the SSH agent: %w", err)
	}
	return sig, nil
}
