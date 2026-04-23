package socks5

import (
	"context"
	"net"

	"tg-ws-proxy/internal/wsbridge"
)

func (s *Server) tryTelegramWebsocketRoute(
	ctx context.Context,
	conn net.Conn,
	req request,
	plan telegramRoutePlan,
	init []byte,
	clientAddr string,
) (*wsbridge.Client, bool) {
	if plan.targetIP == "" {
		s.runTCPFallbackWithInit(ctx, conn, req.DstHost, req.DstPort, init, plan.effectiveDC, plan.isMedia, clientAddr, func() {
			s.debugf("[%s] route=tcp-fallback reason=no-dc-override dc=%d effective_dc=%d destination=%s:%d", clientAddr, plan.dc, plan.effectiveDC, req.DstHost, req.DstPort)
		})
		return nil, true
	}

	if !isWSEnabledDC(plan.wsDomainDC) {
		s.runTCPFallbackWithInit(ctx, conn, plan.fallbackHost, req.DstPort, init, plan.effectiveDC, plan.isMedia, clientAddr, func() {
			s.debugf("[%s] route=tcp-fallback reason=ws-disabled-dc dc=%d effective_dc=%d ws_dc=%d target=%s", clientAddr, plan.dc, plan.effectiveDC, plan.wsDomainDC, plan.targetIP)
		})
		return nil, true
	}

	ws, err := s.connectTelegramThenCloudflareWS(ctx, clientAddr, plan.dc, plan.effectiveDC, plan.isMedia, plan.targetIP)
	if err != nil {
		s.runTCPFallbackWithInit(ctx, conn, plan.fallbackHost, req.DstPort, init, plan.effectiveDC, plan.isMedia, clientAddr, func() {
			s.debugf("[%s] route=tcp-fallback reason=%s dc=%d effective_dc=%d target=%s", clientAddr, fallbackReason(err), plan.dc, plan.effectiveDC, plan.targetIP)
		})
		return nil, true
	}

	return ws, false
}
