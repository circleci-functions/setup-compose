// Package ready waits for published ports to accept connections.
package ready

import (
	"context"
	"errors"
	"net"
	"os"
	"time"
)

// Probe reports whether something is serving at addr.
//
// A connection alone proves nothing: Docker's userland proxy accepts on the
// published port whether or not the container listens, then closes the
// connection when it cannot reach it. So Probe reads briefly as well. A server
// that sends a greeting, or waits for the client to speak first, is ready; a
// connection closed straight away is the proxy finding nothing behind it.
func Probe(ctx context.Context, addr string) bool {
	d := net.Dialer{Timeout: time.Second}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return false
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	n, err := conn.Read(make([]byte, 1))
	if n > 0 {
		return true
	}
	return errors.Is(err, os.ErrDeadlineExceeded)
}

// Wait probes each address until all are ready or ctx is done, and returns
// those that never were.
func Wait(ctx context.Context, addrs []string, interval time.Duration) []string {
	pending := append([]string(nil), addrs...)
	for {
		var still []string
		for _, a := range pending {
			if !Probe(ctx, a) {
				still = append(still, a)
			}
		}
		if len(still) == 0 {
			return nil
		}
		pending = still
		select {
		case <-ctx.Done():
			return pending
		case <-time.After(interval):
		}
	}
}
