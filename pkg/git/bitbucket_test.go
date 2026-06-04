package git

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newBitbucketTestProvider(srv *httptest.Server) *BitbucketProvider {
	return &BitbucketProvider{httpClient: srv.Client(), apiBase: srv.URL}
}

func TestBitbucketProvider_ImplementsInterface(t *testing.T) {
	var _ GitProvider = &BitbucketProvider{}
}

func TestBitbucketProvider_GetRepositories(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/workspaces":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"values": []map[string]string{{"slug": "myws"}},
			})
		case strings.HasPrefix(r.URL.Path, "/repositories/myws"):
			json.NewEncoder(w).Encode(map[string]interface{}{
				"values": []map[string]interface{}{
					{
						"slug":       "myrepo",
						"full_name":  "myws/myrepo",
						"mainbranch": map[string]string{"name": "main"},
						"links": map[string]interface{}{
							"clone": []map[string]string{
								{"name": "https", "href": "https://bitbucket.org/myws/myrepo.git"},
							},
						},
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	p := newBitbucketTestProvider(srv)
	repos, err := p.GetRepositories(context.Background(), "tok")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repos) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(repos))
	}
	if repos[0].Owner != "myws" {
		t.Errorf("owner: got %q, want %q", repos[0].Owner, "myws")
	}
	if repos[0].Name != "myrepo" {
		t.Errorf("name: got %q, want %q", repos[0].Name, "myrepo")
	}
	if repos[0].DefaultBranch != "main" {
		t.Errorf("defaultBranch: got %q, want %q", repos[0].DefaultBranch, "main")
	}
}

func TestBitbucketProvider_GetRepositories_Pagination(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/workspaces" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"values": []map[string]string{{"slug": "ws"}},
			})
			return
		}
		callCount++
		base := "http://" + r.Host
		if callCount == 1 {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"values": []map[string]interface{}{
					{"slug": "r1", "full_name": "ws/r1", "mainbranch": map[string]string{"name": "main"},
						"links": map[string]interface{}{"clone": []map[string]string{}}},
				},
				"next": base + "/repositories/ws?page=2",
			})
		} else {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"values": []map[string]interface{}{
					{"slug": "r2", "full_name": "ws/r2", "mainbranch": map[string]string{"name": "develop"},
						"links": map[string]interface{}{"clone": []map[string]string{}}},
				},
			})
		}
	}))
	defer srv.Close()

	p := newBitbucketTestProvider(srv)
	repos, err := p.GetRepositories(context.Background(), "tok")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repos) != 2 {
		t.Fatalf("expected 2 repos (pagination), got %d", len(repos))
	}
}

func TestBitbucketProvider_ListForms(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"values": []map[string]string{
				{"path": ".courtyard/forms/deploy.yaml", "type": "commit_file"},
				{"path": ".courtyard/forms/rollback.yml", "type": "commit_file"},
				{"path": ".courtyard/forms/", "type": "commit_directory"},
			},
		})
	}))
	defer srv.Close()

	p := newBitbucketTestProvider(srv)
	forms, err := p.ListForms(context.Background(), "tok", "myws", "myrepo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(forms) != 2 {
		t.Fatalf("expected 2 forms, got %d: %v", len(forms), forms)
	}
	for _, f := range forms {
		if strings.HasSuffix(f, ".yaml") || strings.HasSuffix(f, ".yml") {
			t.Errorf("form name should not include extension: %q", f)
		}
	}
}

func TestBitbucketProvider_GetFormConfig(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("name: deploy\n"))
	}))
	defer srv.Close()

	p := newBitbucketTestProvider(srv)
	cfg, err := p.GetFormConfig(context.Background(), "tok", "myws", "myrepo", "deploy")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(cfg.Raw) != "name: deploy\n" {
		t.Errorf("raw: got %q, want %q", string(cfg.Raw), "name: deploy\n")
	}
}
