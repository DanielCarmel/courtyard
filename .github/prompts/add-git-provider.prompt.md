---
description: "Scaffold a new Git provider implementation for Courtyard. Creates the provider file, implements the GitProvider interface, adds OAuth config, and registers it."
agent: "courtyard"
argument-hint: "Provider name (e.g., gitlab, gitea)"
---
# Add Git Provider

Implement a new Git provider for Courtyard. This involves:

## Steps

1. **Create provider file** at `pkg/git/{name}.go`
   - Implement the full `GitProvider` interface
   - Accept user token per-method (never store/cache)
   - Handle pagination for list operations
   - Use `net/http` directly (no third-party client unless one exists)

2. **Add OAuth configuration** in `pkg/server/oauth.go`
   - Add OAuth2 endpoints (authorize URL, token URL)
   - Add required scopes
   - Handle token refresh if tokens are short-lived
   - Add routes: `GET /auth/{provider}/login`, `GET /auth/{provider}/callback`

3. **Register the provider** in `pkg/git/registry.go`
   - Add to the provider map with the provider name as key

4. **Add environment variables** for the new provider
   - `{PROVIDER}_CLIENT_ID`
   - `{PROVIDER}_CLIENT_SECRET`
   - Update `cmd/courtyard/main.go` to read them

5. **Write tests** at `pkg/git/{name}_test.go`
   - Mock API responses with `httptest.NewServer`
   - Test all GitProvider methods
   - Test pagination handling
   - Test error cases (401, 404, 422)

6. **Verify**: `go build ./... && go vet ./... && go test ./...`

## Interface Reference

```go
type GitProvider interface {
    GetRepositories(ctx context.Context, token string) ([]Repository, error)
    GetFormConfig(ctx context.Context, token string, owner string, repo string, formName string) (*FormConfig, error)
    ListForms(ctx context.Context, token string, owner string, repo string) ([]string, error)
    GetTemplateFiles(ctx context.Context, token string, owner string, repo string, formName string) (map[string][]byte, error)
    CreateBranchAndPullRequest(ctx context.Context, token string, owner string, repo string, baseBranch string, branchName string, commitMessage string, files []OutputFile) (prURL string, err error)
}
```

## Constraints

- No admin tokens — every call uses the end-user's token
- Handle paginated responses — never assume single page
- Create API client per-request — never cache clients
- Single commit per submission in CreateBranchAndPullRequest
