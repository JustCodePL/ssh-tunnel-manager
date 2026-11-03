package rdp

import (
	"context"
	"fmt"
	"net"
)

// DefaultPort is the standard RDP port.
const DefaultPort = 3389

// probeRequest is an X.224 connection request offering TLS, NLA and extended
// NLA. It has no cookie, so the probe never discloses a user name.
var probeRequest = []byte{
	0x03, 0x00, 0x00, 0x13, // TPKT, 19 bytes
	0x0e, 0xe0, 0x00, 0x00, 0x00, 0x00, 0x00, // X.224 connection request
	0x01, 0x00, 0x08, 0x00, 0x0b, 0x00, 0x00, 0x00, // RDP_NEG_REQ
}

// Probe reports whether the peer on conn answers like an RDP server. It closes
// conn when it returns, or when ctx ends. A peer that rejects the offered
// security protocols still counts as an RDP server.
func Probe(ctx context.Context, conn net.Conn) (bool, error) {
	defer conn.Close()
	// SSH channels do not support deadlines, so cancellation closes the conn.
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()

	if _, err := conn.Write(probeRequest); err != nil {
		return false, fmt.Errorf("writing X.224 probe: %w", err)
	}
	packet, err := readTPKT(conn)
	if err != nil {
		if ctx.Err() != nil {
			return false, fmt.Errorf("probing RDP: %w", ctx.Err())
		}
		// The port answered, but not with RDP.
		return false, nil
	}
	return isConnectionConfirm(packet), nil
}
