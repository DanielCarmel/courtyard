package server

import (
	"context"
	"net/http"
	"time"

	"golang.org/x/oauth2"
	bitbucketoauth "golang.org/x/oauth2/bitbucket"
	githuboauth "golang.org/x/oauth2/github"
)

const (
	sessionCookieName = "courtyard_session"
	stateCookieName   = "courtyard_oauth_state"
	stateCookieTTL    = 5 * time.Minute
)

// OAuthConfig holds the OAuth2 configs and session store needed by the OAuth handlers.
type OAuthConfig struct {
	GitHub    *oauth2.Config
	Bitbucket *oauth2.Config
	Sessions  *SessionStore
	BaseURL   string
}

// NewOAuthConfig builds the OAuthConfig from environment-supplied credentials.
func NewOAuthConfig(githubClientID, githubClientSecret, bitbucketClientID, bitbucketClientSecret, baseURL string) *OAuthConfig {
	return &OAuthConfig{
		BaseURL: baseURL,
		GitHub: &oauth2.Config{
			ClientID:     githubClientID,
			ClientSecret: githubClientSecret,
			Scopes:       []string{"repo", "read:user"},
			Endpoint:     githuboauth.Endpoint,
			RedirectURL:  baseURL + "/auth/github/callback",
		},
		Bitbucket: &oauth2.Config{
			ClientID:     bitbucketClientID,
			ClientSecret: bitbucketClientSecret,
			Scopes:       []string{"repository", "repository:write", "pullrequest", "pullrequest:write"},
			Endpoint:     bitbucketoauth.Endpoint,
			RedirectURL:  baseURL + "/auth/bitbucket/callback",
		},
	}
}

// HandleGitHubLogin redirects the user to GitHub for OAuth authorization.
func (o *OAuthConfig) HandleGitHubLogin(w http.ResponseWriter, r *http.Request) {
	state, err := randomHex(16)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	setStateCookie(w, state)
	http.Redirect(w, r, o.GitHub.AuthCodeURL(state), http.StatusFound)
}

// HandleGitHubCallback exchanges the OAuth code for a token and creates a session.
func (o *OAuthConfig) HandleGitHubCallback(w http.ResponseWriter, r *http.Request) {
	if !verifyState(r) {
		http.Error(w, "invalid state", http.StatusBadRequest)
		return
	}
	clearStateCookie(w)

	tok, err := o.GitHub.Exchange(context.Background(), r.URL.Query().Get("code"))
	if err != nil {
		http.Error(w, "token exchange failed", http.StatusBadRequest)
		return
	}

	sess, err := o.Sessions.Create(tok.AccessToken, "github")
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	setSessionCookie(w, sess.ID)
	http.Redirect(w, r, "/", http.StatusFound)
}

// HandleBitbucketLogin redirects the user to Bitbucket for OAuth authorization.
func (o *OAuthConfig) HandleBitbucketLogin(w http.ResponseWriter, r *http.Request) {
	state, err := randomHex(16)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	setStateCookie(w, state)
	http.Redirect(w, r, o.Bitbucket.AuthCodeURL(state), http.StatusFound)
}

// HandleBitbucketCallback exchanges the OAuth code for a token and creates a session.
func (o *OAuthConfig) HandleBitbucketCallback(w http.ResponseWriter, r *http.Request) {
	if !verifyState(r) {
		http.Error(w, "invalid state", http.StatusBadRequest)
		return
	}
	clearStateCookie(w)

	tok, err := o.Bitbucket.Exchange(context.Background(), r.URL.Query().Get("code"))
	if err != nil {
		http.Error(w, "token exchange failed", http.StatusBadRequest)
		return
	}

	sess, err := o.Sessions.Create(tok.AccessToken, "bitbucket")
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	setSessionCookie(w, sess.ID)
	http.Redirect(w, r, "/", http.StatusFound)
}

// HandleLogout clears the session and redirects to the home page.
func (o *OAuthConfig) HandleLogout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(sessionCookieName)
	if err == nil {
		o.Sessions.Delete(cookie.Value)
	}
	clearSessionCookie(w)
	http.Redirect(w, r, "/", http.StatusFound)
}

// --- cookie helpers ---

func setStateCookie(w http.ResponseWriter, state string) {
	http.SetCookie(w, &http.Cookie{
		Name:     stateCookieName,
		Value:    state,
		Path:     "/",
		MaxAge:   int(stateCookieTTL.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func clearStateCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     stateCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func verifyState(r *http.Request) bool {
	cookie, err := r.Cookie(stateCookieName)
	if err != nil {
		return false
	}
	return cookie.Value != "" && cookie.Value == r.URL.Query().Get("state")
}

func setSessionCookie(w http.ResponseWriter, sessionID string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    sessionID,
		Path:     "/",
		MaxAge:   int(sessionTTL.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}
