package socks5

import (
	"context"
	"net"

	"tg-ws-proxy/internal/config"
	"tg-ws-proxy/internal/wsbridge"
)

type telegramWSRouteAction uint8

const (
	telegramWSRouteConnect telegramWSRouteAction = iota
	telegramWSRouteTCPFallbackNoOverride
	telegramWSRouteTCPFallbackWSDisabled
)

type telegramWSRouteDecision struct {
	action            telegramWSRouteAction
	allowCloudflareWS bool
	allowTelegramWS   bool
	fallbackHost      string
}

func decideTelegramWSRoute(cfg config.Config, plan telegramRoutePlan) telegramWSRouteDecision {
	allowCloudflareWS := cfg.UseCFProxy && len(cfg.CFDomains) > 0 && plan.wsDomainDC != 0
	allowTelegramWS := plan.targetIP != "" && isWSEnabledDC(plan.wsDomainDC)

	switch {
	case !allowTelegramWS && !allowCloudflareWS && plan.targetIP == "":
		return telegramWSRouteDecision{
			action:            telegramWSRouteTCPFallbackNoOverride,
			allowCloudflareWS: allowCloudflareWS,
			allowTelegramWS:   allowTelegramWS,
			fallbackHost:      "",
		}
	case !allowTelegramWS && !allowCloudflareWS:
		return telegramWSRouteDecision{
			action:            telegramWSRouteTCPFallbackWSDisabled,
			allowCloudflareWS: allowCloudflareWS,
			allowTelegramWS:   allowTelegramWS,
			fallbackHost:      plan.fallbackHost,
		}
	default:
		return telegramWSRouteDecision{
			action:            telegramWSRouteConnect,
			allowCloudflareWS: allowCloudflareWS,
			allowTelegramWS:   allowTelegramWS,
			fallbackHost:      plan.fallbackHost,
		}
	}
}

func (s *Server) tryTelegramWebsocketRoute(
	ctx context.Context,
	conn net.Conn,
	req request,
	plan telegramRoutePlan,
	init []byte,
	clientAddr string,
) (*wsbridge.Client, bool) {
	decision := decideTelegramWSRoute(s.cfg, plan)

	switch decision.action {
	case telegramWSRouteTCPFallbackNoOverride:
		s.runTCPFallbackWithInit(ctx, conn, req.DstHost, req.DstPort, init, plan.effectiveDC, plan.isMedia, clientAddr, func() {
			s.debugf("[%s] route=tcp-fallback reason=no-dc-override dc=%d effective_dc=%d destination=%s:%d", clientAddr, plan.dc, plan.effectiveDC, req.DstHost, req.DstPort)
		})
		return nil, true
	case telegramWSRouteTCPFallbackWSDisabled:
		s.runTCPFallbackWithInit(ctx, conn, decision.fallbackHost, req.DstPort, init, plan.effectiveDC, plan.isMedia, clientAddr, func() {
			s.debugf("[%s] route=tcp-fallback reason=ws-disabled-dc dc=%d effective_dc=%d ws_dc=%d target=%s", clientAddr, plan.dc, plan.effectiveDC, plan.wsDomainDC, plan.targetIP)
		})
		return nil, true
	}

	ws, err := s.connectTelegramThenCloudflareWS(ctx, clientAddr, plan.dc, plan.effectiveDC, plan.isMedia, plan.targetIP, decision.allowTelegramWS)
	if err != nil {
		s.runTCPFallbackWithInit(ctx, conn, decision.fallbackHost, req.DstPort, init, plan.effectiveDC, plan.isMedia, clientAddr, func() {
			s.debugf("[%s] route=tcp-fallback reason=%s dc=%d effective_dc=%d target=%s", clientAddr, fallbackReason(err), plan.dc, plan.effectiveDC, plan.targetIP)
		})
		return nil, true
	}

	return ws, false
}
