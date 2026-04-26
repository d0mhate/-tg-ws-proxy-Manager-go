package socks5

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"strconv"
	"time"
)

var errUDPFragmentUnsupported = errors.New("udp fragmentation is not supported")

type udpPacket struct {
	Host    string
	Port    int
	Payload []byte
}

func (s *Server) handleUDPAssociate(ctx context.Context, conn net.Conn, req request) {
	clientAddr := remoteAddr(conn)
	pc, bindHost, bindPort, err := s.listenUDPAssociate(conn)
	if err != nil {
		s.logger.Printf("[%s] udp associate setup failed: %v", clientAddr, err)
		_ = writeReply(conn, 0x05)
		return
	}
	defer pc.Close()

	if err := writeReplyAddr(conn, 0x00, bindHost, bindPort); err != nil {
		s.logger.Printf("[%s] udp associate reply failed: %v", clientAddr, err)
		return
	}
	s.debugf("[%s] route=udp-associate bind=%s:%d expected=%s:%d", clientAddr, bindHost, bindPort, req.DstHost, req.DstPort)

	if err := s.serveUDPAssociation(ctx, conn, pc, req); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, io.EOF) {
		s.logger.Printf("[%s] udp associate ended with error: %v", clientAddr, err)
		return
	}
	s.debugf("[%s] udp association finished", clientAddr)
}

func (s *Server) listenUDPAssociate(conn net.Conn) (net.PacketConn, string, int, error) {
	tcpLocal, _ := conn.LocalAddr().(*net.TCPAddr)

	network := "udp4"
	bindHost := net.IPv4zero.String()
	replyHost := bindHost

	if tcpLocal != nil && tcpLocal.IP != nil && tcpLocal.IP.To4() == nil {
		network = "udp6"
		bindHost = "::"
		replyHost = "::"
	}
	if tcpLocal != nil && tcpLocal.IP != nil && !tcpLocal.IP.IsUnspecified() {
		bindHost = tcpLocal.IP.String()
		replyHost = bindHost
	}

	pc, err := net.ListenPacket(network, net.JoinHostPort(bindHost, "0"))
	if err != nil {
		return nil, "", 0, err
	}

	udpAddr, ok := pc.LocalAddr().(*net.UDPAddr)
	if !ok {
		_ = pc.Close()
		return nil, "", 0, errors.New("unexpected udp listener address")
	}
	if replyHost == "" || replyHost == "::" || replyHost == "0.0.0.0" {
		if udpAddr.IP != nil && !udpAddr.IP.IsUnspecified() {
			replyHost = udpAddr.IP.String()
		}
	}
	if replyHost == "" {
		replyHost = net.IPv4zero.String()
	}
	return pc, replyHost, udpAddr.Port, nil
}

func (s *Server) serveUDPAssociation(ctx context.Context, conn net.Conn, pc net.PacketConn, req request) error {
	associated := s.expectedUDPClientAddr(conn, req)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = io.Copy(io.Discard, conn)
	}()

	buf := make([]byte, 64*1024)
	for {
		_ = pc.SetReadDeadline(time.Now().Add(time.Second))
		n, src, err := pc.ReadFrom(buf)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-done:
					return nil
				default:
					continue
				}
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-done:
				return nil
			default:
				return err
			}
		}

		srcAddr, ok := src.(*net.UDPAddr)
		if !ok {
			continue
		}

		if isAssociatedUDPClient(srcAddr, associated) {
			packet, perr := parseUDPAssociatePacket(buf[:n])
			if perr != nil {
				s.debugf("[%s] udp client packet ignored: %v", remoteAddr(conn), perr)
				continue
			}
			if associated.Port == 0 {
				associated = &net.UDPAddr{IP: append(net.IP(nil), srcAddr.IP...), Port: srcAddr.Port}
			}
			dstAddr, derr := net.ResolveUDPAddr("udp", net.JoinHostPort(packet.Host, strconv.Itoa(packet.Port)))
			if derr != nil {
				s.debugf("[%s] udp destination resolve failed for %s:%d: %v", remoteAddr(conn), packet.Host, packet.Port, derr)
				continue
			}
			if _, werr := pc.WriteTo(packet.Payload, dstAddr); werr != nil {
				return werr
			}
			continue
		}

		if associated == nil || associated.Port == 0 {
			continue
		}
		payload, perr := buildUDPAssociatePacket(srcAddr.IP.String(), srcAddr.Port, buf[:n])
		if perr != nil {
			continue
		}
		if _, werr := pc.WriteTo(payload, associated); werr != nil {
			return werr
		}
	}
}

func (s *Server) expectedUDPClientAddr(conn net.Conn, req request) *net.UDPAddr {
	tcpRemote, _ := conn.RemoteAddr().(*net.TCPAddr)
	if tcpRemote == nil {
		return nil
	}

	ip := append(net.IP(nil), tcpRemote.IP...)
	if parsed := net.ParseIP(req.DstHost); parsed != nil && !parsed.IsUnspecified() {
		ip = append(net.IP(nil), parsed...)
	}
	return &net.UDPAddr{IP: ip, Port: req.DstPort}
}

func isAssociatedUDPClient(src *net.UDPAddr, expected *net.UDPAddr) bool {
	if src == nil || expected == nil {
		return false
	}
	if expected.IP != nil && len(expected.IP) > 0 && !src.IP.Equal(expected.IP) {
		return false
	}
	if expected.Port != 0 && src.Port != expected.Port {
		return false
	}
	return true
}

func parseUDPAssociatePacket(data []byte) (udpPacket, error) {
	var packet udpPacket
	if len(data) < 4 {
		return packet, io.ErrUnexpectedEOF
	}
	if data[0] != 0x00 || data[1] != 0x00 {
		return packet, errors.New("invalid udp associate reserved bytes")
	}
	if data[2] != 0x00 {
		return packet, errUDPFragmentUnsupported
	}

	offset := 4
	switch data[3] {
	case 0x01:
		if len(data) < offset+4+2 {
			return packet, io.ErrUnexpectedEOF
		}
		packet.Host = net.IP(data[offset : offset+4]).String()
		offset += 4
	case 0x03:
		if len(data) < offset+1 {
			return packet, io.ErrUnexpectedEOF
		}
		size := int(data[offset])
		offset++
		if len(data) < offset+size+2 {
			return packet, io.ErrUnexpectedEOF
		}
		packet.Host = string(data[offset : offset+size])
		offset += size
	case 0x04:
		if len(data) < offset+16+2 {
			return packet, io.ErrUnexpectedEOF
		}
		packet.Host = net.IP(data[offset : offset+16]).String()
		offset += 16
	default:
		return packet, errors.New("unsupported udp associate address type")
	}

	packet.Port = int(binary.BigEndian.Uint16(data[offset : offset+2]))
	offset += 2
	packet.Payload = append([]byte(nil), data[offset:]...)
	return packet, nil
}

func buildUDPAssociatePacket(host string, port int, payload []byte) ([]byte, error) {
	packet := []byte{0x00, 0x00, 0x00}

	ip := net.ParseIP(host)
	if ip4 := ip.To4(); ip4 != nil {
		packet = append(packet, 0x01)
		packet = append(packet, ip4...)
	} else if ip16 := ip.To16(); ip16 != nil {
		packet = append(packet, 0x04)
		packet = append(packet, ip16...)
	} else {
		if len(host) > 255 {
			return nil, errors.New("domain name too long")
		}
		packet = append(packet, 0x03, byte(len(host)))
		packet = append(packet, []byte(host)...)
	}

	var portBuf [2]byte
	binary.BigEndian.PutUint16(portBuf[:], uint16(port))
	packet = append(packet, portBuf[:]...)
	packet = append(packet, payload...)
	return packet, nil
}
