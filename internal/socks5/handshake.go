package socks5

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"time"

	"tg-ws-proxy/internal/config"
)

var socksReplies = map[byte][]byte{
	0x00: {0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0},
	0x05: {0x05, 0x05, 0x00, 0x01, 0, 0, 0, 0, 0, 0},
	0x07: {0x05, 0x07, 0x00, 0x01, 0, 0, 0, 0, 0, 0},
	0x08: {0x05, 0x08, 0x00, 0x01, 0, 0, 0, 0, 0, 0},
}

type request struct {
	Cmd     byte
	DstHost string
	DstPort int
}

type handshakeError struct {
	stage      string
	err        error
	firstBytes []byte
}

func (e *handshakeError) Error() string {
	if e == nil || e.err == nil {
		return ""
	}
	return e.err.Error()
}

func (e *handshakeError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

func handshake(conn net.Conn, cfg config.Config) (request, error) {
	var req request
	buf := make([]byte, 262)

	if _, err := io.ReadFull(conn, buf[:2]); err != nil {
		return req, &handshakeError{stage: "greeting", err: err}
	}
	if buf[0] != 0x05 {
		return req, &handshakeError{stage: "greeting", err: errors.New("unsupported socks version"), firstBytes: append([]byte(nil), buf[:2]...)}
	}

	nMethods := int(buf[1])
	if nMethods == 0 {
		return req, &handshakeError{stage: "greeting", err: errors.New("no auth methods provided"), firstBytes: append([]byte(nil), buf[:2]...)}
	}
	if _, err := io.ReadFull(conn, buf[:nMethods]); err != nil {
		return req, &handshakeError{stage: "greeting", err: err}
	}
	method, err := negotiateAuthMethod(buf[:nMethods], cfg)
	if err != nil {
		_, _ = conn.Write([]byte{0x05, 0xff})
		return req, &handshakeError{stage: "greeting", err: err}
	}
	if _, err := conn.Write([]byte{0x05, method}); err != nil {
		return req, &handshakeError{stage: "greeting", err: err}
	}
	if method == socksAuthUserPass {
		if err := authenticateUserPass(conn, cfg.Username, cfg.Password); err != nil {
			return req, &handshakeError{stage: "auth", err: err}
		}
	}

	if _, err := io.ReadFull(conn, buf[:4]); err != nil {
		return req, &handshakeError{stage: "request", err: err}
	}
	if buf[0] != 0x05 {
		return req, &handshakeError{stage: "request", err: errors.New("unsupported socks version")}
	}
	req.Cmd = buf[1]
	if req.Cmd != socksCmdConnect && req.Cmd != socksCmdUDPAssociate {
		return req, &handshakeError{stage: "request", err: errors.New("only connect and udp associate are supported")}
	}

	switch buf[3] {
	case 0x01:
		if _, err := io.ReadFull(conn, buf[:4]); err != nil {
			return req, &handshakeError{stage: "request", err: err}
		}
		req.DstHost = net.IP(buf[:4]).String()
	case 0x03:
		if _, err := io.ReadFull(conn, buf[:1]); err != nil {
			return req, &handshakeError{stage: "request", err: err}
		}
		size := int(buf[0])
		if _, err := io.ReadFull(conn, buf[:size]); err != nil {
			return req, &handshakeError{stage: "request", err: err}
		}
		req.DstHost = string(buf[:size])
	case 0x04:
		if _, err := io.ReadFull(conn, buf[:16]); err != nil {
			return req, &handshakeError{stage: "request", err: err}
		}
		req.DstHost = net.IP(buf[:16]).String()
	default:
		return req, &handshakeError{stage: "request", err: errors.New("address type not supported")}
	}

	if _, err := io.ReadFull(conn, buf[:2]); err != nil {
		return req, &handshakeError{stage: "request", err: err}
	}
	req.DstPort = int(binary.BigEndian.Uint16(buf[:2]))
	return req, nil
}

func negotiateAuthMethod(methods []byte, cfg config.Config) (byte, error) {
	required := byte(socksAuthNoAuth)
	if cfg.Username != "" || cfg.Password != "" {
		required = byte(socksAuthUserPass)
	}
	for _, method := range methods {
		if method == required {
			return required, nil
		}
	}
	if required == socksAuthUserPass {
		return 0xff, errors.New("username/password auth is required")
	}
	return 0xff, errors.New("no supported auth methods provided")
}

func authenticateUserPass(conn net.Conn, username, password string) error {
	buf := make([]byte, 513)
	if _, err := io.ReadFull(conn, buf[:2]); err != nil {
		return err
	}
	if buf[0] != 0x01 {
		_, _ = conn.Write([]byte{0x01, 0x01})
		return errors.New("unsupported username/password auth version")
	}

	userLen := int(buf[1])
	if _, err := io.ReadFull(conn, buf[:userLen]); err != nil {
		return err
	}
	gotUser := string(buf[:userLen])

	if _, err := io.ReadFull(conn, buf[:1]); err != nil {
		return err
	}
	passLen := int(buf[0])
	if _, err := io.ReadFull(conn, buf[:passLen]); err != nil {
		return err
	}
	gotPass := string(buf[:passLen])

	if gotUser != username || gotPass != password {
		_, _ = conn.Write([]byte{0x01, 0x01})
		return errors.New(errInvalidUsernamePassword)
	}
	if _, err := conn.Write([]byte{0x01, 0x00}); err != nil {
		return err
	}
	return nil
}

func writeReply(conn net.Conn, status byte) error {
	return writeReplyAddr(conn, status, net.IPv4zero.String(), 0)
}

func writeReplyAddr(conn net.Conn, status byte, host string, port int) error {
	reply, err := buildReply(status, host, port)
	if err != nil {
		reply = socksReplies[0x05]
	}
	_, err = conn.Write(reply)
	return err
}

func buildReply(status byte, host string, port int) ([]byte, error) {
	replyStatus := status
	reply, ok := socksReplies[replyStatus]
	if !ok {
		replyStatus = 0x05
		reply = socksReplies[replyStatus]
	}
	if host == "" && port == 0 {
		return append([]byte(nil), reply...), nil
	}

	ip := net.ParseIP(host)
	if ip4 := ip.To4(); ip4 != nil {
		out := []byte{0x05, replyStatus, 0x00, 0x01}
		out = append(out, ip4...)
		var portBuf [2]byte
		binary.BigEndian.PutUint16(portBuf[:], uint16(port))
		out = append(out, portBuf[:]...)
		return out, nil
	}
	if ip16 := ip.To16(); ip16 != nil {
		out := []byte{0x05, replyStatus, 0x00, 0x04}
		out = append(out, ip16...)
		var portBuf [2]byte
		binary.BigEndian.PutUint16(portBuf[:], uint16(port))
		out = append(out, portBuf[:]...)
		return out, nil
	}
	if len(host) > 255 {
		return nil, errors.New("domain name too long")
	}
	out := []byte{0x05, replyStatus, 0x00, 0x03, byte(len(host))}
	out = append(out, []byte(host)...)
	var portBuf [2]byte
	binary.BigEndian.PutUint16(portBuf[:], uint16(port))
	out = append(out, portBuf[:]...)
	return out, nil
}

func readWithContext(ctx context.Context, conn net.Conn, buf []byte, timeout time.Duration) (int, error) {
	if timeout > 0 {
		_ = conn.SetReadDeadline(time.Now().Add(timeout))
	}

	type readResult struct {
		n   int
		err error
	}
	done := make(chan readResult, 1)
	go func() {
		n, err := io.ReadFull(conn, buf)
		done <- readResult{n: n, err: err}
	}()

	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case result := <-done:
		if timeout > 0 {
			_ = conn.SetReadDeadline(time.Time{})
		}
		return result.n, result.err
	}
}

func normalizeEOF(err error) error {
	if errors.Is(err, io.EOF) {
		return nil
	}
	return err
}

func remoteAddr(conn net.Conn) string {
	if conn == nil || conn.RemoteAddr() == nil {
		return "unknown"
	}
	return conn.RemoteAddr().String()
}

func choosePatchedDC(dc int, isMedia bool) int {
	if isMedia {
		return -dc
	}
	return dc
}
