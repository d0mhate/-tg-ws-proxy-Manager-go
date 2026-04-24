package socks5

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"net"
	"strconv"
	"testing"
	"time"

	"tg-ws-proxy/internal/config"
)

func TestBuildUDPAssociatePacketRoundTripDomain(t *testing.T) {
	packet, err := buildUDPAssociatePacket("example.com", 443, []byte{0x01, 0x02, 0x03})
	if err != nil {
		t.Fatalf("buildUDPAssociatePacket returned error: %v", err)
	}

	parsed, err := parseUDPAssociatePacket(packet)
	if err != nil {
		t.Fatalf("parseUDPAssociatePacket returned error: %v", err)
	}
	if parsed.Host != "example.com" {
		t.Fatalf("unexpected host after round-trip: %q", parsed.Host)
	}
	if parsed.Port != 443 {
		t.Fatalf("unexpected port after round-trip: %d", parsed.Port)
	}
	if !bytes.Equal(parsed.Payload, []byte{0x01, 0x02, 0x03}) {
		t.Fatalf("unexpected payload after round-trip: %x", parsed.Payload)
	}
}

func TestParseUDPAssociatePacketRejectsFragmentation(t *testing.T) {
	_, err := parseUDPAssociatePacket([]byte{
		0x00, 0x00, 0x01, 0x01,
		127, 0, 0, 1,
		0x00, 0x35,
		0xaa,
	})
	if !errors.Is(err, errUDPFragmentUnsupported) {
		t.Fatalf("expected errUDPFragmentUnsupported, got %v", err)
	}
}

func TestWriteReplyUsesGeneralFailureForUnknownStatus(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	done := make(chan error, 1)
	go func() {
		done <- writeReply(serverConn, 0xff)
	}()

	reply := make([]byte, 10)
	if _, err := io.ReadFull(clientConn, reply); err != nil {
		t.Fatalf("failed to read reply: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatalf("writeReply returned error: %v", err)
	}

	if reply[1] != 0x05 {
		t.Fatalf("unexpected fallback reply status: %d", reply[1])
	}
}

func TestHandleConnUDPAssociateRelaysDatagrams(t *testing.T) {
	echoPC, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start udp echo server: %v", err)
	}
	defer echoPC.Close()

	echoDone := make(chan struct{})
	go func() {
		defer close(echoDone)
		buf := make([]byte, 2048)
		n, src, readErr := echoPC.ReadFrom(buf)
		if readErr != nil {
			return
		}
		_, _ = echoPC.WriteTo(buf[:n], src)
	}()

	srv := NewServer(config.Default(), log.New(io.Discard, "", 0))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen for tcp control channel: %v", err)
	}
	defer listener.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		srv.handleConn(ctx, conn)
	}()

	controlConn, err := net.Dial("tcp4", listener.Addr().String())
	if err != nil {
		t.Fatalf("failed to connect tcp control channel: %v", err)
	}

	if _, err := controlConn.Write([]byte{0x05, 0x01, 0x00}); err != nil {
		t.Fatalf("failed to send auth greeting: %v", err)
	}

	authReply := make([]byte, 2)
	if _, err := io.ReadFull(controlConn, authReply); err != nil {
		t.Fatalf("failed to read auth reply: %v", err)
	}
	if !bytes.Equal(authReply, []byte{0x05, 0x00}) {
		t.Fatalf("unexpected auth reply: %v", authReply)
	}

	if _, err := controlConn.Write([]byte{
		0x05, 0x03, 0x00, 0x01,
		0x00, 0x00, 0x00, 0x00,
		0x00, 0x00,
	}); err != nil {
		t.Fatalf("failed to send udp associate request: %v", err)
	}

	bindHost, bindPort := readSocksReplyAddr(t, controlConn)
	if bindPort == 0 {
		t.Fatal("expected udp associate bind port")
	}

	clientPC, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to open udp client socket: %v", err)
	}
	defer clientPC.Close()

	payload := []byte("hello over udp associate")
	echoAddr := echoPC.LocalAddr().(*net.UDPAddr)
	packet, err := buildUDPAssociatePacket(echoAddr.IP.String(), echoAddr.Port, payload)
	if err != nil {
		t.Fatalf("failed to build udp associate packet: %v", err)
	}

	udpBindAddr, err := net.ResolveUDPAddr("udp4", net.JoinHostPort(bindHost, strconv.Itoa(bindPort)))
	if err != nil {
		t.Fatalf("failed to resolve udp bind addr: %v", err)
	}
	if _, err := clientPC.WriteTo(packet, udpBindAddr); err != nil {
		t.Fatalf("failed to send udp associate payload: %v", err)
	}

	buf := make([]byte, 2048)
	_ = clientPC.SetDeadline(time.Now().Add(2 * time.Second))
	n, _, err := clientPC.ReadFrom(buf)
	if err != nil {
		t.Fatalf("failed to read udp associate response: %v", err)
	}

	resp, err := parseUDPAssociatePacket(buf[:n])
	if err != nil {
		t.Fatalf("failed to parse udp associate response: %v", err)
	}
	if resp.Host != echoAddr.IP.String() || resp.Port != echoAddr.Port {
		t.Fatalf("unexpected udp response source: %s:%d", resp.Host, resp.Port)
	}
	if !bytes.Equal(resp.Payload, payload) {
		t.Fatalf("unexpected udp response payload: %q", resp.Payload)
	}

	_ = controlConn.Close()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("udp associate handler did not complete after control channel close")
	}
	select {
	case <-echoDone:
	case <-time.After(2 * time.Second):
		t.Fatal("udp echo server did not complete")
	}
}
