package server

import (
	"encoding/json"
	"net/http"

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

// HandleMe returns the authenticated user's provider name.
func (h *Handlers) HandleMe(w http.ResponseWriter, r *http.Request) {
	provider := providerFromContext(r.Context())
	writeJSON(w, http.StatusOK, map[string]string{"provider": provider})
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

// --- helpers ---

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
