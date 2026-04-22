package mtpserver

import (
	"context"
	"errors"
	"fmt"
	"time"

	"tg-ws-proxy/internal/telegram"
	"tg-ws-proxy/internal/wsbridge"
)

const wsFailFastDial = 2 * time.Second

func (s *MTServer) dialDirectWS(
	ctx context.Context,
	dc int,
	isMedia bool,
	route directRouteCandidate,
) (*wsbridge.Client, bool, error) {
	if route.targetIP == "" {
		return nil, false, fmt.Errorf("no target IP configured for dc=%d", dc)
	}

	key := routeCooldownKey{requestDC: dc, targetDC: route.targetDC, isMedia: isMedia}
	domains := telegram.WSDomains(route.wsDomainDC, isMedia)
	dialCfg := s.cfg
	routePolicyEnabled := s.hasAlternativeDirectRoute(dc)

	routeInCooldown := false
	if routePolicyEnabled && s.routeCooldowns != nil {
		routeInCooldown = s.routeCooldowns.active(key)
	}
	if routeInCooldown && s.cfg.Verbose {
		s.agg.Printf("mtproto: route cooldown active dc=%d target-dc=%d via %s", dc, route.targetDC, route.targetIP)
	}
	if routePolicyEnabled {
		if timeout := s.routeCooldowns.timeoutFor(key, dialCfg.DialTimeout, wsFailFastDial); timeout > 0 && timeout != dialCfg.DialTimeout {
			dialCfg.DialTimeout = timeout
			if s.cfg.Verbose {
				s.agg.Printf("mtproto: route fail-fast timeout dc=%d target-dc=%d via %s timeout=%s", dc, route.targetDC, route.targetIP, dialCfg.DialTimeout)
			}
		}
	}

	if s.pool != nil && !routeInCooldown {
		s.pool.SetDialFunc(s.wsDialFunc)
		if ws, ok := s.pool.Get(route.wsDomainDC, isMedia, route.targetIP, domains); ok {
			s.stats.recordPoolHit(dc)
			if s.cfg.Verbose {
				s.agg.Printf("mtproto: pool hit dc=%d via %s", dc, route.targetIP)
			}
			return ws, false, nil
		}
		s.stats.recordPoolMiss(dc)
		if s.cfg.Verbose {
			s.agg.Printf("mtproto: pool miss dc=%d via %s", dc, route.targetIP)
		}
	}

	var lastErr error
	allRedirects := true
	sawRedirect := false
	for _, domain := range domains {
		if s.cfg.Verbose {
			s.agg.Printf("mtproto: direct dial dc=%d → %s via %s", dc, domain, route.targetIP)
		}
		ws, err := s.wsDialFunc(ctx, dialCfg, route.targetIP, domain)
		if err == nil {
			s.stats.recordDirectConnected(dc)
			if s.cfg.Verbose {
				s.agg.Printf("mtproto: direct connected dc=%d → %s", dc, route.targetIP)
			}
			return ws, false, nil
		}

		s.stats.recordDirectDialFailed(dc)
		if s.cfg.Verbose {
			s.agg.Printf("mtproto: direct dial failed dc=%d → %s: %v", dc, route.targetIP, err)
		}

		var hErr *wsbridge.HandshakeError
		if errors.As(err, &hErr) && hErr.IsRedirect() {
			sawRedirect = true
		} else {
			allRedirects = false
		}
		lastErr = err
	}

	return nil, sawRedirect && allRedirects, lastErr
}

func (s *MTServer) dialDirectWSWithFallback(
	ctx context.Context,
	dc int,
	isMedia bool,
	routes []directRouteCandidate,
) (*wsbridge.Client, directRouteCandidate, error) {
	if len(routes) == 0 {
		return nil, directRouteCandidate{}, fmt.Errorf("no direct route configured for dc=%d", dc)
	}

	orderedRoutes := s.orderedDirectRoutes(dc, isMedia, routes)
	var lastErr error

	for _, route := range orderedRoutes {
		ws, allRedirects, err := s.dialDirectWS(ctx, dc, isMedia, route)
		if err == nil {
			s.routeCooldowns.clear(routeCooldownKey{requestDC: dc, targetDC: route.targetDC, isMedia: isMedia})
			return ws, route, nil
		}

		if s.hasAlternativeDirectRoute(dc) {
			key := routeCooldownKey{requestDC: dc, targetDC: route.targetDC, isMedia: isMedia}
			if allRedirects {
				s.routeCooldowns.markRedirect(key)
			} else {
				s.routeCooldowns.markFailure(key)
			}
		}
		lastErr = err
	}

	return nil, directRouteCandidate{}, lastErr
}
