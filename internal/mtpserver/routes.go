package mtpserver

import "tg-ws-proxy/internal/telegram"

type directRouteCandidate struct {
	targetDC   int
	wsDomainDC int
	targetIP   string
}

func (s *MTServer) directRouteCandidates(dc int) []directRouteCandidate {
	effectiveDC := s.effectiveDC(dc)
	wsDomainDC := s.wsDomainDC(dc)
	targetIP := s.cfg.DCIPs[effectiveDC]
	if targetIP == "" {
		return nil
	}
	return []directRouteCandidate{{
		targetDC:   effectiveDC,
		wsDomainDC: wsDomainDC,
		targetIP:   targetIP,
	}}
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

func (s *MTServer) markDirectRouteFailure(dc int, isMedia bool, route directRouteCandidate) {
	if s.routeCooldowns == nil || route.targetIP == "" || !s.hasAlternativeDirectRoute(dc) {
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
