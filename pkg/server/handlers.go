package server

import (
	"archive/zip"
	"encoding/json"
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/media-luna/courtyard/pkg/engine"
	"github.com/media-luna/courtyard/pkg/git"
)

// Handlers holds dependencies for all HTTP API handlers.
type Handlers struct {
	registry *git.Registry
}

// NewHandlers creates a Handlers instance.
func NewHandlers(registry *git.Registry) *Handlers {
	return &Handlers{registry: registry}
}

// HandleMe returns the authenticated user's profile information.
func (h *Handlers) HandleMe(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	token := tokenFromContext(ctx)
	provider := providerFromContext(ctx)

	p, err := h.registry.Get(provider)
	if err != nil {
		writeError(w, http.StatusBadRequest, "unknown provider")
		return
	}

	info, err := p.GetCurrentUser(ctx, token)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, info)
}

// HandleListRepos lists all repos accessible to the authenticated user.
func (h *Handlers) HandleListRepos(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	token := tokenFromContext(ctx)
	provider := providerFromContext(ctx)

	p, err := h.registry.Get(provider)
	if err != nil {
		writeError(w, http.StatusBadRequest, "unknown provider")
		return
	}

	repos, err := p.GetRepositories(ctx, token)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, repos)
}

// HandleListForms lists available forms for a repository.
func (h *Handlers) HandleListForms(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	token := tokenFromContext(ctx)
	provider := providerFromContext(ctx)
	owner := r.PathValue("owner")
	repo := r.PathValue("repo")

	p, err := h.registry.Get(provider)
	if err != nil {
		writeError(w, http.StatusBadRequest, "unknown provider")
		return
	}

	forms, err := p.ListForms(ctx, token, owner, repo)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, forms)
}

// HandleGetForm returns the parsed form spec for a specific form.
func (h *Handlers) HandleGetForm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	token := tokenFromContext(ctx)
	provider := providerFromContext(ctx)
	owner := r.PathValue("owner")
	repo := r.PathValue("repo")
	form := r.PathValue("form")

	p, err := h.registry.Get(provider)
	if err != nil {
		writeError(w, http.StatusBadRequest, "unknown provider")
		return
	}

	cfg, err := p.GetFormConfig(ctx, token, owner, repo, form)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	spec, err := engine.ParseFormSpec(cfg.Raw)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, spec)
}

// HandlePreview evaluates templates and returns rendered files without committing.
func (h *Handlers) HandlePreview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	token := tokenFromContext(ctx)
	provider := providerFromContext(ctx)
	owner := r.PathValue("owner")
	repo := r.PathValue("repo")
	form := r.PathValue("form")

	if r.ContentLength > 1<<20 {
		writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
		return
	}

	var values map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&values); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	p, err := h.registry.Get(provider)
	if err != nil {
		writeError(w, http.StatusBadRequest, "unknown provider")
		return
	}

	cfg, err := p.GetFormConfig(ctx, token, owner, repo, form)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	spec, err := engine.ParseFormSpec(cfg.Raw)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	if fieldErrs := engine.ValidateValues(spec, values); len(fieldErrs) > 0 {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]interface{}{"errors": fieldErrs})
		return
	}

	templateFiles, err := p.GetTemplateFiles(ctx, token, owner, repo, form)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	files, err := engine.Evaluate(ctx, spec, templateFiles, values)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	type previewFile struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	var preview []previewFile
	for _, f := range files {
		preview = append(preview, previewFile{Path: f.Path, Content: string(f.Content)})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"files": preview})
}

const (
	defaultTreeMax = 500
	maxTreeMax     = 1000
)

// HandleListTree returns file paths under a given directory in the repo.
// Query params: path (directory prefix, default repo root), max (default 500, cap 1000).
// Response: {"paths": [...], "truncated": bool}
func (h *Handlers) HandleListTree(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	token := tokenFromContext(ctx)
	provider := providerFromContext(ctx)
	owner := r.PathValue("owner")
	repo := r.PathValue("repo")

	dirPath := r.URL.Query().Get("path")
	maxPaths := defaultTreeMax
	if s := r.URL.Query().Get("max"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			maxPaths = n
		}
	}
	if maxPaths > maxTreeMax {
		maxPaths = maxTreeMax
	}

	p, err := h.registry.Get(provider)
	if err != nil {
		writeError(w, http.StatusBadRequest, "unknown provider")
		return
	}

	paths, truncated, err := p.ListTree(ctx, token, owner, repo, dirPath, maxPaths)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if paths == nil {
		paths = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"paths":     paths,
		"truncated": truncated,
	})
}

// HandleSubmit evaluates templates and creates a PR.
func (h *Handlers) HandleSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	token := tokenFromContext(ctx)
	provider := providerFromContext(ctx)
	owner := r.PathValue("owner")
	repo := r.PathValue("repo")
	form := r.PathValue("form")

	if r.ContentLength > 1<<20 {
		writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
		return
	}

	var values map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&values); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	p, err := h.registry.Get(provider)
	if err != nil {
		writeError(w, http.StatusBadRequest, "unknown provider")
		return
	}

	// Always re-fetch templates fresh for submit.
	cfg, err := p.GetFormConfig(ctx, token, owner, repo, form)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	spec, err := engine.ParseFormSpec(cfg.Raw)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	if fieldErrs := engine.ValidateValues(spec, values); len(fieldErrs) > 0 {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]interface{}{"errors": fieldErrs})
		return
	}

	templateFiles, err := p.GetTemplateFiles(ctx, token, owner, repo, form)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	files, err := engine.Evaluate(ctx, spec, templateFiles, values)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	branchName, err := engine.EvaluateString(spec.BranchName, values)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "branch name template: "+err.Error())
		return
	}
	commitMsg, err := engine.EvaluateString(spec.CommitMessage, values)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "commit message template: "+err.Error())
		return
	}
	baseBranch := spec.TargetBranch
	if baseBranch == "" {
		baseBranch = "main"
	}

	prURL, err := p.CreateBranchAndPullRequest(ctx, token, owner, repo, baseBranch, branchName, commitMsg, files)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"pr_url": prURL})
}

// --- Studio handlers ---

// studioRequest is the common body for all three Studio endpoints.
type studioRequest struct {
	Owner     string            `json:"owner"`
	Repo      string            `json:"repo"`
	FormSpec  string            `json:"formSpec"`
	Templates map[string]string `json:"templates"`
	Values    map[string]interface{} `json:"values"`
}

// sanitizeTemplateName prevents path traversal by stripping ".." components
// and leading slashes from template file names supplied by the client.
func sanitizeTemplateName(name string) string {
	// Clean the path, then strip any leading slash.
	cleaned := path.Clean(name)
	cleaned = strings.TrimPrefix(cleaned, "/")
	// Reject any remaining traversal attempts.
	if strings.Contains(cleaned, "..") {
		return ""
	}
	return cleaned
}

// HandleStudioPreview parses an inline form spec + templates from the request
// body, evaluates them against the supplied values, and returns rendered files.
// No Git access is required — this is a pure engine operation.
func (h *Handlers) HandleStudioPreview(w http.ResponseWriter, r *http.Request) {
	if r.ContentLength > 1<<20 {
		writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
		return
	}
	var body studioRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if strings.TrimSpace(body.FormSpec) == "" {
		writeError(w, http.StatusBadRequest, "formSpec is required")
		return
	}

	spec, err := engine.ParseFormSpec([]byte(body.FormSpec))
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	values := body.Values
	if values == nil {
		values = map[string]interface{}{}
	}

	if fieldErrs := engine.ValidateValues(spec, values); len(fieldErrs) > 0 {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]interface{}{"errors": fieldErrs})
		return
	}

	templateFiles := make(map[string][]byte, len(body.Templates))
	for name, content := range body.Templates {
		safe := sanitizeTemplateName(name)
		if safe == "" {
			writeError(w, http.StatusBadRequest, "invalid template name: "+name)
			return
		}
		templateFiles[safe] = []byte(content)
	}

	files, err := engine.Evaluate(r.Context(), spec, templateFiles, values)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	type previewFile struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	var preview []previewFile
	for _, f := range files {
		preview = append(preview, previewFile{Path: f.Path, Content: string(f.Content)})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"files": preview})
}

// HandleStudioCommit commits the inline form spec and templates into the target
// repo as a new .courtyard/ config, then opens a Pull Request.
func (h *Handlers) HandleStudioCommit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	token := tokenFromContext(ctx)
	provider := providerFromContext(ctx)

	if r.ContentLength > 1<<20 {
		writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
		return
	}
	var body studioRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if body.Owner == "" || body.Repo == "" {
		writeError(w, http.StatusBadRequest, "owner and repo are required")
		return
	}
	if strings.TrimSpace(body.FormSpec) == "" {
		writeError(w, http.StatusBadRequest, "formSpec is required")
		return
	}

	spec, err := engine.ParseFormSpec([]byte(body.FormSpec))
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	var files []git.OutputFile

	// Add the form spec YAML.
	files = append(files, git.OutputFile{
		Path:    ".courtyard/forms/" + spec.Name + ".yaml",
		Content: []byte(body.FormSpec),
	})

	// Add each template file.
	for name, content := range body.Templates {
		safe := sanitizeTemplateName(name)
		if safe == "" {
			writeError(w, http.StatusBadRequest, "invalid template name: "+name)
			return
		}
		files = append(files, git.OutputFile{
			Path:    ".courtyard/templates/" + spec.Name + "/" + safe,
			Content: []byte(content),
		})
	}

	baseBranch := spec.TargetBranch
	if baseBranch == "" {
		baseBranch = "main"
	}
	branchName := "courtyard/studio/add-" + spec.Name
	commitMsg := "feat(.courtyard): add " + spec.Name + " form"

	p, err := h.registry.Get(provider)
	if err != nil {
		writeError(w, http.StatusBadRequest, "unknown provider")
		return
	}

	prURL, err := p.CreateBranchAndPullRequest(ctx, token, body.Owner, body.Repo, baseBranch, branchName, commitMsg, files)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"pr_url": prURL})
}

// HandleStudioDownload packages the inline form spec and templates into a ZIP
// file mirroring the .courtyard/ directory structure and returns it for download.
func (h *Handlers) HandleStudioDownload(w http.ResponseWriter, r *http.Request) {
	if r.ContentLength > 1<<20 {
		writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
		return
	}
	var body studioRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if strings.TrimSpace(body.FormSpec) == "" {
		writeError(w, http.StatusBadRequest, "formSpec is required")
		return
	}

	spec, err := engine.ParseFormSpec([]byte(body.FormSpec))
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="courtyard-`+spec.Name+`.zip"`)

	zw := zip.NewWriter(w)
	defer zw.Close()

	// Write form spec.
	fw, err := zw.Create(".courtyard/forms/" + spec.Name + ".yaml")
	if err != nil {
		// Headers already sent; we can only log the issue at this point.
		return
	}
	fw.Write([]byte(body.FormSpec)) //nolint:errcheck

	// Write each template file.
	for name, content := range body.Templates {
		safe := sanitizeTemplateName(name)
		if safe == "" {
			continue // skip invalid names silently — ZIP is best-effort
		}
		tf, err := zw.Create(".courtyard/templates/" + spec.Name + "/" + safe)
		if err != nil {
			return
		}
		tf.Write([]byte(content)) //nolint:errcheck
	}
}

// --- helpers ---

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
