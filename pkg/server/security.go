package server

import (
	"net/http"
	"sync"
	"time"
)

// CSRFMiddleware enforces CSRF protection on mutating requests (POST, PUT, PATCH, DELETE).
// It uses two layers:
//  1. SameSite=Lax cookies — already set; prevents most CSRF.
//  2. Custom header check: the browser's fetch API sends credentials only for same-origin
//     requests. We require a custom header (X-Requested-With: XMLHttpRequest) on all
//     state-changing API calls to distinguish fetch() from form submissions.
func CSRFMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost || r.Method == http.MethodPut ||
			r.Method == http.MethodPatch || r.Method == http.MethodDelete {
			// Only enforce on /api/* routes (OAuth callbacks are GET).
			if len(r.URL.Path) >= 4 && r.URL.Path[:4] == "/api" {
				if r.Header.Get("X-Requested-With") != "XMLHttpRequest" {
					http.Error(w, `{"error":"CSRF check failed"}`, http.StatusForbidden)
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

// rateLimiter tracks request counts per IP using a simple token-bucket approximation.
type rateLimiter struct {
	mu       sync.Mutex
	buckets  map[string]*bucket
	max      int           // max requests per window
	window   time.Duration // time window
}

type bucket struct {
	count     int
	windowEnd time.Time
}

func newRateLimiter(max int, window time.Duration) *rateLimiter {
	rl := &rateLimiter{
		buckets: make(map[string]*bucket),
		max:     max,
		window:  window,
	}
	go rl.sweep()
	return rl
}

func (rl *rateLimiter) allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := time.Now()
	b, ok := rl.buckets[key]
	if !ok || now.After(b.windowEnd) {
		rl.buckets[key] = &bucket{count: 1, windowEnd: now.Add(rl.window)}
		return true
	}
	if b.count >= rl.max {
		return false
	}
	b.count++
	return true
}

func (rl *rateLimiter) sweep() {
	ticker := time.NewTicker(rl.window)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now()
		rl.mu.Lock()
		for k, b := range rl.buckets {
			if now.After(b.windowEnd) {
				delete(rl.buckets, k)
			}
		}
		rl.mu.Unlock()
	}
}

// oauthRateLimiter is a shared rate limiter for OAuth endpoints.
// 20 requests per IP per minute is generous enough for legitimate users.
var oauthRateLimiter = newRateLimiter(20, time.Minute)

// RateLimitMiddleware applies per-IP rate limiting to OAuth login/callback endpoints.
func RateLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)
		if !oauthRateLimiter.allow(ip) {
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// clientIP extracts the remote IP address (without port).
func clientIP(r *http.Request) string {
	// Trust X-Forwarded-For only if running behind a known proxy; for now use RemoteAddr.
	addr := r.RemoteAddr
	for i := len(addr) - 1; i >= 0; i-- {
		if addr[i] == ':' {
			return addr[:i]
		}
	}
	return addr
}
