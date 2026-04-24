package socks5

import (
	"context"
	"errors"
	"io"
	"net"

	"tg-ws-proxy/internal/mtproto"
	"tg-ws-proxy/internal/wsbridge"
)

type initReadResult struct {
	init            []byte
	routeByInitOnly bool
	handled         bool
}

func (s *Server) readAndClassifyInit(ctx context.Context, conn net.Conn, req request, clientAddr string, isTelegramCandidate bool) (initReadResult, bool) {
	init := make([]byte, 64)
	n, err := readWithContext(ctx, conn, init, s.cfg.InitTimeout)
	if err != nil {
		if !isTelegramCandidate {
			s.runProbeReadPassthrough(ctx, conn, req.DstHost, req.DstPort, init, n, err, clientAddr)
			return initReadResult{}, true
		}
		s.stats.recordError("mtproto_init", err)
		s.recordVerboseConnFailure(clientAddr, "mtproto_init", err)
		return initReadResult{}, true
	}

	result := initReadResult{init: init}
	if !isTelegramCandidate {
		inferred, info, parseErr := inferTelegramCandidateFromInit(init)
		if parseErr != nil {
			s.runPassthroughWithInit(ctx, conn, req.DstHost, req.DstPort, init, clientAddr, "mtproto-probe-miss")
			return initReadResult{}, true
		}
		if inferred {
			result.routeByInitOnly = true
			s.debugf("[%s] telegram route inferred from mtproto init on destination %s:%d dc=%d media=%v", clientAddr, req.DstHost, req.DstPort, info.DC, info.IsMedia)
		}
	}

	if mtproto.IsHTTPTransport(result.init) {
		if result.routeByInitOnly {
			s.runPassthroughWithInit(ctx, conn, req.DstHost, req.DstPort, result.init, clientAddr, "http-probe")
			return initReadResult{handled: true}, true
		}
		s.runTCPFallbackWithInit(ctx, conn, req.DstHost, req.DstPort, result.init, 0, false, clientAddr, func() {
			s.debugf("[%s] route=tcp-fallback reason=http-transport destination=%s:%d", clientAddr, req.DstHost, req.DstPort)
		})
		return initReadResult{handled: true}, true
	}

	return result, false
}

func (s *Server) bridgeWebsocketRoute(ctx context.Context, conn net.Conn, ws *wsbridge.Client, plan telegramRoutePlan, init []byte, clientAddr string) {
	defer ws.Close()
	s.stats.incWSConnections()
	s.stats.recordWSRoute(plan.effectiveDC, plan.isMedia)
	s.debugf("[%s] route=websocket dc=%d effective_dc=%d ws_dc=%d media=%v target=%s", clientAddr, plan.dc, plan.effectiveDC, plan.wsDomainDC, plan.isMedia, plan.targetIP)

	var splitter *mtproto.Splitter
	if plan.proto != 0 && (plan.initPatched || plan.isMedia || plan.proto != mtproto.ProtoIntermediate) {
		splitter, _ = mtproto.NewSplitter(init, plan.proto)
		if splitter != nil {
			s.debugf("[%s] websocket splitter enabled for proto=0x%08x", clientAddr, plan.proto)
		}
	}

	if err := wsbridge.Bridge(ctx, conn, ws, init, splitter); err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, context.Canceled) {
		s.stats.recordError("ws_bridge", err)
		s.recordVerboseConnFailure(clientAddr, "ws_bridge", err)
		return
	}
	s.debugf("[%s] connection finished", clientAddr)
}
