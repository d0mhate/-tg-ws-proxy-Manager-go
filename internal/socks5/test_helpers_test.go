package socks5

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"io"
	"net"
	"testing"
	"time"

	"tg-ws-proxy/internal/config"
)

func runHandshakeClient(t *testing.T, request []byte) (requestOut request, err error) {
	t.Helper()
	return runHandshakeClientWithConfig(t, config.Default(), request[:3], nil, request[3:])
}

func runHandshakeClientWithConfig(t *testing.T, cfg config.Config, greeting []byte, authPayload []byte, requestPayload []byte) (requestOut request, err error) {
	t.Helper()

	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	clientErrCh := make(chan error, 1)
	go func() {
		defer close(clientErrCh)

		if _, writeErr := clientConn.Write(greeting); writeErr != nil {
			clientErrCh <- writeErr
			return
		}

		reply := make([]byte, 2)
		if _, readErr := io.ReadFull(clientConn, reply); readErr != nil {
			clientErrCh <- readErr
			return
		}
		expectedMethod := byte(socksAuthNoAuth)
		if cfg.Username != "" || cfg.Password != "" {
			expectedMethod = socksAuthUserPass
		}
		if !bytes.Equal(reply, []byte{0x05, expectedMethod}) {
			clientErrCh <- io.ErrUnexpectedEOF
			return
		}

		if expectedMethod == socksAuthUserPass {
			if _, writeErr := clientConn.Write(authPayload); writeErr != nil {
				clientErrCh <- writeErr
				return
			}
			authReply := make([]byte, 2)
			if _, readErr := io.ReadFull(clientConn, authReply); readErr != nil {
				clientErrCh <- readErr
				return
			}
			if !bytes.Equal(authReply, []byte{0x01, 0x00}) {
				clientErrCh <- io.ErrUnexpectedEOF
				return
			}
		}

		if _, writeErr := clientConn.Write(requestPayload); writeErr != nil {
			clientErrCh <- writeErr
			return
		}
	}()

	requestOut, err = handshake(serverConn, cfg)
	if clientErr := <-clientErrCh; clientErr != nil {
		t.Fatalf("client side of handshake failed: %v", clientErr)
	}
	return requestOut, err
}

func runHandleConnFlow(t *testing.T, srv *Server, connectReq []byte, init []byte, assertReply func([]byte)) {
	t.Helper()
	runHandleConnFlowWithAuth(t, srv, connectReq, init, "", "", assertReply)
}

func runHandleConnFlowWithAuth(t *testing.T, srv *Server, connectReq []byte, init []byte, username string, password string, assertReply func([]byte)) {
	t.Helper()

	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		srv.handleConn(ctx, serverConn)
	}()

	greeting := []byte{0x05, 0x01, 0x00}
	expectedMethod := byte(socksAuthNoAuth)
	if username != "" || password != "" {
		greeting = []byte{0x05, 0x02, socksAuthNoAuth, socksAuthUserPass}
		expectedMethod = socksAuthUserPass
	}
	if _, err := clientConn.Write(greeting); err != nil {
		t.Fatalf("failed to send auth greeting: %v", err)
	}

	authReply := make([]byte, 2)
	if _, err := io.ReadFull(clientConn, authReply); err != nil {
		t.Fatalf("failed to read auth reply: %v", err)
	}
	if !bytes.Equal(authReply, []byte{0x05, expectedMethod}) {
		t.Fatalf("unexpected auth reply: %v", authReply)
	}
	if expectedMethod == socksAuthUserPass {
		if _, err := clientConn.Write(buildUserPassAuthPayload(username, password)); err != nil {
			t.Fatalf("failed to send username/password auth payload: %v", err)
		}
		userPassReply := make([]byte, 2)
		if _, err := io.ReadFull(clientConn, userPassReply); err != nil {
			t.Fatalf("failed to read username/password auth reply: %v", err)
		}
		if !bytes.Equal(userPassReply, []byte{0x01, 0x00}) {
			t.Fatalf("unexpected username/password auth reply: %v", userPassReply)
		}
	}

	if _, err := clientConn.Write(connectReq); err != nil {
		t.Fatalf("failed to send connect request: %v", err)
	}

	reply := make([]byte, 10)
	if _, err := io.ReadFull(clientConn, reply); err != nil {
		t.Fatalf("failed to read connect reply: %v", err)
	}
	if assertReply != nil {
		assertReply(reply)
	}

	if len(init) > 0 {
		if _, err := clientConn.Write(init); err != nil {
			t.Fatalf("failed to send init packet: %v", err)
		}
	}

	_ = clientConn.Close()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handleConn did not complete")
	}
}

func buildUserPassAuthPayload(username, password string) []byte {
	out := []byte{0x01, byte(len(username))}
	out = append(out, []byte(username)...)
	out = append(out, byte(len(password)))
	out = append(out, []byte(password)...)
	return out
}

func runHandleConnInvalidAuthAttempt(t *testing.T, srv *Server, username string, password string) {
	t.Helper()

	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		srv.handleConn(ctx, serverConn)
	}()

	if _, err := clientConn.Write([]byte{0x05, 0x02, socksAuthNoAuth, socksAuthUserPass}); err != nil {
		t.Fatalf("failed to send auth greeting: %v", err)
	}

	authReply := make([]byte, 2)
	if _, err := io.ReadFull(clientConn, authReply); err != nil {
		t.Fatalf("failed to read auth reply: %v", err)
	}
	if !bytes.Equal(authReply, []byte{0x05, socksAuthUserPass}) {
		t.Fatalf("unexpected auth reply: %v", authReply)
	}

	if _, err := clientConn.Write(buildUserPassAuthPayload(username, password)); err != nil {
		t.Fatalf("failed to send invalid username/password auth payload: %v", err)
	}

	userPassReply := make([]byte, 2)
	if _, err := io.ReadFull(clientConn, userPassReply); err != nil {
		t.Fatalf("failed to read invalid username/password auth reply: %v", err)
	}
	if !bytes.Equal(userPassReply, []byte{0x01, 0x01}) {
		t.Fatalf("unexpected invalid username/password auth reply: %v", userPassReply)
	}

	_ = clientConn.Close()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handleConn did not complete after invalid auth attempt")
	}
}

func runHandleConnRawHandshakeAttempt(t *testing.T, srv *Server, payload []byte) {
	t.Helper()

	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		srv.handleConn(ctx, serverConn)
	}()

	if len(payload) > 0 {
		if _, err := clientConn.Write(payload); err != nil {
			t.Fatalf("failed to send raw handshake payload: %v", err)
		}
	}

	_ = clientConn.Close()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handleConn did not complete after raw handshake attempt")
	}
}

func ipv4ConnectRequest(ip string, port int) []byte {
	out := []byte{0x05, 0x01, 0x00, 0x01}
	out = append(out, net.ParseIP(ip).To4()...)
	var portBuf [2]byte
	binary.BigEndian.PutUint16(portBuf[:], uint16(port))
	out = append(out, portBuf[:]...)
	return out
}

func ipv6ConnectRequest(ip net.IP) []byte {
	return ipv6ConnectRequestWithPort(ip, 443)
}

func ipv6ConnectRequestWithPort(ip net.IP, port int) []byte {
	out := []byte{0x05, 0x01, 0x00, 0x04}
	out = append(out, ip.To16()...)
	var portBuf [2]byte
	binary.BigEndian.PutUint16(portBuf[:], uint16(port))
	out = append(out, portBuf[:]...)
	return out
}

func makeMTProtoInitPacket(t *testing.T, proto uint32, dc int16) []byte {
	t.Helper()

	init := make([]byte, 64)
	for i := range init {
		init[i] = byte(i + 1)
	}

	var plain [8]byte
	binary.LittleEndian.PutUint32(plain[:4], proto)
	binary.LittleEndian.PutUint16(plain[4:6], uint16(dc))

	keystream := initKeystreamForTest(t, init)
	for i := 0; i < len(plain); i++ {
		init[56+i] = plain[i] ^ keystream[56+i]
	}
	return init
}

func initKeystreamForTest(t *testing.T, init []byte) []byte {
	t.Helper()

	block, err := aes.NewCipher(init[8:40])
	if err != nil {
		t.Fatalf("aes.NewCipher failed: %v", err)
	}
	stream := cipher.NewCTR(block, init[40:56])
	zero := make([]byte, 64)
	keystream := make([]byte, 64)
	stream.XORKeyStream(keystream, zero)
	return keystream
}

func readSocksReplyAddr(t *testing.T, conn net.Conn) (string, int) {
	t.Helper()

	head := make([]byte, 4)
	if _, err := io.ReadFull(conn, head); err != nil {
		t.Fatalf("failed to read socks reply head: %v", err)
	}
	if head[0] != 0x05 {
		t.Fatalf("unexpected socks version in reply: %d", head[0])
	}
	if head[1] != 0x00 {
		t.Fatalf("unexpected socks reply status: %d", head[1])
	}

	var host string
	switch head[3] {
	case 0x01:
		addr := make([]byte, 4)
		if _, err := io.ReadFull(conn, addr); err != nil {
			t.Fatalf("failed to read ipv4 reply addr: %v", err)
		}
		host = net.IP(addr).String()
	case 0x04:
		addr := make([]byte, 16)
		if _, err := io.ReadFull(conn, addr); err != nil {
			t.Fatalf("failed to read ipv6 reply addr: %v", err)
		}
		host = net.IP(addr).String()
	case 0x03:
		size := make([]byte, 1)
		if _, err := io.ReadFull(conn, size); err != nil {
			t.Fatalf("failed to read domain size: %v", err)
		}
		addr := make([]byte, int(size[0]))
		if _, err := io.ReadFull(conn, addr); err != nil {
			t.Fatalf("failed to read domain reply addr: %v", err)
		}
		host = string(addr)
	default:
		t.Fatalf("unexpected socks reply atyp: %d", head[3])
	}

	portBuf := make([]byte, 2)
	if _, err := io.ReadFull(conn, portBuf); err != nil {
		t.Fatalf("failed to read reply port: %v", err)
	}
	return host, int(binary.BigEndian.Uint16(portBuf))
}
