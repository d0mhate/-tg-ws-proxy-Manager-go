package mtpserver

import "tg-ws-proxy/internal/telegram"

type directRouteCandidate struct {
	targetDC   int
	wsDomainDC int
	targetIP   string
}

func (s *MTServer) directRouteCandidates(dc int) []directRouteCandidate {
	wsDomainDC := s.wsDomainDC(dc)

	var routes []directRouteCandidate
	appendRoute := func(targetDC int) {
		if targetDC == 0 {
			return
		}

		targetIP := s.cfg.DCIPs[targetDC]
		if targetIP == "" {
			return
		}

		candidate := directRouteCandidate{
			targetDC:   targetDC,
			wsDomainDC: wsDomainDC,
			targetIP:   targetIP,
		}
		for _, existing := range routes {
			if existing.wsDomainDC == candidate.wsDomainDC && existing.targetIP == candidate.targetIP {
				return
			}
		}
		routes = append(routes, candidate)
	}

	effectiveDC := s.effectiveDC(dc)
	appendRoute(effectiveDC)
	if normalizedDC := telegram.NormalizeDC(dc); normalizedDC != effectiveDC {
		appendRoute(normalizedDC)
	}

	return routes
}

func (s *MTServer) orderedDirectRoutes(dc int, isMedia bool, routes []directRouteCandidate) []directRouteCandidate {
	if len(routes) < 2 || s.routeCooldowns == nil {
		return routes
	}

	available := make([]directRouteCandidate, 0, len(routes))
	for _, route := range routes {
		key := routeCooldownKey{requestDC: dc, targetDC: route.targetDC, isMedia: isMedia}
		if s.routeCooldowns.active(key) {
			if s.cfg.Verbose {
				s.agg.Printf("mtproto: route cooldown active dc=%d target-dc=%d via %s", dc, route.targetDC, route.targetIP)
			}
			continue
		}
		available = append(available, route)
	}

	if len(available) > 0 {
		return available
	}

	return []directRouteCandidate{s.preferredFallbackRoute(routes)}
}

func (s *MTServer) preferredFallbackRoute(routes []directRouteCandidate) directRouteCandidate {
	if len(routes) == 0 {
		return directRouteCandidate{}
	}

	for _, route := range routes {
		if route.targetDC == route.wsDomainDC {
			return route
		}
	}

	return routes[0]
}

func (s *MTServer) hasAlternativeDirectRoute(dc int) bool {
	return len(s.directRouteCandidates(dc)) > 1
}

func (s *MTServer) shouldCooldownDirectRouteFailure(dc int, route directRouteCandidate) bool {
	if s.routeCooldowns == nil || route.targetIP == "" || !s.hasAlternativeDirectRoute(dc) {
		return false
	}

	// Keep the dc203 -> dc2 fallback hot even after transient timeouts.
	// This matches the practical behavior seen in upstream variants where the
	// normalized dc2 route is often the only path that still carries media.
	if dc == 203 && route.targetDC == telegram.NormalizeDC(dc) {
		return false
	}

	return true
}

func (s *MTServer) markDirectRouteFailure(dc int, isMedia bool, route directRouteCandidate) {
	if !s.shouldCooldownDirectRouteFailure(dc, route) {
		return
	}

	s.routeCooldowns.markFailure(routeCooldownKey{
		requestDC: dc,
		targetDC:  route.targetDC,
		isMedia:   isMedia,
	})
}

func (s *MTServer) markDirectRouteBridgeFailure(dc int, isMedia bool, route directRouteCandidate) {
	if s.routeCooldowns == nil || route.targetIP == "" || !s.hasAlternativeDirectRoute(dc) {
		return
	}

	s.routeCooldowns.markBridgeFailure(routeCooldownKey{
		requestDC: dc,
		targetDC:  route.targetDC,
		isMedia:   isMedia,
	})
}

func (s *MTServer) effectiveDC(dc int) int {
	if dc == 0 {
		return 0
	}
	if _, ok := s.cfg.DCIPs[dc]; ok {
		return dc
	}
	return telegram.NormalizeDC(dc)
}

func (s *MTServer) wsDomainDC(dc int) int {
	if dc == 0 {
		return 0
	}
	return telegram.NormalizeDC(dc)
}

func cfWSHost(domain string, dc int) string {
	return telegram.CFWSDomain(domain, dc)
}
