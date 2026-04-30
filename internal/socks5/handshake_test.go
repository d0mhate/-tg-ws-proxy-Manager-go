package socks5

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"tg-ws-proxy/internal/config"
)

func TestHandshakeDomainConnect(t *testing.T) {
	req, err := runHandshakeClient(t, []byte{
		0x05, 0x01, 0x00,
		0x05, 0x01, 0x00, 0x03,
		0x0c,
		't', 'e', 'l', 'e', 'g', 'r', 'a', 'm', '.', 'o', 'r', 'g',
		0x01, 0xbb,
	})
	if err != nil {
		t.Fatalf("handshake returned error: %v", err)
	}

	if req.DstHost != "telegram.org" {
		t.Fatalf("unexpected destination host: %q", req.DstHost)
	}
	if req.DstPort != 443 {
		t.Fatalf("unexpected destination port: %d", req.DstPort)
	}
}

func TestHandshakeIPv4Connect(t *testing.T) {
	req, err := runHandshakeClient(t, []byte{
		0x05, 0x01, 0x00,
		0x05, 0x01, 0x00, 0x01,
		149, 154, 167, 220,
		0x01, 0xbb,
	})
	if err != nil {
		t.Fatalf("handshake returned error: %v", err)
	}

	if req.DstHost != testIPv4DC2 {
		t.Fatalf("unexpected destination host: %q", req.DstHost)
	}
	if req.DstPort != 443 {
		t.Fatalf("unexpected destination port: %d", req.DstPort)
	}
}

func TestHandshakeUDPAssociate(t *testing.T) {
	req, err := runHandshakeClient(t, []byte{
		0x05, 0x01, 0x00,
		0x05, 0x03, 0x00, 0x01,
		0x00, 0x00, 0x00, 0x00,
		0x00, 0x00,
	})
	if err != nil {
		t.Fatalf("handshake returned error: %v", err)
	}

	if req.Cmd != socksCmdUDPAssociate {
		t.Fatalf("unexpected command: %d", req.Cmd)
	}
	if req.DstHost != "0.0.0.0" {
		t.Fatalf("unexpected destination host: %q", req.DstHost)
	}
	if req.DstPort != 0 {
		t.Fatalf("unexpected destination port: %d", req.DstPort)
	}
}

func TestHandshakeRejectsUnsupportedCommand(t *testing.T) {
	_, err := runHandshakeClient(t, []byte{
		0x05, 0x01, 0x00,
		0x05, 0x02, 0x00, 0x01,
	})
	if err == nil {
		t.Fatal("expected handshake to reject unsupported command")
	}
}

func TestHandshakeUsernamePasswordConnect(t *testing.T) {
	cfg := config.Default()
	cfg.Username = "alice"
	cfg.Password = "secret"

	req, err := runHandshakeClientWithConfig(t, cfg,
		[]byte{0x05, 0x02, socksAuthNoAuth, socksAuthUserPass},
		buildUserPassAuthPayload("alice", "secret"),
		ipv4ConnectRequest(testIPv4DC2, 443),
	)
	if err != nil {
		t.Fatalf("handshake returned error: %v", err)
	}
	if req.DstHost != testIPv4DC2 || req.DstPort != 443 {
		t.Fatalf("unexpected request parsed after auth: %s:%d", req.DstHost, req.DstPort)
	}
}

func TestHandshakeAcceptsNoAuthWhenServerDoesNotRequireCredentials(t *testing.T) {
	req, err := runHandshakeClientWithConfig(t, config.Default(),
		[]byte{0x05, 0x02, socksAuthNoAuth, socksAuthUserPass},
		nil,
		ipv4ConnectRequest(testIPv4DC2, 443),
	)
	if err != nil {
		t.Fatalf("handshake returned error: %v", err)
	}
	if req.DstHost != testIPv4DC2 || req.DstPort != 443 {
		t.Fatalf("unexpected request parsed without auth: %s:%d", req.DstHost, req.DstPort)
	}
}

func TestHandshakeRejectsMissingUsernamePasswordMethod(t *testing.T) {
	cfg := config.Default()
	cfg.Username = "alice"
	cfg.Password = "secret"

	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	errCh := make(chan error, 1)
	go func() {
		defer close(errCh)
		if _, err := clientConn.Write([]byte{0x05, 0x01, socksAuthNoAuth}); err != nil {
			errCh <- err
			return
		}
		reply := make([]byte, 2)
		if _, err := io.ReadFull(clientConn, reply); err != nil {
			errCh <- err
			return
		}
		if !bytes.Equal(reply, []byte{0x05, 0xff}) {
			errCh <- errors.New("unexpected auth method rejection reply")
			return
		}
	}()

	_, err := handshake(serverConn, cfg)
	if err == nil {
		t.Fatal("expected handshake to reject missing username/password method")
	}
	if clientErr := <-errCh; clientErr != nil {
		t.Fatalf("client side of handshake failed: %v", clientErr)
	}
}

func TestHandshakeRejectsInvalidUsernamePassword(t *testing.T) {
	cfg := config.Default()
	cfg.Username = "alice"
	cfg.Password = "secret"

	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	errCh := make(chan error, 1)
	go func() {
		defer close(errCh)
		if _, err := clientConn.Write([]byte{0x05, 0x01, socksAuthUserPass}); err != nil {
			errCh <- err
			return
		}
		reply := make([]byte, 2)
		if _, err := io.ReadFull(clientConn, reply); err != nil {
			errCh <- err
			return
		}
		if !bytes.Equal(reply, []byte{0x05, socksAuthUserPass}) {
			errCh <- errors.New("unexpected auth method selection reply")
			return
		}
		if _, err := clientConn.Write(buildUserPassAuthPayload("alice", "wrong")); err != nil {
			errCh <- err
			return
		}
		authReply := make([]byte, 2)
		if _, err := io.ReadFull(clientConn, authReply); err != nil {
			errCh <- err
			return
		}
		if !bytes.Equal(authReply, []byte{0x01, 0x01}) {
			errCh <- errors.New("unexpected auth rejection reply")
			return
		}
	}()

	_, err := handshake(serverConn, cfg)
	if err == nil {
		t.Fatal("expected handshake to reject invalid username/password")
	}
	if clientErr := <-errCh; clientErr != nil {
		t.Fatalf("client side of handshake failed: %v", clientErr)
	}
}

func TestReadWithContextReturnsOnCancelWithoutLeakingReader(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	ctx, cancel := context.WithCancel(context.Background())
	resultCh := make(chan error, 1)
	go func() {
		buf := make([]byte, 8)
		_, err := readWithContext(ctx, serverConn, buf, 0)
		resultCh <- err
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case err := <-resultCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context cancellation, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("readWithContext did not return after context cancellation")
	}
}

func TestReadWithContextReturnsImmediatelyWhenContextAlreadyCanceled(t *testing.T) {
	conn := newBlockingConn()
	defer conn.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	buf := make([]byte, 8)
	_, err := readWithContext(ctx, conn, buf, time.Second)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
	if elapsed := time.Since(start); elapsed >= 100*time.Millisecond {
		t.Fatalf("expected immediate return for already-canceled context, took %s", elapsed)
	}
}
