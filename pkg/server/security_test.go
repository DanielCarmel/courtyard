package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCSRFMiddleware(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := CSRFMiddleware(next)

	tests := []struct {
		name       string
		method     string
		path       string
		header     string
		wantStatus int
	}{
		{
			name:       "GET passes without header",
			method:     "GET",
			path:       "/api/repos",
			header:     "",
			wantStatus: http.StatusOK,
		},
		{
			name:       "POST /api without header is rejected",
			method:     "POST",
			path:       "/api/repos/o/r/forms/f/submit",
			header:     "",
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "POST /api with header is allowed",
			method:     "POST",
			path:       "/api/repos/o/r/forms/f/submit",
			header:     "XMLHttpRequest",
			wantStatus: http.StatusOK,
		},
		{
			name:       "POST non-api path passes without header",
			method:     "POST",
			path:       "/auth/github/callback",
			header:     "",
			wantStatus: http.StatusOK,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader("{}"))
			if tc.header != "" {
				req.Header.Set("X-Requested-With", tc.header)
			}
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)
			if rr.Code != tc.wantStatus {
				t.Errorf("got %d, want %d", rr.Code, tc.wantStatus)
			}
		})
	}
}

func TestRateLimitMiddleware(t *testing.T) {
	// Use a very low limit for testing.
	rl := newRateLimiter(3, 1000*1000*1000 /* 1s, effectively */)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)
		if !rl.allow(ip) {
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("GET", "/auth/github/login", nil)
		req.RemoteAddr = "1.2.3.4:1234"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("request %d: got %d, want 200", i+1, rr.Code)
		}
	}

	// 4th request should be rate limited.
	req := httptest.NewRequest("GET", "/auth/github/login", nil)
	req.RemoteAddr = "1.2.3.4:1234"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429 on rate-limited request, got %d", rr.Code)
	}
}
