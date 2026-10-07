package controller

import (
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/example/control-plane/internal/models"
)

type rateLimitWindow struct {
	started time.Time
	count   int
}

// rateLimit limits repeated requests from one client for a single sensitive action.
func (a *App) rateLimit(scope string, limit int, window time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := scope + ":" + clientIP(r)
			if user, ok := r.Context().Value(userKey).(models.User); ok {
				key += ":" + user.ID.Hex()
			}
			allowed, retryAfter := a.allowRateLimitedRequest(key, limit, window, time.Now())
			if !allowed {
				seconds := int(math.Ceil(retryAfter.Seconds()))
				if seconds < 1 {
					seconds = 1
				}
				w.Header().Set("Retry-After", strconv.Itoa(seconds))
				writeJSON(w, http.StatusTooManyRequests, map[string]string{
					"error": "too many attempts; try again later",
				})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// allowRateLimitedRequest applies a fixed window and periodically clears expired clients.
func (a *App) allowRateLimitedRequest(key string, limit int, duration time.Duration, now time.Time) (bool, time.Duration) {
	a.rateLimitMu.Lock()
	defer a.rateLimitMu.Unlock()
	if a.rateLimits == nil {
		a.rateLimits = make(map[string]rateLimitWindow)
	}
	entry, exists := a.rateLimits[key]
	if !exists || now.Sub(entry.started) >= duration {
		a.rateLimits[key] = rateLimitWindow{started: now, count: 1}
		if len(a.rateLimits)%256 == 0 {
			for client, window := range a.rateLimits {
				if now.Sub(window.started) >= duration {
					delete(a.rateLimits, client)
				}
			}
		}
		return true, 0
	}
	if entry.count >= limit {
		return false, duration - now.Sub(entry.started)
	}
	entry.count++
	a.rateLimits[key] = entry
	return true, 0
}

// clientIP uses the peer address, trusting forwarded addresses only from localhost proxies.
func clientIP(r *http.Request) string {
	remoteHost, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		remoteHost = r.RemoteAddr
	}
	remoteIP := net.ParseIP(remoteHost)
	if remoteIP != nil && remoteIP.IsLoopback() {
		if forwarded := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-For"), ",")[0]); net.ParseIP(forwarded) != nil {
			return forwarded
		}
	}
	if remoteIP != nil {
		return remoteIP.String()
	}
	return remoteHost
}
