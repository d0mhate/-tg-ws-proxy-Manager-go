package socks5

import (
	"errors"
	"net"

	"tg-ws-proxy/internal/mtproto"
	"tg-ws-proxy/internal/telegram"
)

type telegramRoutePlan struct {
	init                    []byte
	dc                      int
	effectiveDC             int
	wsDomainDC              int
	isMedia                 bool
	proto                   uint32
	targetIP                string
	fallbackHost            string
	initPatched             bool
	inferredFromDestination bool
	routeByInitOnly         bool
}

func classifyInitialRoute(req request) (isTelegramCandidate bool, shouldProbeMTProto bool, isIPv6 bool) {
	dstIP := net.ParseIP(req.DstHost)
	isIPv6 = dstIP != nil && dstIP.To4() == nil
	isTelegramCandidate = telegram.IsTelegramIP(req.DstHost) || isLikelyTelegramIPv6(req, isIPv6)
	shouldProbeMTProto = !isTelegramCandidate && shouldProbeTelegramByPort(req)
	return isTelegramCandidate, shouldProbeMTProto, isIPv6
}

func inferTelegramCandidateFromInit(req request, init []byte) (bool, bool, mtproto.InitInfo, error) {
	info, err := mtproto.ParseInit(init)
	if err != nil {
		return false, false, mtproto.InitInfo{}, err
	}
	return true, true, info, nil
}

func (s *Server) buildTelegramRoutePlan(req request, init []byte, isIPv6 bool, routeByInitOnly bool, clientAddr string) telegramRoutePlan {
	info, err := mtproto.ParseInit(init)
	if err != nil && !errors.Is(err, mtproto.ErrInvalidProto) {
		s.recordVerboseConnFailure(clientAddr, "mtproto_parse", err)
	}

	plan := telegramRoutePlan{
		init:            init,
		dc:              info.DC,
		isMedia:         info.IsMedia,
		proto:           info.Proto,
		routeByInitOnly: routeByInitOnly,
	}
	s.debugf("[%s] mtproto init parsed: dc=%d media=%v proto=0x%08x", clientAddr, plan.dc, plan.isMedia, plan.proto)

	if plan.dc == 0 {
		if endpoint, ok := telegram.LookupEndpoint(req.DstHost); ok {
			plan.dc = endpoint.DC
			plan.isMedia = endpoint.IsMedia
			plan.inferredFromDestination = true
			s.debugf("[%s] dc inferred from destination ip: dc=%d media=%v", clientAddr, plan.dc, plan.isMedia)
			if _, ok := s.cfg.DCIPs[plan.dc]; ok {
				patched, patchErr := mtproto.PatchInitDC(plan.init, choosePatchedDC(plan.dc, plan.isMedia))
				if patchErr == nil {
					plan.init = patched
					plan.initPatched = true
					s.debugf("[%s] patched mtproto init with dc=%d", clientAddr, choosePatchedDC(plan.dc, plan.isMedia))
				}
			}
		}
	}

	plan.effectiveDC = s.effectiveDC(plan.dc)
	if plan.effectiveDC != 0 && plan.effectiveDC != plan.dc {
		patched, patchErr := mtproto.PatchInitDC(plan.init, choosePatchedDC(plan.effectiveDC, plan.isMedia))
		if patchErr == nil {
			plan.init = patched
			plan.initPatched = true
			s.debugf("[%s] normalized dc=%d -> %d and patched mtproto init", clientAddr, plan.dc, plan.effectiveDC)
		}
	}

	plan.targetIP = s.cfg.DCIPs[plan.effectiveDC]
	plan.fallbackHost = req.DstHost
	if isIPv6 || plan.effectiveDC != plan.dc || plan.routeByInitOnly || plan.inferredFromDestination {
		plan.fallbackHost = plan.targetIP
		if plan.targetIP != "" {
			s.debugf("[%s] telegram route will fallback via dc target %s", clientAddr, plan.targetIP)
		}
	}

	plan.wsDomainDC = s.wsDomainDC(plan.effectiveDC)
	return plan
}
