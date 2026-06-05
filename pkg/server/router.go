package server

import (
	"io/fs"
	"net/http"
)

// NewRouter builds and returns the HTTP mux for the entire application.
// uiFS is the embedded filesystem containing the UI; pass nil to skip UI serving.
func NewRouter(handlers *Handlers, oauth *OAuthConfig, sessions *SessionStore, uiFS fs.FS) *http.ServeMux {
	mux := http.NewServeMux()

	// Health check — no auth required.
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	// OAuth routes — rate-limited, no auth required.
	rateLimited := func(h http.HandlerFunc) http.Handler {
		return RateLimitMiddleware(http.HandlerFunc(h))
	}
	mux.Handle("GET /auth/github/login", rateLimited(oauth.HandleGitHubLogin))
	mux.Handle("GET /auth/github/callback", rateLimited(oauth.HandleGitHubCallback))
	mux.Handle("GET /auth/bitbucket/login", rateLimited(oauth.HandleBitbucketLogin))
	mux.Handle("GET /auth/bitbucket/callback", rateLimited(oauth.HandleBitbucketCallback))
	mux.HandleFunc("GET /auth/logout", oauth.HandleLogout)

	// Authenticated + CSRF-protected API routes.
	api := func(h http.HandlerFunc) http.Handler {
		return CSRFMiddleware(AuthMiddleware(sessions, h))
	}

	mux.Handle("GET /api/me", api(handlers.HandleMe))
	mux.Handle("GET /api/repos", api(handlers.HandleListRepos))
	mux.Handle("GET /api/repos/{owner}/{repo}/forms", api(handlers.HandleListForms))
	mux.Handle("GET /api/repos/{owner}/{repo}/forms/{form}", api(handlers.HandleGetForm))
	mux.Handle("GET /api/repos/{owner}/{repo}/tree", api(handlers.HandleListTree))
	mux.Handle("POST /api/repos/{owner}/{repo}/forms/{form}/preview", api(handlers.HandlePreview))
	mux.Handle("POST /api/repos/{owner}/{repo}/forms/{form}/submit", api(handlers.HandleSubmit))

	// Studio: inline form builder (no .courtyard/ config in repo required).
	mux.Handle("POST /api/studio/preview", api(handlers.HandleStudioPreview))
	mux.Handle("POST /api/studio/commit", api(handlers.HandleStudioCommit))
	mux.Handle("POST /api/studio/download", api(handlers.HandleStudioDownload))

	// Serve embedded UI for all other paths.
	if uiFS != nil {
		mux.Handle("/", http.FileServer(http.FS(uiFS)))
	}

	return mux
}
