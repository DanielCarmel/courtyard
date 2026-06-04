---
description: "Git provider implementation patterns. Use when implementing or modifying GitHub/Bitbucket provider code in pkg/git/. Covers the GitProvider interface contract, pagination, token handling, and API client construction."
applyTo: "pkg/git/**"
---
# Git Provider Implementation

## Interface Contract

Every provider must implement `GitProvider` exactly. All methods:
- Accept `ctx context.Context` as first param
- Accept `token string` — the end-user's OAuth access token
- Never cache or store tokens beyond the request scope
- Never use app-level or admin tokens

## Client Construction

- Create a new API client per request — never reuse or cache clients
- For GitHub: `github.NewClient(github.WithHTTPClient(oauth2HttpClient))`
- For Bitbucket: create a new `*http.Client` with Bearer token per request

## Pagination

- Always handle paginated API responses — never assume single page
- GitHub: check `resp.NextPage != 0`, loop until exhausted
- Bitbucket: follow the `next` URL field in response JSON until absent
- Set reasonable page size (100 per page)

## Error Handling

- Wrap API errors with provider + method context:
  `fmt.Errorf("github.GetRepositories: %w", err)`
- Translate HTTP status codes to meaningful errors:
  - 401 → user token expired/invalid
  - 403 → insufficient permissions
  - 404 → resource not found (repo, file, branch)
  - 422 → validation error (branch already exists, etc.)
- Return structured error types where the handler needs to distinguish cases

## GitHub-Specific

- Use Git Trees API (`recursive: true`) for `GetTemplateFiles` — one call for the full tree
- Use `client.Git.GetBlob()` for individual file content after getting tree
- Reject blobs > 1MB
- For `CreateBranchAndPullRequest`:
  1. `GetRef` → get base branch SHA
  2. Handle `branchMode`: check if branch exists, reuse or error accordingly
  3. `CreateTree` → all output files in one tree
  4. `CreateCommit` → single commit with all changes
  5. `UpdateRef` or `CreateRef` → point branch at new commit
  6. `PullRequests.Create` → open PR (skip if reuse mode and PR already open)

## Bitbucket-Specific

- Base URL: `https://api.bitbucket.org/2.0`
- Workspace aggregation: call `GET /workspaces?role=member` first, then aggregate repos across all workspaces
- File commits: use `POST /repositories/{workspace}/{repo}/src` with `multipart/form-data`
- No tree API — commit files directly in one request
- Tokens expire in 1 hour — refresh transparently using refresh_token

## Testing

- Mock with `httptest.NewServer` — return canned JSON responses
- Test pagination (multi-page responses)
- Test error cases (401, 404, network errors)
- Test the full `CreateBranchAndPullRequest` flow with mock responses for each step
