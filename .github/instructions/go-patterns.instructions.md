---
description: "Go code patterns for Courtyard. Use when writing or modifying Go source files. Covers error handling, context usage, testing, and package conventions."
applyTo: "**/*.go"
---
# Go Patterns

## Error Handling
- Wrap errors with context: `fmt.Errorf("GetRepositories: %w", err)`
- Never use `panic()` in HTTP handlers or library code
- Return errors to callers; let the handler layer decide the HTTP status
- Use `errors.Is()` / `errors.As()` for error inspection

## Context
- First parameter is always `ctx context.Context` for any function doing I/O
- Use `context.WithTimeout` for external API calls (30s default for Git APIs)
- Use `context.WithTimeout` of 5s for template evaluation
- Check `ctx.Err()` before expensive operations

## Interfaces
- Accept interfaces, return concrete structs
- Define interfaces where they are consumed (`pkg/server/` defines what it needs from `pkg/git/`)
- Keep interfaces small — prefer 1-3 methods per interface

## Testing
- Table-driven tests with `t.Run(name, func(t *testing.T) {...})`
- Use `httptest.NewServer` for mocking HTTP APIs
- Test file lives next to implementation: `github.go` → `github_test.go`
- Use `t.Helper()` in test helper functions
- No test frameworks — stdlib `testing` package only

## Naming
- Package names: short, lowercase, no underscores (`git`, `engine`, `server`)
- Exported types: `GitProvider`, `FormSpec`, `OutputFile`
- Unexported helpers: `doRequest`, `parseResponse`
- Constructor functions: `NewGitHubProvider(...)`, not `CreateGitHubProvider`

## HTTP Handlers
- Signature: `func(w http.ResponseWriter, r *http.Request)`
- Extract path params with `r.PathValue("param")` (Go 1.22+)
- Always set `Content-Type` header before writing response body
- Use `http.Error()` for error responses, JSON for success
- Decode request body with `json.NewDecoder(r.Body).Decode(&v)` — limit body size first

## Concurrency
- No goroutines in request handlers unless explicitly needed
- Session map protected by `sync.RWMutex`
- Never hold locks across I/O calls
