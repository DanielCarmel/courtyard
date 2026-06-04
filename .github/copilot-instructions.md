# Courtyard — Project Guidelines

## What This Is

Media Luna Courtyard: a stateless, single-binary, open-source GitOps proxy. Reads `.courtyard/` specs from Git repos, renders dynamic forms from YAML-defined schemas, evaluates Go templates with user input + Sprig functions, and opens Pull Requests using the logged-in user's OAuth tokens.

## Architecture

```
cmd/courtyard/main.go       → Entry point, config, embed UI, graceful shutdown
pkg/git/                     → GitProvider interface + GitHub/Bitbucket implementations
pkg/engine/                  → YAML form parser + Go template evaluator
pkg/server/                  → HTTP router, handlers, OAuth flow, auth middleware
ui/                          → Embedded frontend (Alpine.js, no build step)
```

## Non-Negotiable Constraints

1. **No database.** In-memory session map only. No Postgres, MySQL, Redis, SQLite.
2. **No admin tokens.** Every Git API call uses the end-user's OAuth token.
3. **Git-only mutation.** The only write operation is creating a branch + PR. No cluster writes.
4. **Single commit per submission.** All output files packed into one Git commit.
5. **No frontend build step.** UI is plain HTML/JS/CSS served from `embed.FS`.
6. **Stateless binary.** Server restart loses sessions (acceptable — tokens are short-lived).

## Build & Test

```sh
go build ./cmd/courtyard          # Build binary
go test ./...                      # Run all tests
go vet ./...                       # Static analysis
docker build -t courtyard .        # Container image
```

## Key Dependencies

| Package | Import Path | Purpose |
|---------|-------------|---------|
| go-github | `github.com/google/go-github/v66/github` | GitHub API client |
| Sprig | `github.com/Masterminds/sprig/v3` | Template functions |
| oauth2 | `golang.org/x/oauth2` | OAuth2 flow |
| yaml.v3 | `gopkg.in/yaml.v3` | YAML parsing |

## Conventions

- Go 1.22+ required (enhanced `net/http` mux routing with `{param}` syntax)
- All config via environment variables — no config files
- `context.Context` as first parameter on all functions that do I/O
- Errors wrapped with `fmt.Errorf("methodName: %w", err)`
- Table-driven tests with `t.Run()`
- No `init()` functions, no global mutable state
- Interfaces: accept interfaces, return concrete types

## Environment Variables

| Variable | Default | Purpose |
|----------|---------|---------|
| `COURTYARD_ADDR` | `:8080` | Listen address |
| `COURTYARD_BASE_URL` | — | Public URL for OAuth redirects |
| `COURTYARD_COOKIE_KEY` | — | 32-byte hex key for session ID signing |
| `GITHUB_CLIENT_ID` | — | GitHub OAuth App client ID |
| `GITHUB_CLIENT_SECRET` | — | GitHub OAuth App client secret |
| `BITBUCKET_CLIENT_ID` | — | Bitbucket OAuth consumer key |
| `BITBUCKET_CLIENT_SECRET` | — | Bitbucket OAuth consumer secret |
