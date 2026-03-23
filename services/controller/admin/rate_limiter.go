package admin

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// rateLimiter implements a per-IP sliding window rate limiter.
// It tracks request timestamps per IP and prunes expired entries.
type rateLimiter struct {
	mu          sync.Mutex
	window      time.Duration
	maxRequests int
	clients     map[string][]time.Time
}

// newRateLimiter creates a rate limiter with the given window and max requests.
// It starts a background goroutine that prunes stale entries every minute.
func newRateLimiter(window time.Duration, maxRequests int) *rateLimiter {
	rl := &rateLimiter{
		window:      window,
		maxRequests: maxRequests,
		clients:     make(map[string][]time.Time),
	}
	go rl.cleanup()
	return rl
}

// Allow returns true if the IP has not exceeded the rate limit.
func (rl *rateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.window)

	// Prune expired timestamps for this IP.
	timestamps := rl.clients[ip]
	valid := timestamps[:0]
	for _, t := range timestamps {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}

	if len(valid) >= rl.maxRequests {
		rl.clients[ip] = valid
		return false
	}

	rl.clients[ip] = append(valid, now)
	return true
}

// cleanup runs every minute to remove IPs with no recent requests.
func (rl *rateLimiter) cleanup() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		cutoff := now.Add(-rl.window)
		for ip, timestamps := range rl.clients {
			valid := timestamps[:0]
			for _, t := range timestamps {
				if t.After(cutoff) {
					valid = append(valid, t)
				}
			}
			if len(valid) == 0 {
				delete(rl.clients, ip)
			} else {
				rl.clients[ip] = valid
			}
		}
		rl.mu.Unlock()
	}
}

// clientIP extracts the client IP address from the request.
// It checks X-Forwarded-For first, then falls back to RemoteAddr.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// X-Forwarded-For can contain multiple IPs; use the first one.
		ip := strings.TrimSpace(strings.SplitN(xff, ",", 2)[0])
		if ip != "" {
			return ip
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// withRateLimit wraps an http.Handler with rate limiting.
// If the rate limit is exceeded, it responds with 429 Too Many Requests
// and a Retry-After header indicating how long to wait.
func withRateLimit(rl *rateLimiter, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)
		if !rl.Allow(ip) {
			retryAfter := int(rl.window.Seconds())
			w.Header().Set("Retry-After", fmt.Sprintf("%d", retryAfter))
			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}
