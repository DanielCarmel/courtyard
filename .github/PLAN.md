# Plan: Media Luna Courtyard — Stateless GitOps Proxy

## TL;DR
Build a single-binary Go web application that reads `.courtyard/` specs from Git repos, renders dynamic forms from JSON Schema, evaluates Go templates with user input, and opens Pull Requests using the logged-in user's OAuth tokens. No database, no in-cluster state — Git is the source of truth.

---

## Phase 0: Project Scaffolding

**Goal:** Initialize Go module, directory layout, Dockerfile skeleton, and CI-ready structure.

1. `go mod init github.com/media-luna/courtyard`
2. Create directory tree:
   ```
   cmd/courtyard/main.go
   pkg/git/provider.go
   pkg/git/models.go
   pkg/git/github.go
   pkg/git/bitbucket.go
   pkg/engine/parser.go
   pkg/engine/evaluator.go
   pkg/server/router.go
   pkg/server/middleware.go
   pkg/server/oauth.go
   pkg/server/handlers.go
   ui/index.html
   ui/static/app.js
   ui/static/style.css
   Dockerfile
   .gitignore
   README.md
   ```
3. Minimal `main.go` that starts an HTTP server on `:8080` and serves a health-check endpoint `/healthz`.
4. Dockerfile: multi-stage build (Go builder → `scratch`/`distroless` runtime).

**Verification:** `go build ./cmd/courtyard && curl localhost:8080/healthz` returns 200.

---

## Phase 1: Core Interfaces & Models

**Goal:** Define domain types and the `GitProvider` interface so GitHub/Bitbucket can be swapped cleanly.

### Step 1.1 — Domain models (`pkg/git/models.go`)
Define:
- `Repository` — `Owner string`, `Name string`, `CloneURL string`, `DefaultBranch string`
- `FormConfig` — see `FormSpec` in Phase 4 (custom YAML field definitions, not JSON Schema)
- `OutputFile` — `Path string`, `Content []byte`

### Step 1.2 — GitProvider interface (`pkg/git/provider.go`)
```go
type GitProvider interface {
    GetRepositories(ctx context.Context, token string) ([]Repository, error)
    GetFormConfig(ctx context.Context, token string, owner string, repo string, formName string) (*FormConfig, error)
    ListForms(ctx context.Context, token string, owner string, repo string) ([]string, error)
    GetTemplateFiles(ctx context.Context, token string, owner string, repo string, formName string) (map[string][]byte, error)
    CreateBranchAndPullRequest(ctx context.Context, token string, owner string, repo string, baseBranch string, branchName string, commitMessage string, files []OutputFile) (prURL string, err error)
}
```

### Step 1.3 — Provider registry (`pkg/git/registry.go`)
A simple `map[string]GitProvider` keyed by provider name (`"github"`, `"bitbucket"`), with a constructor function.

**Verification:** `go build ./...` compiles, `go vet ./...` passes.

---

## Phase 2: GitHub Provider & OAuth

**Goal:** Implement the GitHub provider and the full OAuth2 login flow.

### Step 2.1 — GitHub OAuth handler (`pkg/server/oauth.go`)
- Use `golang.org/x/oauth2` with `github.Endpoint`
- Scopes: `repo` (read/write repos), `read:user`
- Endpoints: `GET /auth/github/login` → redirect to GitHub; `GET /auth/github/callback` → exchange code for token
- **OAuth state parameter:** Generate crypto/rand state string, store in short-lived cookie (5 min TTL), pass as `state` param, verify on callback. Reject if mismatch.
- Store session in server-side in-memory map. Cookie contains only the session ID (random, HttpOnly, Secure, SameSite=Lax).
- `GET /auth/logout` — delete session from map, clear cookie
- **Constraint:** Never persist tokens to disk. In-memory only, lost on restart (acceptable for short-lived OAuth tokens).

### Step 2.2 — GitHub GitProvider (`pkg/git/github.go`)
- Import `github.com/google/go-github/v66/github` (pin a stable recent version)
- Construct client per-request: `github.NewClient(github.WithHTTPClient(oauth2Client))`
- Implement all `GitProvider` methods:
  - `GetRepositories` → `client.Repositories.ListByAuthenticatedUser(ctx, opts)` with pagination
  - `ListForms` → `client.Repositories.GetContents(ctx, owner, repo, ".courtyard/forms", nil)` — list YAML files
  - `GetFormConfig` → fetch `.courtyard/forms/{formName}.yaml`, decode YAML
  - `GetTemplateFiles` → Use Git Trees API: `client.Git.GetTree(ctx, owner, repo, "HEAD:.courtyard/templates/{formName}", true)` to get full recursive tree in one call, then `client.Git.GetBlob()` for each file. Reject blobs > 1MB.
  - `CreateBranchAndPullRequest`:
    1. Get base branch SHA via `client.Git.GetRef`
    2. Check if target branch exists (for `branchMode` handling)
    3. If `reuse` and branch exists: get current branch tip SHA as parent
    4. If `fresh` and branch exists: return error with suggestion
    5. Create/update ref via `client.Git.CreateRef` or `client.Git.UpdateRef`
    6. Build tree entries for all `OutputFile`s via `client.Git.CreateTree`
    7. Create commit (parent = branch tip for reuse, or base for fresh) via `client.Git.CreateCommit`
    8. Update ref to new commit via `client.Git.UpdateRef`
    9. Create PR via `client.PullRequests.Create` (skip if PR already exists for reuse mode)

### Step 2.3 — Auth middleware (`pkg/server/middleware.go`)
- Extract token from session cookie on every `/api/*` request
- If missing/expired → return 401
- Inject token into `context.Context` for downstream handlers

**Verification:** Unit tests with mocked HTTP responses (use `httptest.NewServer`). Integration test: manually verify OAuth login against a real GitHub OAuth App (dev credentials via env vars).

---

## Phase 3: Bitbucket Cloud Provider *(parallel with Phase 4)*

**Goal:** Implement the Bitbucket Cloud provider using raw REST calls. Bitbucket Server (self-hosted) is explicitly out of scope.

### Step 3.1 — Bitbucket OAuth handler (`pkg/server/oauth.go`)
- Separate OAuth2 config with Bitbucket endpoints:
  - AuthURL: `https://bitbucket.org/site/oauth2/authorize`
  - TokenURL: `https://bitbucket.org/site/oauth2/access_token`
- Scopes: `repository`, `repository:write`, `pullrequest`, `pullrequest:write`
- Tokens expire in 1 hour — store refresh_token in the cookie, refresh transparently
- Endpoints: `GET /auth/bitbucket/login`, `GET /auth/bitbucket/callback`

### Step 3.2 — Bitbucket GitProvider (`pkg/git/bitbucket.go`)
- Base URL: `https://api.bitbucket.org/2.0`
- Auth: `Authorization: Bearer {token}` header
- **Workspace aggregation:** `GetRepositories` first calls `GET /workspaces?role=member` to list all workspaces the user belongs to, then aggregates repos across all of them via `GET /repositories/{workspace}` with pagination.
- Implement all `GitProvider` methods using `net/http`:
  - `GetRepositories` → aggregate across all user workspaces (see above)
  - `ListForms` → `GET /repositories/{workspace}/{repo}/src/{branch}/.courtyard/forms/`
  - `GetFormConfig` → `GET /repositories/{workspace}/{repo}/src/{branch}/.courtyard/forms/{name}.yaml`
  - `GetTemplateFiles` → list + fetch from `/src/{branch}/.courtyard/templates/{formName}/`
  - `CreateBranchAndPullRequest`:
    1. `POST /refs/branches` to create branch
    2. `POST /src` with `multipart/form-data` to commit files (Bitbucket has no tree API)
    3. `POST /pullrequests` to open PR

**Verification:** Unit tests with `httptest.NewServer` mocking Bitbucket responses. Test pagination and workspace aggregation.

---

## Phase 4: Template Engine

**Goal:** Parse `.courtyard/forms/*.yaml` schemas and evaluate `.courtyard/templates/**/*.tmpl` files with user input + Sprig functions.

### Step 4.1 — Form schema parser (`pkg/engine/parser.go`)
- Define `FormSpec` struct for the YAML form definition files (custom format, NOT JSON Schema):
  ```yaml
  # Example: .courtyard/forms/deploy-app.yaml
  name: deploy-app
  description: Deploy a new application service
  targetBranch: main                                    # base branch for PR
  branchName: "courtyard/{{ .appName }}/{{ .environment }}"  # Go template for PR branch name
  branchMode: reuse                                     # "reuse" (default) or "fresh"
  commitMessage: "feat({{ .team }}): deploy {{ .appName }} to {{ .environment }}"
  outputPath: "clusters/genieo/us-east-1/{{ .environment }}/{{ .namespace }}"  # default output dir

  fields:
    - name: team
      type: enum
      label: Team
      options: [devops, platform, payments]
      required: true
    - name: appName
      type: string
      label: Application Name
      required: true
      validation: "^[a-z][a-z0-9-]*$"
    - name: environment
      type: enum
      label: Environment
      options: [dev, staging, prod]
      required: true
    - name: namespace
      type: string
      label: Kubernetes Namespace
      required: true
    - name: replicas
      type: number
      label: Replica Count
      default: 3
    - name: enableIngress
      type: boolean
      label: Enable Ingress
      default: false

  # Per-file output path overrides (optional — defaults to outputPath + filename sans .tmpl)
  templates:
    deployment.yaml.tmpl: {}                                         # uses default outputPath
    values.tf.tmpl:
      outputPath: "terraform/{{ .environment }}/{{ .appName }}.tf"   # override
  ```
- Go struct `FormSpec`:
  - `Name`, `Description` string
  - `TargetBranch` string (default: repo's default branch)
  - `BranchName` string (Go template for the PR branch name)
  - `BranchMode` string (`"reuse"` | `"fresh"`, default `"reuse"`)
  - `CommitMessage` string (Go template)
  - `OutputPath` string (Go template — default output directory)
  - `Fields []FieldSpec` — custom field definitions
  - `Templates map[string]TemplateOverride` — per-file output path overrides
- `FieldSpec` struct: `Name`, `Type` (string/number/boolean/enum), `Label`, `Required`, `Default`, `Options []string`, `Validation` (regex)
- `TemplateOverride` struct: `OutputPath` string (optional Go template)
- Parse with `gopkg.in/yaml.v3`

### Step 4.2 — Template evaluator (`pkg/engine/evaluator.go`)
- `func Evaluate(spec *FormSpec, templateFiles map[string][]byte, values map[string]interface{}) ([]OutputFile, error)`
- For each template file:
  1. Resolve output path: evaluate per-file `TemplateOverride.OutputPath` if set, else combine `spec.OutputPath` + relative path (sans `.tmpl`)
  2. Create `text/template.New(name).Funcs(sprig.TxtFuncMap()).Parse(content)`
  3. Execute template with `values`
  4. Collect into `[]OutputFile` (with resolved output path)
- Return all errors aggregated (don't fail on first error)
- Also provide helpers to evaluate `spec.BranchName` and `spec.CommitMessage` as Go templates with `values`

### Step 4.3 — Preview endpoint support
- Same `Evaluate` function is used for both preview and commit flows
- Preview returns the rendered files as JSON; commit sends them to `CreateBranchAndPullRequest`

**Verification:**
- Unit test: provide a map of template files and values, assert correct output
- Unit test: malformed template returns error, does not panic
- Unit test: Sprig functions (`upper`, `default`, `toYaml`, etc.) work in templates

---

## Phase 5: HTTP API & Server

**Goal:** Wire everything together with a clean HTTP API.

### Step 5.1 — Router (`pkg/server/router.go`)
Use `net/http` stdlib mux (Go 1.22+ enhanced routing):

| Method | Path | Handler | Auth |
|--------|------|---------|------|
| GET | `/healthz` | Health check | No |
| GET | `/auth/{provider}/login` | Start OAuth | No |
| GET | `/auth/{provider}/callback` | OAuth callback | No |
| GET | `/auth/logout` | Clear session | No |
| GET | `/api/me` | Return current user info | Yes |
| GET | `/api/repos` | List repos | Yes |
| GET | `/api/repos/{owner}/{repo}/forms` | List available forms | Yes |
| GET | `/api/repos/{owner}/{repo}/forms/{form}` | Get form schema | Yes |
| POST | `/api/repos/{owner}/{repo}/forms/{form}/preview` | Evaluate templates, return preview | Yes |
| POST | `/api/repos/{owner}/{repo}/forms/{form}/submit` | Evaluate + create PR | Yes |
| GET | `/*` | Serve embedded UI | No |

### Step 5.2 — Handlers (`pkg/server/handlers.go`)
Each handler:
1. Extract token from context
2. Determine provider from session/cookie
3. Call `GitProvider` method
4. Return JSON response

### Step 5.3 — Configuration (`cmd/courtyard/main.go`)
- All config via environment variables:
  - `COURTYARD_ADDR` (default `:8080`)
  - `GITHUB_CLIENT_ID`, `GITHUB_CLIENT_SECRET`
  - `BITBUCKET_CLIENT_ID`, `BITBUCKET_CLIENT_SECRET`
  - `COURTYARD_COOKIE_KEY` (32-byte key for cookie signing)
  - `COURTYARD_BASE_URL` (for OAuth redirect URLs)
- No config files

**Verification:** `go build ./cmd/courtyard` succeeds. Manual smoke test of full flow.

---

## Phase 6: Frontend (Embedded UI)

**Goal:** Lightweight SPA that renders YAML-defined forms and shows live preview.

### Step 6.1 — HTML shell (`ui/index.html`)
- Single HTML file, no build step
- Include Alpine.js (CDN or vendored) for reactivity
- Custom form renderer that reads the YAML-defined schema and generates fields (string, number, boolean, enum) — no React, no `@rjsf/core`

### Step 6.2 — App logic (`ui/static/app.js`)
- On load: `GET /api/me` → show user info or redirect to login
- Repo selector: `GET /api/repos` → dropdown
- Form selector: `GET /api/repos/{owner}/{repo}/forms` → dropdown
- Form renderer: `GET /api/repos/{owner}/{repo}/forms/{form}` → dynamically render fields from YAML schema
- Live preview: on form change (debounced 500ms), `POST /api/repos/{owner}/{repo}/forms/{form}/preview` → show rendered YAML/HCL in `<pre>` blocks
- Submit: `POST /api/repos/{owner}/{repo}/forms/{form}/submit` → show PR link on success

### Step 6.3 — Embed in binary (`cmd/courtyard/main.go`)
```go
//go:embed all:ui
var uiFS embed.FS
```
- Use `io/fs.Sub(uiFS, "ui")` + `http.FileServer` to serve at `/`
- API routes take precedence over static file serving

### Step 6.4 — Styling (`ui/static/style.css`)
- Minimal, clean CSS (no framework needed)
- Two-panel layout: form on left, preview on right
- Mobile-responsive

**Verification:** Open browser, complete full flow: login → select repo → select form → fill fields → see preview → submit → PR link shown.

---

## Phase 7: Packaging & Hardening

### Step 7.1 — Dockerfile
```dockerfile
FROM golang:1.23-alpine AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -o /courtyard ./cmd/courtyard

FROM gcr.io/distroless/static
COPY --from=build /courtyard /courtyard
ENTRYPOINT ["/courtyard"]
```

### Step 7.2 — Security hardening
- CSRF protection on all POST endpoints (SameSite cookie + custom header check)
- Rate limiting on OAuth endpoints
- Input validation: reject form payloads > 1MB
- Template evaluation timeout (5s context deadline)
- Cookie: `Secure`, `HttpOnly`, `SameSite=Lax`

### Step 7.3 — Graceful shutdown
- `signal.NotifyContext` for SIGTERM/SIGINT
- `http.Server.Shutdown(ctx)` with 10s timeout

**Verification:** `docker build -t courtyard .` succeeds. `docker run -p 8080:8080 courtyard` starts and serves `/healthz`.

---

## Relevant Files

| File | Purpose |
|------|---------|
| `cmd/courtyard/main.go` | Entry point: config loading, server init, embed UI, graceful shutdown |
| `pkg/git/models.go` | Domain types: `Repository`, `FormSpec`, `OutputFile` |
| `pkg/git/provider.go` | `GitProvider` interface definition |
| `pkg/git/registry.go` | Provider registry (map of name → GitProvider) |
| `pkg/git/github.go` | GitHub `GitProvider` implementation using `go-github` |
| `pkg/git/bitbucket.go` | Bitbucket Cloud `GitProvider` implementation using `net/http` |
| `pkg/engine/parser.go` | YAML form spec parser, `FormSpec` struct |
| `pkg/engine/evaluator.go` | Go template evaluator with Sprig functions |
| `pkg/engine/validator.go` | Server-side field value validation |
| `pkg/server/router.go` | HTTP route definitions |
| `pkg/server/handlers.go` | API handler functions |
| `pkg/server/middleware.go` | Auth middleware (token extraction from session) |
| `pkg/server/oauth.go` | OAuth2 flow handlers for GitHub and Bitbucket |
| `pkg/server/session.go` | In-memory session store (TTL, sweep goroutine) |
| `ui/index.html` | SPA shell |
| `ui/static/app.js` | Frontend logic (Alpine.js + YAML-driven form rendering) |
| `ui/static/style.css` | Minimal CSS |
| `Dockerfile` | Multi-stage build → distroless |
| `go.mod` | Module definition |

## Key Dependencies

| Package | Version | Purpose |
|---------|---------|---------|
| `github.com/google/go-github/v66` | v66.x | GitHub API client |
| `github.com/Masterminds/sprig/v3` | v3.3.0 | Template functions |
| `golang.org/x/oauth2` | v0.36.0 | OAuth2 flow |
| `gopkg.in/yaml.v3` | v3.0.1 | YAML parsing |

---

## Verification (End-to-End)

1. `go vet ./...` — no issues
2. `go test ./...` — all unit tests pass
3. `go build -o courtyard ./cmd/courtyard` — single binary produced
4. `docker build -t courtyard .` — image builds
5. Manual: set GitHub OAuth env vars → login → pick repo with `.courtyard/` dir → fill form → preview renders correctly → submit creates PR with correct files

---

## Decisions & Constraints

- **No database:** In-memory session map. No Postgres/MySQL/Redis.
- **No admin tokens:** Every API call uses the end-user's OAuth token from the session.
- **Git is the only mutation:** The only write operation is `CreateBranchAndPullRequest`. No cluster writes.
- **Single commit per submission:** All template output files are packed into one Git commit on the new branch.
- **No frontend build step:** UI is plain HTML/JS/CSS served from `embed.FS`. No webpack, no npm.
- **Go 1.22+ required:** For enhanced `net/http` mux routing (`GET /path/{param}`).
- **Provider determined at login time:** The session cookie records which provider the user authenticated with.

## Decisions (Confirmed)

1. **Form renderer:** Custom Alpine.js renderer driven by the YAML schema file. No React, no `@rjsf/core`.
2. **Bitbucket scope:** Cloud only. Bitbucket Server is explicitly out of scope.
3. **Bitbucket workspaces:** List all workspaces via `GET /workspaces?role=member` and aggregate repos.
4. **targetRepo (cross-repo PRs):** Deferred — v1 always targets the same repo where `.courtyard/` lives.
5. **Branch naming:** Templated `branchName` field in FormSpec (Go template), e.g. `"courtyard/{{ .appName }}/{{ .environment }}"`.
6. **branchMode: reuse:** Appends a new commit on top of the existing branch. Creates it if new.
7. **branchMode: fresh:** Errors if the templated branch name already exists. Error message suggests changing values or switching to `reuse`.
8. **Backend validation:** Server-side validation of all field values (required, type, regex, enum) before template evaluation.
9. **Nested template directories:** Supported. Output path preserves relative directory structure under `outputPath`.
10. **OAuth state parameter:** Generate random state, store in short-lived cookie (5 min TTL), verify on callback.
11. **Session storage:** Server-side in-memory map keyed by random session ID in HttpOnly cookie. 8h TTL, sweep every 15 min.
12. **Preview vs submit:** Preview is advisory. Submit always re-fetches templates fresh from Git.
13. **GitHub API strategy:** Git Trees API (`recursive: true`) for `GetTemplateFiles`. Single call + blob fetches.
14. **Template size limit:** Reject individual template files > 1MB.
15. **Multi-provider:** Single provider per session for v1.

## Scope Exclusions (Explicit)

- Cross-repo PRs (`targetRepo` field) — deferred
- Bitbucket Server / Data Center — excluded
- Multi-provider simultaneous sessions — deferred
- Horizontal scaling / shared session store — out of scope (single binary)
