package ready

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"
)

// listen serves on a loopback port with handle, and returns its address.
func listen(t *testing.T, handle func(net.Conn)) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	assert.NilError(t, err)
	t.Cleanup(func() { _ = l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go handle(c)
		}
	}()
	return l.Addr().String()
}

func TestProbe(t *testing.T) {
	ctx := context.Background()

	waits := listen(t, func(c net.Conn) { time.Sleep(time.Second); _ = c.Close() })
	assert.Check(t, Probe(ctx, waits), "a server waiting for the client is ready")

	greets := listen(t, func(c net.Conn) { _, _ = c.Write([]byte("220 hello\r\n")); time.Sleep(time.Second); _ = c.Close() })
	assert.Check(t, Probe(ctx, greets), "a server that greets is ready")

	closes := listen(t, func(c net.Conn) { _ = c.Close() })
	assert.Check(t, !Probe(ctx, closes), "a proxy closing at once is not ready")

	l, err := net.Listen("tcp", "127.0.0.1:0")
	assert.NilError(t, err)
	refused := l.Addr().String()
	_ = l.Close()
	assert.Check(t, !Probe(ctx, refused), "a refused connection is not ready")
}

func TestWait(t *testing.T) {
	ok := listen(t, func(c net.Conn) { time.Sleep(time.Second); _ = c.Close() })
	assert.NilError(t, Wait(context.Background(), []string{ok}, 10*time.Millisecond))

	bad := listen(t, func(c net.Conn) { _ = c.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	err := Wait(ctx, []string{ok, bad}, 50*time.Millisecond)
	assert.Check(t, cmp.ErrorContains(err, bad))
	assert.Check(t, !strings.Contains(err.Error(), ok), "%q is ready", ok)
}
