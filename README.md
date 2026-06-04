# Courtyard

A stateless, single-binary GitOps proxy. Reads `.courtyard/` specs from Git repos, renders dynamic forms from YAML-defined schemas, evaluates Go templates with user input + Sprig functions, and opens Pull Requests using the logged-in user's own OAuth token.

No database. No admin tokens. No cluster writes.

---

## How it works

```
User fills form → Courtyard evaluates templates → PR opened in Git (as the user)
```

1. Sign in with GitHub or Bitbucket — Courtyard stores only a short-lived session (8h, in-memory).
2. Pick a repo that contains a `.courtyard/` directory.
3. Pick a form — Courtyard fetches and renders the YAML-defined field schema.
4. Fill in the fields — a live preview shows the rendered output files.
5. Submit — Courtyard commits all output files to a new branch and opens a PR on your behalf.

---

## Quick start (Docker Compose)

```sh
# 1. Create a GitHub OAuth App
#    Homepage URL:      http://localhost:8080
#    Callback URL:      http://localhost:8080/auth/github/callback
#    https://github.com/settings/developers

# 2. Configure credentials
cp .env.example .env
# Edit .env — set GITHUB_CLIENT_ID and GITHUB_CLIENT_SECRET

# 3. Start
docker compose up --build

# 4. Open
open http://localhost:8080
```

---

## Quick start (binary)

```sh
# Build
go build -o courtyard ./cmd/courtyard

# Run
export GITHUB_CLIENT_ID=xxx
export GITHUB_CLIENT_SECRET=yyy
export COURTYARD_BASE_URL=http://localhost:8080
./courtyard
```

Health check: `curl http://localhost:8080/healthz`

---

## Adding Courtyard to a repo

Create a `.courtyard/` directory in any GitHub or Bitbucket repo:

```
.courtyard/
  forms/
    my-form.yaml          ← field schema + metadata
  templates/
    my-form/
      output.yaml.tmpl    ← Go template (Sprig functions available)
```

### Form schema (`forms/my-form.yaml`)

```yaml
name: deploy-service
description: Deploy a service to Kubernetes
targetBranch: main
branchName: "courtyard/{{ .team }}/{{ .appName }}/{{ .environment }}"
branchMode: reuse          # reuse (default) | fresh
commitMessage: "feat({{ .team }}): deploy {{ .appName }} to {{ .environment }}"
outputPath: "clusters/{{ .environment }}/{{ .namespace }}"

fields:
  - name: appName
    type: string           # string | number | boolean | enum
    label: Application Name
    required: true
    validation: "^[a-z][a-z0-9-]+$"   # regex (string fields only)

  - name: environment
    type: enum
    label: Environment
    options: [dev, staging, prod]
    required: true

  - name: replicas
    type: number
    label: Replica Count
    default: 2

  - name: enableIngress
    type: boolean
    label: Enable Ingress
    default: false

# Per-file output path overrides (optional)
templates:
  deployment.yaml.tmpl: {}                          # uses outputPath above
  terraform.tf.tmpl:
    outputPath: "terraform/{{ .environment }}.tf"   # custom path
```

### Template files (`templates/my-form/*.tmpl`)

Standard Go `text/template` syntax with all [Sprig functions](https://masterminds.github.io/sprig/) available:

```yaml
# deployment.yaml.tmpl
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ .appName }}
  namespace: {{ .namespace }}
spec:
  replicas: {{ .replicas | default 2 }}
  template:
    spec:
      containers:
        - name: {{ .appName }}
          image: {{ .image }}:{{ .imageTag | default "latest" }}
{{ if .enableIngress }}
---
# ingress rendered only when enableIngress is true
{{ end }}
```

An example form + templates is in the [`example/`](example/) directory.

---

## Branch modes

| `branchMode` | Behaviour |
|---|---|
| `reuse` (default) | Creates the branch if new, otherwise appends a commit to the existing branch. Good for iterative deployments. |
| `fresh` | Errors if the templated branch name already exists. Forces unique branches per submission. |

---

## API

All API routes require an active session cookie (obtained via OAuth login).  
POST routes also require the `X-Requested-With: XMLHttpRequest` header (CSRF protection).

| Method | Path | Description |
|---|---|---|
| `GET` | `/healthz` | Liveness check — returns `{"status":"ok"}` |
| `GET` | `/auth/github/login` | Start GitHub OAuth flow |
| `GET` | `/auth/github/callback` | GitHub OAuth callback |
| `GET` | `/auth/bitbucket/login` | Start Bitbucket OAuth flow |
| `GET` | `/auth/bitbucket/callback` | Bitbucket OAuth callback |
| `GET` | `/auth/logout` | Clear session |
| `GET` | `/api/me` | Current user's provider name |
| `GET` | `/api/repos` | List accessible repositories |
| `GET` | `/api/repos/{owner}/{repo}/forms` | List available forms in a repo |
| `GET` | `/api/repos/{owner}/{repo}/forms/{form}` | Get parsed form schema |
| `POST` | `/api/repos/{owner}/{repo}/forms/{form}/preview` | Render templates (no commit) |
| `POST` | `/api/repos/{owner}/{repo}/forms/{form}/submit` | Render + commit + open PR |

---

## Configuration

All configuration is via environment variables — no config files.

| Variable | Default | Description |
|---|---|---|
| `COURTYARD_ADDR` | `:8080` | TCP listen address |
| `COURTYARD_BASE_URL` | — | Public URL (used for OAuth redirect URIs) |
| `GITHUB_CLIENT_ID` | — | GitHub OAuth App client ID |
| `GITHUB_CLIENT_SECRET` | — | GitHub OAuth App client secret |
| `BITBUCKET_CLIENT_ID` | — | Bitbucket OAuth consumer key |
| `BITBUCKET_CLIENT_SECRET` | — | Bitbucket OAuth consumer secret |

---

## Security

- **No admin tokens.** Every Git API call uses the end-user's OAuth token.
- **Session cookies** — `HttpOnly`, `SameSite=Lax`, 8h TTL, in-memory only (lost on restart).
- **CSRF** — Custom header (`X-Requested-With`) required on all mutating API calls.
- **Rate limiting** — OAuth endpoints rate-limited to 20 req/min per IP.
- **Template timeout** — Template evaluation times out after 5 seconds.
- **Blob size limit** — Template files > 1 MB are rejected.
- **Distroless runtime** — Container image has no shell or package manager.

---

## Development

```sh
go build ./cmd/courtyard   # build binary
go test ./...              # run all tests
go vet ./...               # static analysis
docker build -t courtyard . # build container image
```

Requires Go 1.22+ (for enhanced `net/http` mux routing).

---

## Architecture

```
cmd/courtyard/main.go    → entry point, config, embed UI, graceful shutdown
pkg/git/                 → GitProvider interface + GitHub/Bitbucket implementations
pkg/engine/              → YAML form parser, Go template evaluator, field validator
pkg/server/              → HTTP router, handlers, OAuth flows, session store, middleware
ui/                      → Embedded SPA (Alpine.js, no build step)
```

Stateless by design. Horizontal scaling is possible — sessions are in-memory and lost on restart (acceptable; OAuth tokens are short-lived).
