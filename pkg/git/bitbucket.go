package git

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"
)

const bitbucketDefaultAPIBase = "https://api.bitbucket.org/2.0"

// BitbucketProvider implements GitProvider using the Bitbucket Cloud REST API.
type BitbucketProvider struct {
	httpClient *http.Client
	apiBase    string // injectable for tests; defaults to bitbucketDefaultAPIBase
}

// NewBitbucketProvider returns a new BitbucketProvider.
func NewBitbucketProvider() *BitbucketProvider {
	return &BitbucketProvider{httpClient: http.DefaultClient, apiBase: bitbucketDefaultAPIBase}
}

func (b *BitbucketProvider) do(ctx context.Context, token, method, url string, body io.Reader, contentType string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	return b.httpClient.Do(req)
}

func (b *BitbucketProvider) doJSON(ctx context.Context, token, method, url string, out interface{}) error {
	resp, err := b.do(ctx, token, method, url, nil, "")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(bodyBytes))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// GetRepositories aggregates repos across all workspaces the user belongs to.
func (b *BitbucketProvider) GetRepositories(ctx context.Context, token string) ([]Repository, error) {
	// List all workspaces.
	type workspace struct {
		Slug string `json:"slug"`
	}
	type workspacePage struct {
		Values  []workspace `json:"values"`
		Next    string      `json:"next"`
	}

	var workspaces []workspace
	url := b.apiBase + "/workspaces?role=member&pagelen=50"
	for url != "" {
		var page workspacePage
		if err := b.doJSON(ctx, token, "GET", url, &page); err != nil {
			return nil, fmt.Errorf("GetRepositories: list workspaces: %w", err)
		}
		workspaces = append(workspaces, page.Values...)
		url = page.Next
	}

	// Aggregate repos across all workspaces.
	type repoLinks struct {
		Clone []struct {
			Href string `json:"href"`
			Name string `json:"name"`
		} `json:"clone"`
	}
	type bbRepo struct {
		Slug          string    `json:"slug"`
		FullName      string    `json:"full_name"`
		MainBranch    struct{ Name string `json:"name"` } `json:"mainbranch"`
		Links         repoLinks `json:"links"`
	}
	type repoPage struct {
		Values []bbRepo `json:"values"`
		Next   string   `json:"next"`
	}

	var repos []Repository
	for _, ws := range workspaces {
		pageURL := fmt.Sprintf("%s/repositories/%s?pagelen=50", b.apiBase, ws.Slug)
		for pageURL != "" {
			var page repoPage
			if err := b.doJSON(ctx, token, "GET", pageURL, &page); err != nil {
				return nil, fmt.Errorf("GetRepositories: list repos for workspace %q: %w", ws.Slug, err)
			}
			for _, r := range page.Values {
				parts := strings.SplitN(r.FullName, "/", 2)
				owner := ""
				name := r.Slug
				if len(parts) == 2 {
					owner, name = parts[0], parts[1]
				}
				cloneURL := ""
				for _, c := range r.Links.Clone {
					if c.Name == "https" {
						cloneURL = c.Href
					}
				}
				defaultBranch := r.MainBranch.Name
				if defaultBranch == "" {
					defaultBranch = "main"
				}
				repos = append(repos, Repository{
					Owner:         owner,
					Name:          name,
					CloneURL:      cloneURL,
					DefaultBranch: defaultBranch,
				})
			}
			pageURL = page.Next
		}
	}
	return repos, nil
}

// ListForms returns names of forms in .courtyard/forms/ (without extension).
func (b *BitbucketProvider) ListForms(ctx context.Context, token, owner, repo string) ([]string, error) {
	url := fmt.Sprintf("%s/repositories/%s/%s/src/HEAD/.courtyard/forms/", b.apiBase, owner, repo)
	type entry struct {
		Path string `json:"path"`
		Type string `json:"type"`
	}
	type page struct {
		Values []entry `json:"values"`
	}
	var p page
	if err := b.doJSON(ctx, token, "GET", url, &p); err != nil {
		return nil, fmt.Errorf("ListForms: %w", err)
	}
	var forms []string
	for _, e := range p.Values {
		if e.Type != "commit_file" {
			continue
		}
		name := e.Path[strings.LastIndex(e.Path, "/")+1:]
		if strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml") {
			forms = append(forms, strings.TrimSuffix(strings.TrimSuffix(name, ".yaml"), ".yml"))
		}
	}
	return forms, nil
}

// GetFormConfig fetches and returns the raw YAML for a form spec.
func (b *BitbucketProvider) GetFormConfig(ctx context.Context, token, owner, repo, formName string) (*FormConfig, error) {
	url := fmt.Sprintf("%s/repositories/%s/%s/src/HEAD/.courtyard/forms/%s.yaml", b.apiBase, owner, repo, formName)
	resp, err := b.do(ctx, token, "GET", url, nil, "")
	if err != nil {
		return nil, fmt.Errorf("GetFormConfig: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GetFormConfig: HTTP %d: %s", resp.StatusCode, string(body))
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("GetFormConfig: read body: %w", err)
	}
	return &FormConfig{Raw: raw}, nil
}

// GetTemplateFiles returns the raw contents of all template files for a form.
func (b *BitbucketProvider) GetTemplateFiles(ctx context.Context, token, owner, repo, formName string) (map[string][]byte, error) {
	prefix := fmt.Sprintf(".courtyard/templates/%s/", formName)
	listURL := fmt.Sprintf("%s/repositories/%s/%s/src/HEAD/%s", b.apiBase, owner, repo, prefix)

	type entry struct {
		Path string `json:"path"`
		Type string `json:"type"`
	}
	type page struct {
		Values []entry `json:"values"`
		Next   string  `json:"next"`
	}

	var entries []entry
	url := listURL
	for url != "" {
		var p page
		if err := b.doJSON(ctx, token, "GET", url, &p); err != nil {
			return nil, fmt.Errorf("GetTemplateFiles: list: %w", err)
		}
		entries = append(entries, p.Values...)
		url = p.Next
	}

	files := make(map[string][]byte)
	for _, e := range entries {
		if e.Type != "commit_file" {
			continue
		}
		fileURL := fmt.Sprintf("%s/repositories/%s/%s/src/HEAD/%s", b.apiBase, owner, repo, e.Path)
		resp, err := b.do(ctx, token, "GET", fileURL, nil, "")
		if err != nil {
			return nil, fmt.Errorf("GetTemplateFiles: fetch %q: %w", e.Path, err)
		}
		content, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("GetTemplateFiles: read %q: %w", e.Path, err)
		}
		relPath := strings.TrimPrefix(e.Path, prefix)
		files[relPath] = content
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("GetTemplateFiles: no templates found at %q", prefix)
	}
	return files, nil
}

// CreateBranchAndPullRequest creates a branch, commits all files, and opens a PR.
func (b *BitbucketProvider) CreateBranchAndPullRequest(
	ctx context.Context,
	token, owner, repo, baseBranch, branchName, commitMessage string,
	files []OutputFile,
) (string, error) {
	// 1. Create the branch.
	branchBody, _ := json.Marshal(map[string]interface{}{
		"name": branchName,
		"target": map[string]string{"hash": baseBranch},
	})
	branchURL := fmt.Sprintf("%s/repositories/%s/%s/refs/branches", b.apiBase, owner, repo)
	resp, err := b.do(ctx, token, "POST", branchURL, bytes.NewReader(branchBody), "application/json")
	if err != nil {
		return "", fmt.Errorf("CreateBranchAndPullRequest: create branch: %w", err)
	}
	resp.Body.Close()
	// 409 conflict = branch already exists; treat as acceptable for reuse mode.
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusConflict {
		return "", fmt.Errorf("CreateBranchAndPullRequest: create branch HTTP %d", resp.StatusCode)
	}

	// 2. Commit files via multipart/form-data POST to /src.
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	// Add branch field.
	_ = mw.WriteField("branch", branchName)
	_ = mw.WriteField("message", commitMessage)

	// Add each file.
	for _, f := range files {
		h := make(textproto.MIMEHeader)
		h.Set("Content-Disposition", fmt.Sprintf(`form-data; name=%q; filename=%q`, f.Path, f.Path))
		h.Set("Content-Type", "application/octet-stream")
		part, err := mw.CreatePart(h)
		if err != nil {
			return "", fmt.Errorf("CreateBranchAndPullRequest: create part %q: %w", f.Path, err)
		}
		if _, err := part.Write(f.Content); err != nil {
			return "", fmt.Errorf("CreateBranchAndPullRequest: write part %q: %w", f.Path, err)
		}
	}
	mw.Close()

	srcURL := fmt.Sprintf("%s/repositories/%s/%s/src", b.apiBase, owner, repo)
	resp, err = b.do(ctx, token, "POST", srcURL, &buf, mw.FormDataContentType())
	if err != nil {
		return "", fmt.Errorf("CreateBranchAndPullRequest: commit files: %w", err)
	}
	resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("CreateBranchAndPullRequest: commit files HTTP %d", resp.StatusCode)
	}

	// 3. Open a PR.
	prBody, _ := json.Marshal(map[string]interface{}{
		"title": commitMessage,
		"source": map[string]interface{}{
			"branch": map[string]string{"name": branchName},
		},
		"destination": map[string]interface{}{
			"branch": map[string]string{"name": baseBranch},
		},
		"description": "Created by Courtyard",
	})
	prURL := fmt.Sprintf("%s/repositories/%s/%s/pullrequests", b.apiBase, owner, repo)
	resp, err = b.do(ctx, token, "POST", prURL, bytes.NewReader(prBody), "application/json")
	if err != nil {
		return "", fmt.Errorf("CreateBranchAndPullRequest: create PR: %w", err)
	}
	defer resp.Body.Close()
	// 409 = PR already exists for this branch pair.
	if resp.StatusCode == http.StatusConflict {
		return "", nil // PR already open — caller may handle
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("CreateBranchAndPullRequest: create PR HTTP %d: %s", resp.StatusCode, string(body))
	}

	var pr struct {
		Links struct {
			HTML struct {
				Href string `json:"href"`
			} `json:"html"`
		} `json:"links"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&pr); err != nil {
		return "", fmt.Errorf("CreateBranchAndPullRequest: decode PR response: %w", err)
	}
	return pr.Links.HTML.Href, nil
}
