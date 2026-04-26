package socks5

import (
	"context"
	"errors"
	"fmt"

	"tg-ws-proxy/internal/config"
	"tg-ws-proxy/internal/telegram"
	"tg-ws-proxy/internal/wsbridge"
)

type wsDialAttemptResult struct {
	ws           *wsbridge.Client
	lastErr      error
	allRedirects bool
	sawRedirect  bool
}

func (s *Server) tryPooledWebsocket(dc int, isMedia bool, targetIP string, domains []string) (*wsbridge.Client, bool) {
	if s.pool == nil {
		return nil, false
	}
	s.pool.SetDialFunc(s.wsDialFunc)
	if ws, ok := s.pool.Get(dc, isMedia, targetIP, domains); ok {
		s.stats.incPoolHit()
		s.debugf("ws pool hit: dc=%d media=%v target=%s", dc, isMedia, targetIP)
		return ws, true
	}
	s.stats.incPoolMiss()
	return nil, false
}

func (s *Server) websocketDialConfig(dc int, isMedia bool, key routeKey) config.Config {
	dialCfg := s.cfg
	if dialCfg.DialTimeout <= 0 || dialCfg.DialTimeout > wsFailFastDial {
		dialCfg.DialTimeout = wsFailFastDial
		s.debugf("ws fail-fast timeout: dc=%d media=%v timeout=%s", dc, isMedia, dialCfg.DialTimeout)
	}
	if s.isCooldownActive(key) {
		s.debugf("ws cooldown active: dc=%d media=%v timeout=%s", dc, isMedia, dialCfg.DialTimeout)
	}
	return dialCfg
}

func (s *Server) dialWebsocketDomains(ctx context.Context, dialCfg config.Config, key routeKey, targetIP string, dc int, isMedia bool, domains []string) wsDialAttemptResult {
	result := wsDialAttemptResult{allRedirects: true}
	for _, domain := range domains {
		s.debugf("ws dial attempt: dc=%d media=%v target=%s domain=%s", dc, isMedia, targetIP, domain)
		ws, err := s.wsDialFunc(ctx, dialCfg, targetIP, domain)
		if err == nil {
			s.clearWSFailure(key)
			s.debugf("ws dial success: dc=%d media=%v target=%s domain=%s", dc, isMedia, targetIP, domain)
			result.ws = ws
			return result
		}
		s.debugf("ws dial failed: dc=%d media=%v target=%s domain=%s err=%v", dc, isMedia, targetIP, domain, err)
		s.stats.incWSErrors()
		var hErr *wsbridge.HandshakeError
		if errors.As(err, &hErr) && hErr.IsRedirect() {
			result.sawRedirect = true
		} else {
			result.allRedirects = false
		}
		result.lastErr = err
	}
	return result
}

func websocketDomainsForDC(dc int, isMedia bool) []string {
	return telegram.WSDomains(telegram.NormalizeDC(dc), isMedia)
}

func (s *Server) finalizeWebsocketDialFailure(key routeKey, result wsDialAttemptResult) error {
	if result.sawRedirect && result.allRedirects {
		s.markBlacklisted(key)
		return fmt.Errorf("all websocket routes redirected: %w", errWSBlacklisted)
	}
	s.markWSFailure(key)
	return result.lastErr
}
