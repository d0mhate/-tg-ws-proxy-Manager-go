package socks5

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"
)

func TestBridgeTCPClosesBothSidesOnContextCancel(t *testing.T) {
	left := newBlockingConn()
	right := newBlockingConn()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- bridgeTCP(ctx, left, right)
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected bridgeTCP to return context error")
		}
	case <-time.After(time.Second):
		t.Fatal("bridgeTCP did not return after context cancellation")
	}

	if !left.isClosed() {
		t.Fatal("expected left conn to be closed")
	}
	if !right.isClosed() {
		t.Fatal("expected right conn to be closed")
	}
}

type blockingConn struct {
	mu     sync.Mutex
	closed bool
	done   chan struct{}
}

func newBlockingConn() *blockingConn {
	return &blockingConn{done: make(chan struct{})}
}

func (c *blockingConn) Read([]byte) (int, error) {
	<-c.done
	return 0, net.ErrClosed
}

func (c *blockingConn) Write([]byte) (int, error) {
	<-c.done
	return 0, net.ErrClosed
}

func (c *blockingConn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	close(c.done)
	return nil
}

func (c *blockingConn) isClosed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}

func (c *blockingConn) LocalAddr() net.Addr              { return bridgeDummyAddr("local") }
func (c *blockingConn) RemoteAddr() net.Addr             { return bridgeDummyAddr("remote") }
func (c *blockingConn) SetDeadline(time.Time) error      { return nil }
func (c *blockingConn) SetReadDeadline(time.Time) error  { return nil }
func (c *blockingConn) SetWriteDeadline(time.Time) error { return nil }

type bridgeDummyAddr string

func (a bridgeDummyAddr) Network() string { return "tcp" }
func (a bridgeDummyAddr) String() string  { return string(a) }
