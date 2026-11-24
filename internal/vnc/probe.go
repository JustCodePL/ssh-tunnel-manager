package vnc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
)

// Probe reports whether the peer on conn greets like a VNC server. It reads
// only the server's protocol version, sends nothing, and closes conn when it
// returns or when ctx ends.
func Probe(ctx context.Context, conn net.Conn) (bool, error) {
	defer conn.Close()
	// SSH channels do not support deadlines, so cancellation closes the conn.
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()

	banner := make([]byte, versionLength)
	if _, err := io.ReadFull(conn, banner); err != nil {
		if ctx.Err() != nil {
			return false, fmt.Errorf("probing VNC: %w", ctx.Err())
		}
		// The port answered, but not with RFB.
		return false, nil
	}
	_, err := parseVersion(banner)
	if errors.Is(err, ErrNotVNC) {
		return false, nil
	}
	// An RFB server with a version the app cannot speak is still a VNC server;
	// connecting will explain the problem.
	return true, nil
}
