package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/media-luna/courtyard/pkg/git"
)

// --- stub provider ---

type stubGitProvider struct {
	repos      []git.Repository
	forms      []string
	formConfig *git.FormConfig
	templates  map[string][]byte
	prURL      string
	err        error
}

func (s *stubGitProvider) GetRepositories(_ context.Context, _ string) ([]git.Repository, error) {
	return s.repos, s.err
}
func (s *stubGitProvider) ListForms(_ context.Context, _ string, _, _ string) ([]string, error) {
	return s.forms, s.err
}
func (s *stubGitProvider) GetFormConfig(_ context.Context, _ string, _, _, _ string) (*git.FormConfig, error) {
	return s.formConfig, s.err
}
func (s *stubGitProvider) GetTemplateFiles(_ context.Context, _ string, _, _, _ string) (map[string][]byte, error) {
	return s.templates, s.err
}
func (s *stubGitProvider) CreateBranchAndPullRequest(_ context.Context, _ string, _, _, _, _, _ string, _ []git.OutputFile) (string, error) {
	return s.prURL, s.err
}

// --- test setup helper ---

func newTestServer(stub *stubGitProvider) (*httptest.Server, *SessionStore) {
	registry := git.NewRegistry(map[string]git.GitProvider{"github": stub})
	sessions := NewSessionStore()
	oauth := NewOAuthConfig("id", "secret", "bid", "bsecret", "http://localhost")
	oauth.Sessions = sessions
	handlers := NewHandlers(registry)
	mux := NewRouter(handlers, oauth, sessions, nil)
	return httptest.NewServer(mux), sessions
}

func authenticatedRequest(t *testing.T, sessions *SessionStore, method, url string, body []byte) *http.Request {
	t.Helper()
	sess, err := sessions.Create("mytoken", "github")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	var req *http.Request
	if body != nil {
		req = httptest.NewRequest(method, url, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, url, nil)
	}
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sess.ID})
	return req
}

func TestHandleMe(t *testing.T) {
	stub := &stubGitProvider{}
	srv, sessions := newTestServer(stub)
	defer srv.Close()

	sess, _ := sessions.Create("tok", "github")
	req, _ := http.NewRequest("GET", srv.URL+"/api/me", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sess.ID})

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("got %d, want 200", resp.StatusCode)
	}
	var body map[string]string
	json.NewDecoder(resp.Body).Decode(&body)
	if body["provider"] != "github" {
		t.Errorf("provider: got %q, want 'github'", body["provider"])
	}
}

func TestHandleListRepos(t *testing.T) {
	stub := &stubGitProvider{
		repos: []git.Repository{
			{Owner: "alice", Name: "myrepo", DefaultBranch: "main"},
		},
	}
	srv, sessions := newTestServer(stub)
	defer srv.Close()

	sess, _ := sessions.Create("tok", "github")
	req, _ := http.NewRequest("GET", srv.URL+"/api/repos", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sess.ID})

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("got %d, want 200", resp.StatusCode)
	}
	var repos []git.Repository
	json.NewDecoder(resp.Body).Decode(&repos)
	if len(repos) != 1 || repos[0].Name != "myrepo" {
		t.Errorf("unexpected repos: %+v", repos)
	}
}

func TestHandleListRepos_Unauthenticated(t *testing.T) {
	stub := &stubGitProvider{}
	srv, _ := newTestServer(stub)
	defer srv.Close()

	resp, err := http.DefaultClient.Get(srv.URL + "/api/repos")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("got %d, want 401", resp.StatusCode)
	}
}

func TestHandleHealthz(t *testing.T) {
	stub := &stubGitProvider{}
	srv, _ := newTestServer(stub)
	defer srv.Close()

	resp, err := http.DefaultClient.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("got %d, want 200", resp.StatusCode)
	}
}

func TestHandlePreview(t *testing.T) {
	formYAML := []byte(`
name: deploy
branchName: "courtyard/{{ .app }}"
commitMessage: "deploy {{ .app }}"
outputPath: "clusters"
fields:
  - name: app
    type: string
    label: App
    required: true
`)
	stub := &stubGitProvider{
		formConfig: &git.FormConfig{Raw: formYAML},
		templates:  map[string][]byte{"deploy.yaml.tmpl": []byte("app: {{ .app }}")},
	}
	srv, sessions := newTestServer(stub)
	defer srv.Close()

	sess, _ := sessions.Create("tok", "github")
	body, _ := json.Marshal(map[string]interface{}{"app": "myapp"})
	req, _ := http.NewRequest("POST", srv.URL+"/api/repos/alice/myrepo/forms/deploy/preview", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sess.ID})

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("got %d, want 200", resp.StatusCode)
	}
	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	if _, ok := result["files"]; !ok {
		t.Error("response missing 'files' key")
	}
}
