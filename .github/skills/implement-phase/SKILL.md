---
name: implement-phase
description: "Implement a specific phase of the Courtyard project plan. Use when building Phase 0 through Phase 7. Reads the plan, implements the code for that phase, writes tests, and runs verification. Invoke with the phase number."
argument-hint: "Phase number (0-7) and optional specific step"
---
# Implement Phase

## Purpose

Implements one phase (or step within a phase) of the Courtyard project according to the implementation plan.

## Procedure

### 1. Identify the Phase

The user specifies a phase number (0-7) and optionally a specific step. Map to:
- **Phase 0**: Project scaffolding (go mod, directory tree, main.go, Dockerfile)
- **Phase 1**: Core interfaces & models (GitProvider, domain types, registry)
- **Phase 2**: GitHub provider + OAuth (oauth.go, github.go, middleware.go)
- **Phase 3**: Bitbucket Cloud provider (bitbucket.go, workspace aggregation)
- **Phase 4**: Template engine (parser.go, evaluator.go, validation)
- **Phase 5**: HTTP API server (router.go, handlers.go, wiring)
- **Phase 6**: Frontend (index.html, app.js, style.css, embed)
- **Phase 7**: Packaging & hardening (Dockerfile, CSRF, rate limit, shutdown)

### 2. Check Prerequisites

Before implementing a phase, verify:
- Previous phases are complete (code exists and compiles)
- Dependencies are available in `go.mod`
- Required interfaces/types from prior phases exist

### 3. Implement

Follow the plan exactly for the specified phase. Key rules:
- Create files in the correct locations per project structure
- Use the exact interface signatures from the plan
- Follow Go conventions from [go-patterns instructions](../../instructions/go-patterns.instructions.md)
- Never violate architectural constraints (no DB, no admin tokens, etc.)

### 4. Write Tests

Every implementation file gets a corresponding `_test.go`:
- Table-driven tests with `t.Run()`
- Mock external HTTP APIs with `httptest.NewServer`
- Test both success and error paths
- Test edge cases (empty inputs, pagination boundaries, malformed data)

### 5. Verify

Run these commands and fix any issues:
```sh
go build ./cmd/courtyard
go vet ./...
go test ./...
```

### 6. Report

After successful verification, report:
- Files created/modified
- Test coverage summary
- Any decisions made that weren't explicitly in the plan
- What the next phase depends on from this one

## Plan Reference

Full plan: [.github/PLAN.md](../../PLAN.md)

## Phase Details Reference

### Phase 0 — Scaffolding
```sh
go mod init github.com/media-luna/courtyard
```
Create: `cmd/courtyard/main.go` (HTTP server + `/healthz`), `Dockerfile`, `.gitignore`

### Phase 1 — Models & Interfaces
- `pkg/git/models.go`: Repository, OutputFile, FormSpec structs
- `pkg/git/provider.go`: GitProvider interface
- `pkg/git/registry.go`: Provider map

### Phase 2 — GitHub + OAuth
- `pkg/server/session.go`: In-memory session store (sync.RWMutex, TTL, sweep goroutine)
- `pkg/server/oauth.go`: GitHub OAuth flow (/auth/github/login, /auth/github/callback)
- `pkg/git/github.go`: Full GitProvider implementation
- `pkg/server/middleware.go`: Auth middleware (extract token from session)

### Phase 3 — Bitbucket
- `pkg/server/oauth.go`: Add Bitbucket OAuth config + endpoints
- `pkg/git/bitbucket.go`: Full GitProvider implementation (REST, workspace aggregation)

### Phase 4 — Engine
- `pkg/engine/parser.go`: FormSpec YAML parser with validation
- `pkg/engine/evaluator.go`: Template evaluation with Sprig + output path resolution
- `pkg/engine/validator.go`: Server-side field value validation

### Phase 5 — Server
- `pkg/server/router.go`: Route registration (Go 1.22+ mux)
- `pkg/server/handlers.go`: All API handlers (repos, forms, preview, submit)

### Phase 6 — Frontend
- `ui/index.html`: Alpine.js SPA shell
- `ui/static/app.js`: Form rendering, preview, submit
- `ui/static/style.css`: Two-panel layout
- Update `cmd/courtyard/main.go`: Add `//go:embed all:ui`

### Phase 7 — Hardening
- Update Dockerfile for distroless
- Add CSRF protection (custom header check)
- Add request body size limiting (1MB)
- Add graceful shutdown (signal.NotifyContext)
- Add template evaluation timeout (5s)
