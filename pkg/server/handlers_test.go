package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/media-luna/courtyard/pkg/git"
)

// --- stub provider ---

type stubGitProvider struct {
	repos         []git.Repository
	forms         []string
	formConfig    *git.FormConfig
	templates     map[string][]byte
	treeFiles     []string
	treeTruncated bool
	prURL         string
	err           error
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
func (s *stubGitProvider) GetCurrentUser(_ context.Context, _ string) (*git.UserInfo, error) {
	if s.err != nil {
		return nil, s.err
	}
	return &git.UserInfo{Username: "testuser", AvatarURL: "https://example.com/avatar.png", Provider: "github"}, nil
}
func (s *stubGitProvider) CreateBranchAndPullRequest(_ context.Context, _ string, _, _, _, _, _ string, _ []git.OutputFile) (string, error) {
	return s.prURL, s.err
}
func (s *stubGitProvider) ListTree(_ context.Context, _ string, _, _, _ string, _ int) ([]string, bool, error) {
	return s.treeFiles, s.treeTruncated, s.err
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
	if body["username"] != "testuser" {
		t.Errorf("username: got %q, want 'testuser'", body["username"])
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

// --- Studio handler tests ---

const studioFormYAML = `
name: my-service
branchName: "courtyard/{{ .team }}/{{ .name }}"
commitMessage: "feat: add {{ .name }}"
outputPath: "services/{{ .team }}"
targetBranch: main
fields:
  - name: team
    type: string
    label: Team
    required: true
  - name: name
    type: string
    label: Name
    required: true
`

func TestHandleStudioPreview(t *testing.T) {
	t.Run("happy path renders templates", func(t *testing.T) {
		stub := &stubGitProvider{}
		srv, sessions := newTestServer(stub)
		defer srv.Close()

		sess, _ := sessions.Create("tok", "github")
		payload, _ := json.Marshal(map[string]interface{}{
			"formSpec":  studioFormYAML,
			"templates": map[string]string{"service.yaml.tmpl": "team: {{ .team }}\nname: {{ .name }}"},
			"values":    map[string]interface{}{"team": "platform", "name": "svc-a"},
		})
		req, _ := http.NewRequest("POST", srv.URL+"/api/studio/preview", bytes.NewReader(payload))
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
		files, ok := result["files"].([]interface{})
		if !ok || len(files) == 0 {
			t.Fatalf("expected files in response, got: %+v", result)
		}
		f := files[0].(map[string]interface{})
		if !bytes.Contains([]byte(f["content"].(string)), []byte("platform")) {
			t.Errorf("rendered content missing value: %s", f["content"])
		}
	})

	t.Run("invalid spec YAML returns 422", func(t *testing.T) {
		stub := &stubGitProvider{}
		srv, sessions := newTestServer(stub)
		defer srv.Close()

		sess, _ := sessions.Create("tok", "github")
		payload, _ := json.Marshal(map[string]interface{}{
			"formSpec":  "not: valid: yaml: ::::",
			"templates": map[string]string{},
			"values":    map[string]interface{}{},
		})
		req, _ := http.NewRequest("POST", srv.URL+"/api/studio/preview", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Requested-With", "XMLHttpRequest")
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sess.ID})

		resp, _ := http.DefaultClient.Do(req)
		if resp.StatusCode != http.StatusUnprocessableEntity {
			t.Errorf("got %d, want 422", resp.StatusCode)
		}
	})

	t.Run("validation errors return 422 with errors map", func(t *testing.T) {
		stub := &stubGitProvider{}
		srv, sessions := newTestServer(stub)
		defer srv.Close()

		sess, _ := sessions.Create("tok", "github")
		// required fields not supplied
		payload, _ := json.Marshal(map[string]interface{}{
			"formSpec":  studioFormYAML,
			"templates": map[string]string{},
			"values":    map[string]interface{}{},
		})
		req, _ := http.NewRequest("POST", srv.URL+"/api/studio/preview", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Requested-With", "XMLHttpRequest")
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sess.ID})

		resp, _ := http.DefaultClient.Do(req)
		if resp.StatusCode != http.StatusUnprocessableEntity {
			t.Errorf("got %d, want 422", resp.StatusCode)
		}
		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)
		if result["errors"] == nil {
			t.Error("expected 'errors' key in response")
		}
	})

	t.Run("missing formSpec returns 400", func(t *testing.T) {
		stub := &stubGitProvider{}
		srv, sessions := newTestServer(stub)
		defer srv.Close()

		sess, _ := sessions.Create("tok", "github")
		payload, _ := json.Marshal(map[string]interface{}{
			"templates": map[string]string{},
			"values":    map[string]interface{}{},
		})
		req, _ := http.NewRequest("POST", srv.URL+"/api/studio/preview", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Requested-With", "XMLHttpRequest")
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sess.ID})

		resp, _ := http.DefaultClient.Do(req)
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("got %d, want 400", resp.StatusCode)
		}
	})
}

func TestHandleStudioCommit(t *testing.T) {
	t.Run("happy path creates PR", func(t *testing.T) {
		stub := &stubGitProvider{prURL: "https://github.com/alice/myrepo/pull/42"}
		srv, sessions := newTestServer(stub)
		defer srv.Close()

		sess, _ := sessions.Create("tok", "github")
		payload, _ := json.Marshal(map[string]interface{}{
			"owner":     "alice",
			"repo":      "myrepo",
			"formSpec":  studioFormYAML,
			"templates": map[string]string{"service.yaml.tmpl": "name: {{ .name }}"},
		})
		req, _ := http.NewRequest("POST", srv.URL+"/api/studio/commit", bytes.NewReader(payload))
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
		var result map[string]string
		json.NewDecoder(resp.Body).Decode(&result)
		if result["pr_url"] != stub.prURL {
			t.Errorf("pr_url: got %q, want %q", result["pr_url"], stub.prURL)
		}
	})

	t.Run("missing owner returns 400", func(t *testing.T) {
		stub := &stubGitProvider{}
		srv, sessions := newTestServer(stub)
		defer srv.Close()

		sess, _ := sessions.Create("tok", "github")
		payload, _ := json.Marshal(map[string]interface{}{
			"repo":      "myrepo",
			"formSpec":  studioFormYAML,
			"templates": map[string]string{},
		})
		req, _ := http.NewRequest("POST", srv.URL+"/api/studio/commit", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Requested-With", "XMLHttpRequest")
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sess.ID})

		resp, _ := http.DefaultClient.Do(req)
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("got %d, want 400", resp.StatusCode)
		}
	})
}

func TestHandleStudioDownload(t *testing.T) {
	t.Run("returns ZIP with correct structure", func(t *testing.T) {
		stub := &stubGitProvider{}
		srv, sessions := newTestServer(stub)
		defer srv.Close()

		sess, _ := sessions.Create("tok", "github")
		payload, _ := json.Marshal(map[string]interface{}{
			"formSpec":  studioFormYAML,
			"templates": map[string]string{"service.yaml.tmpl": "name: {{ .name }}"},
		})
		req, _ := http.NewRequest("POST", srv.URL+"/api/studio/download", bytes.NewReader(payload))
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
		if ct := resp.Header.Get("Content-Type"); ct != "application/zip" {
			t.Errorf("Content-Type: got %q, want application/zip", ct)
		}
	})

	t.Run("missing formSpec returns 400", func(t *testing.T) {
		stub := &stubGitProvider{}
		srv, sessions := newTestServer(stub)
		defer srv.Close()

		sess, _ := sessions.Create("tok", "github")
		payload, _ := json.Marshal(map[string]interface{}{
			"templates": map[string]string{},
		})
		req, _ := http.NewRequest("POST", srv.URL+"/api/studio/download", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Requested-With", "XMLHttpRequest")
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sess.ID})

		resp, _ := http.DefaultClient.Do(req)
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("got %d, want 400", resp.StatusCode)
		}
	})
}

func TestHandleListTree(t *testing.T) {
	t.Run("returns paths and truncated=false", func(t *testing.T) {
		stub := &stubGitProvider{
			treeFiles:     []string{"clusters/dev/app/deployment.yaml", "clusters/dev/app/service.yaml"},
			treeTruncated: false,
		}
		srv, sessions := newTestServer(stub)
		defer srv.Close()

		sess, _ := sessions.Create("tok", "github")
		req, _ := http.NewRequest("GET", srv.URL+"/api/repos/alice/myrepo/tree?path=clusters/dev/app", nil)
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
		if result["truncated"] != false {
			t.Errorf("truncated: got %v, want false", result["truncated"])
		}
		paths, ok := result["paths"].([]interface{})
		if !ok || len(paths) != 2 {
			t.Errorf("expected 2 paths, got: %+v", result["paths"])
		}
	})

	t.Run("returns truncated=true when capped", func(t *testing.T) {
		stub := &stubGitProvider{
			treeFiles:     []string{"a/b.yaml"},
			treeTruncated: true,
		}
		srv, sessions := newTestServer(stub)
		defer srv.Close()

		sess, _ := sessions.Create("tok", "github")
		req, _ := http.NewRequest("GET", srv.URL+"/api/repos/alice/myrepo/tree", nil)
		req.Header.Set("X-Requested-With", "XMLHttpRequest")
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sess.ID})

		resp, _ := http.DefaultClient.Do(req)
		if resp.StatusCode != http.StatusOK {
			t.Errorf("got %d, want 200", resp.StatusCode)
		}
		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)
		if result["truncated"] != true {
			t.Errorf("truncated: got %v, want true", result["truncated"])
		}
	})

	t.Run("provider error returns 500", func(t *testing.T) {
		stub := &stubGitProvider{err: fmt.Errorf("git unavailable")}
		srv, sessions := newTestServer(stub)
		defer srv.Close()

		sess, _ := sessions.Create("tok", "github")
		req, _ := http.NewRequest("GET", srv.URL+"/api/repos/alice/myrepo/tree", nil)
		req.Header.Set("X-Requested-With", "XMLHttpRequest")
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sess.ID})

		resp, _ := http.DefaultClient.Do(req)
		if resp.StatusCode != http.StatusInternalServerError {
			t.Errorf("got %d, want 500", resp.StatusCode)
		}
	})

	t.Run("unauthenticated returns 401", func(t *testing.T) {
		stub := &stubGitProvider{}
		srv, _ := newTestServer(stub)
		defer srv.Close()

		resp, err := http.DefaultClient.Get(srv.URL + "/api/repos/alice/myrepo/tree")
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("got %d, want 401", resp.StatusCode)
		}
	})
}
