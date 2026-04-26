package wsbridge

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"
)

func TestBuildFrameMaskedRoundTrip(t *testing.T) {
	payload := []byte("hello websocket")

	frame, err := buildFrame(opBinary, payload, true)
	if err != nil {
		t.Fatalf("buildFrame returned error: %v", err)
	}

	client := &Client{reader: bufio.NewReader(bytes.NewReader(frame))}
	opcode, got, err := client.readFrame()
	if err != nil {
		t.Fatalf("readFrame returned error: %v", err)
	}
	if opcode != opBinary {
		t.Fatalf("unexpected opcode: got %d want %d", opcode, opBinary)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("payload mismatch: got %q want %q", got, payload)
	}
}

func TestWSAcceptKeyRFCExample(t *testing.T) {
	got := wsAcceptKey("dGhlIHNhbXBsZSBub25jZQ==")
	want := "s3pPLMBiTxaQ9kYGzzhZRbK+xOo="
	if got != want {
		t.Fatalf("unexpected accept key: got %q want %q", got, want)
	}
}

func TestRecvRespondsToPingThenReturnsBinary(t *testing.T) {
	pingPayload := []byte("ping")
	binaryPayload := []byte("hello")

	stream := append(frameForServer(t, opPing, pingPayload), frameForServer(t, opBinary, binaryPayload)...)
	conn := newMockConn(nil)
	client := &Client{
		conn:   conn,
		reader: bufio.NewReader(bytes.NewReader(stream)),
	}

	got, err := client.Recv()
	if err != nil {
		t.Fatalf("Recv returned error: %v", err)
	}
	if !bytes.Equal(got, binaryPayload) {
		t.Fatalf("unexpected payload: got %q want %q", got, binaryPayload)
	}

	replyReader := &Client{reader: bufio.NewReader(bytes.NewReader(conn.writeBuf.Bytes()))}
	opcode, payload, err := replyReader.readFrame()
	if err != nil {
		t.Fatalf("failed to parse pong reply: %v", err)
	}
	if opcode != opPong {
		t.Fatalf("unexpected control opcode: got %d want %d", opcode, opPong)
	}
	if !bytes.Equal(payload, pingPayload) {
		t.Fatalf("unexpected pong payload: got %q want %q", payload, pingPayload)
	}
}

func TestRecvHandlesCloseFrame(t *testing.T) {
	stream := frameForServer(t, opClose, []byte{0x03, 0xe8})
	conn := newMockConn(nil)
	client := &Client{
		conn:   conn,
		reader: bufio.NewReader(bytes.NewReader(stream)),
	}

	got, err := client.Recv()
	if err != nil {
		t.Fatalf("Recv returned error: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil payload on close, got %q", got)
	}

	replyReader := &Client{reader: bufio.NewReader(bytes.NewReader(conn.writeBuf.Bytes()))}
	opcode, payload, err := replyReader.readFrame()
	if err != nil {
		t.Fatalf("failed to parse close reply: %v", err)
	}
	if opcode != opClose {
		t.Fatalf("unexpected close opcode: got %d want %d", opcode, opClose)
	}
	if !bytes.Equal(payload, []byte{0x03, 0xe8}) {
		t.Fatalf("unexpected close payload: got %v", payload)
	}
}

func TestHandshakeReturnsRedirectError(t *testing.T) {
	conn := newMockConn([]byte("HTTP/1.1 302 Found\r\nLocation: https://example.invalid\r\n\r\n"))
	client := &Client{
		conn:   conn,
		reader: bufio.NewReader(conn),
	}

	err := client.handshake("kws2.web.telegram.org", "/apiws")
	if err == nil {
		t.Fatal("expected handshake error")
	}

	hErr, ok := err.(*HandshakeError)
	if !ok {
		t.Fatalf("expected HandshakeError, got %T", err)
	}
	if !hErr.IsRedirect() {
		t.Fatal("expected redirect handshake error")
	}
	if hErr.Location != "https://example.invalid" {
		t.Fatalf("unexpected redirect location: %q", hErr.Location)
	}
}

func TestHandshakeRejectsUnexpectedAcceptHeader(t *testing.T) {
	conn := newMockConn([]byte("HTTP/1.1 101 Switching Protocols\r\nSec-WebSocket-Accept: invalid\r\n\r\n"))
	client := &Client{
		conn:   conn,
		reader: bufio.NewReader(conn),
	}

	err := client.handshake("kws2.web.telegram.org", "/apiws")
	if err == nil {
		t.Fatal("expected accept header validation error")
	}
	if err.Error() != "unexpected Sec-WebSocket-Accept header" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDialUsesDialEndpointFields(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	go func() {
		defer serverConn.Close()
		reader := bufio.NewReader(serverConn)
		line, _ := reader.ReadString('\n')
		if line != "GET /apiws HTTP/1.1\r\n" {
			t.Errorf("unexpected request line: %q", line)
			return
		}
		for {
			header, err := reader.ReadString('\n')
			if err != nil {
				t.Errorf("read header: %v", err)
				return
			}
			if header == "\r\n" {
				break
			}
			if header == "Host: kws2.example.com\r\n" {
				_, _ = io.WriteString(serverConn, "HTTP/1.1 101 Switching Protocols\r\nSec-WebSocket-Accept: invalid\r\n\r\n")
				return
			}
		}
	}()

	client := NewClient(clientConn)
	err := client.handshake("kws2.example.com", "/apiws")
	if err == nil {
		t.Fatal("expected handshake validation error")
	}
}

func TestCloseWritesCloseFrame(t *testing.T) {
	conn := newMockConn(nil)
	client := &Client{
		conn:   conn,
		reader: bufio.NewReader(bytes.NewReader(nil)),
	}

	if err := client.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	replyReader := &Client{reader: bufio.NewReader(bytes.NewReader(conn.writeBuf.Bytes()))}
	opcode, payload, err := replyReader.readFrame()
	if err != nil {
		t.Fatalf("failed to parse close frame: %v", err)
	}
	if opcode != opClose {
		t.Fatalf("unexpected close opcode: got %d want %d", opcode, opClose)
	}
	if len(payload) != 0 {
		t.Fatalf("expected empty close payload, got %v", payload)
	}
}

func TestCloseDoesNotBlockOnStuckWrite(t *testing.T) {
	conn := newBlockingWriteConn()
	client := &Client{
		conn:   conn,
		reader: bufio.NewReader(bytes.NewReader(nil)),
	}

	done := make(chan error, 1)
	go func() {
		done <- client.Close()
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected close to report underlying close error")
		}
		if !errors.Is(err, errBlockingConnClosed) {
			t.Fatalf("unexpected close error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Close blocked on stuck write")
	}

	if !conn.closed {
		t.Fatal("expected underlying connection to be closed")
	}
}

func TestBindConnToContextInterruptsBlockedReadOnCancel(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	ctx, cancel := context.WithCancel(context.Background())
	release := bindConnToContext(ctx, serverConn, 0)
	defer release()

	errCh := make(chan error, 1)
	go func() {
		buf := make([]byte, 1)
		_, err := serverConn.Read(buf)
		errCh <- err
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("expected read to be interrupted")
		}
	case <-time.After(time.Second):
		t.Fatal("blocked read was not interrupted by context cancellation")
	}
}

func TestEffectiveConnDeadlinePrefersEarlierContextDeadline(t *testing.T) {
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(50*time.Millisecond))
	defer cancel()

	deadline, ok := effectiveConnDeadline(ctx, time.Second)
	if !ok {
		t.Fatal("expected deadline")
	}
	if remaining := time.Until(deadline); remaining > 200*time.Millisecond {
		t.Fatalf("expected earlier context deadline, got remaining=%s", remaining)
	}
}

func frameForServer(t *testing.T, opcode byte, payload []byte) []byte {
	t.Helper()

	frame, err := buildFrame(opcode, payload, false)
	if err != nil {
		t.Fatalf("buildFrame returned error: %v", err)
	}
	return frame
}

type mockConn struct {
	readBuf        *bytes.Reader
	writeBuf       bytes.Buffer
	closed         bool
	readDeadline   time.Time
	timeoutOnEmpty bool
}

func newMockConn(readData []byte) *mockConn {
	return &mockConn{readBuf: bytes.NewReader(readData)}
}

func (c *mockConn) Read(p []byte) (int, error) {
	if c.closed {
		return 0, io.EOF
	}
	if c.readBuf == nil {
		if c.timeoutOnEmpty && !c.readDeadline.IsZero() && !time.Now().Before(c.readDeadline) {
			return 0, timeoutError{}
		}
		return 0, io.EOF
	}
	n, err := c.readBuf.Read(p)
	if err == io.EOF && c.timeoutOnEmpty && !c.readDeadline.IsZero() && !time.Now().Before(c.readDeadline) {
		return 0, timeoutError{}
	}
	return n, err
}

func (c *mockConn) Write(p []byte) (int, error) {
	return c.writeBuf.Write(p)
}

func (c *mockConn) Close() error {
	c.closed = true
	return nil
}

func (c *mockConn) LocalAddr() net.Addr         { return dummyAddr("local") }
func (c *mockConn) RemoteAddr() net.Addr        { return dummyAddr("remote") }
func (c *mockConn) SetDeadline(time.Time) error { return nil }
func (c *mockConn) SetReadDeadline(t time.Time) error {
	c.readDeadline = t
	return nil
}
func (c *mockConn) SetWriteDeadline(time.Time) error { return nil }

type timeoutError struct{}

func (timeoutError) Error() string   { return "i/o timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

type dummyAddr string

func (a dummyAddr) Network() string { return "tcp" }
func (a dummyAddr) String() string  { return string(a) }

var errBlockingConnClosed = errors.New("blocking conn closed")

type blockingWriteConn struct {
	mu            sync.Mutex
	writeDeadline time.Time
	closed        bool
}

func newBlockingWriteConn() *blockingWriteConn {
	return &blockingWriteConn{}
}

func (c *blockingWriteConn) Read([]byte) (int, error) {
	return 0, io.EOF
}

func (c *blockingWriteConn) Write([]byte) (int, error) {
	for {
		c.mu.Lock()
		closed := c.closed
		deadline := c.writeDeadline
		c.mu.Unlock()

		if closed {
			return 0, errBlockingConnClosed
		}
		if !deadline.IsZero() && !time.Now().Before(deadline) {
			return 0, timeoutError{}
		}
		time.Sleep(time.Millisecond)
	}
}

func (c *blockingWriteConn) Close() error {
	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()
	return errBlockingConnClosed
}

func (c *blockingWriteConn) LocalAddr() net.Addr                { return dummyAddr("local") }
func (c *blockingWriteConn) RemoteAddr() net.Addr               { return dummyAddr("remote") }
func (c *blockingWriteConn) SetDeadline(t time.Time) error      { return c.SetReadDeadline(t) }
func (c *blockingWriteConn) SetReadDeadline(time.Time) error    { return nil }
func (c *blockingWriteConn) SetWriteDeadline(t time.Time) error {
	c.mu.Lock()
	c.writeDeadline = t
	c.mu.Unlock()
	return nil
}
