package server

import (
	"context"
	"net/http"
)

type contextKey string

const (
	contextKeyToken    contextKey = "token"
	contextKeyProvider contextKey = "provider"
	contextKeySession  contextKey = "session"
)

// AuthMiddleware extracts the session from the cookie and injects the token
// and provider name into the request context. Returns 401 if no valid session.
func AuthMiddleware(store *SessionStore, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookieName)
		if err != nil {
			http.Error(w, `{"error":"unauthenticated"}`, http.StatusUnauthorized)
			return
		}
		sess := store.Get(cookie.Value)
		if sess == nil {
			http.Error(w, `{"error":"unauthenticated"}`, http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), contextKeyToken, sess.Token)
		ctx = context.WithValue(ctx, contextKeyProvider, sess.Provider)
		ctx = context.WithValue(ctx, contextKeySession, sess)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// tokenFromContext returns the OAuth token injected by AuthMiddleware.
func tokenFromContext(ctx context.Context) string {
	v, _ := ctx.Value(contextKeyToken).(string)
	return v
}

// providerFromContext returns the provider name injected by AuthMiddleware.
func providerFromContext(ctx context.Context) string {
	v, _ := ctx.Value(contextKeyProvider).(string)
	return v
}
