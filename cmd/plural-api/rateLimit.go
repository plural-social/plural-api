package main

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/plural-social/plural-api/internal/account"
)

const (
	limiterCleanupThreshold = 10_000
	limiterIdleTimeout      = 10 * time.Minute
)

// ipRateLimiter keeps one token bucket per client IP.
// ponytail: in-memory and per process. Move to Postgres or Redis when running
// more than one API process.
type ipRateLimiter struct {
	mu       sync.Mutex
	limiters map[string]*ipLimiterEntry
	every    time.Duration
	burst    int
	now      func() time.Time
}

type ipLimiterEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

func newIPRateLimiter(every time.Duration, burst int) *ipRateLimiter {
	return &ipRateLimiter{
		limiters: map[string]*ipLimiterEntry{},
		every:    every,
		burst:    burst,
		now:      time.Now,
	}
}

// allow reports whether a request from ip may proceed. When the map holds more
// than 10,000 entries, it first deletes entries not seen for 10 minutes. No
// background goroutine.
func (l *ipRateLimiter) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	if len(l.limiters) > limiterCleanupThreshold {
		for key, entry := range l.limiters {
			if now.Sub(entry.lastSeen) > limiterIdleTimeout {
				delete(l.limiters, key)
			}
		}
	}

	entry, ok := l.limiters[ip]
	if !ok {
		entry = &ipLimiterEntry{limiter: rate.NewLimiter(rate.Every(l.every), l.burst)}
		l.limiters[ip] = entry
	}
	entry.lastSeen = now
	return entry.limiter.AllowN(now, 1)
}

// rateLimited returns 429 when the limiter denies the request. A nil limiter
// allows everything, so handler tests do not need one.
func (a *app) rateLimited(l *ipRateLimiter, h handler) handler {
	return func(r *http.Request) (int, any) {
		if l != nil && !l.allow(a.clientIP(r)) {
			return http.StatusTooManyRequests, ErrorResponse{
				Message: "too many requests, try again later",
				Type:    account.ErrTypeRateLimited,
			}
		}
		return h(r)
	}
}

// clientIP returns the host part of r.RemoteAddr. When a.trustProxyHeaders is
// true and X-Forwarded-For is set, it returns the first address in that header.
func (a *app) clientIP(r *http.Request) string {
	if a.trustProxyHeaders {
		if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
			first, _, _ := strings.Cut(forwarded, ",")
			return strings.TrimSpace(first)
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
