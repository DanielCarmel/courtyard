package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthMiddleware(t *testing.T) {
	store := NewSessionStore()

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tok := tokenFromContext(r.Context())
		prov := providerFromContext(r.Context())
		w.Header().Set("X-Token", tok)
		w.Header().Set("X-Provider", prov)
		w.WriteHeader(http.StatusOK)
	})
	handler := AuthMiddleware(store, next)

	t.Run("no cookie returns 401", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/me", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("got %d, want 401", rr.Code)
		}
	})

	t.Run("invalid session ID returns 401", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/me", nil)
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "bogus-id"})
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("got %d, want 401", rr.Code)
		}
	})

	t.Run("valid session injects token and provider", func(t *testing.T) {
		sess, err := store.Create("mytoken", "github")
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		req := httptest.NewRequest("GET", "/api/me", nil)
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sess.ID})
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("got %d, want 200", rr.Code)
		}
		if got := rr.Header().Get("X-Token"); got != "mytoken" {
			t.Errorf("X-Token: got %q, want %q", got, "mytoken")
		}
		if got := rr.Header().Get("X-Provider"); got != "github" {
			t.Errorf("X-Provider: got %q, want %q", got, "github")
		}
	})
}
