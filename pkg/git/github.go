package git

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/google/go-github/v66/github"
	"golang.org/x/oauth2"
)

const maxTemplateBlobSize = 1 * 1024 * 1024 // 1 MB

// GitHubProvider implements GitProvider using the GitHub REST API.
type GitHubProvider struct{}

// NewGitHubProvider returns a new GitHubProvider.
func NewGitHubProvider() *GitHubProvider {
	return &GitHubProvider{}
}

func (g *GitHubProvider) newClient(ctx context.Context, token string) *github.Client {
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	httpClient := oauth2.NewClient(ctx, ts)
	return github.NewClient(httpClient)
}

// GetRepositories returns all repositories accessible to the authenticated user.
func (g *GitHubProvider) GetRepositories(ctx context.Context, token string) ([]Repository, error) {
	client := g.newClient(ctx, token)
	opts := &github.RepositoryListByAuthenticatedUserOptions{
		ListOptions: github.ListOptions{PerPage: 100},
	}
	var repos []Repository
	for {
		ghRepos, resp, err := client.Repositories.ListByAuthenticatedUser(ctx, opts)
		if err != nil {
			return nil, fmt.Errorf("GetRepositories: %w", err)
		}
		for _, r := range ghRepos {
			defaultBranch := r.GetDefaultBranch()
			if defaultBranch == "" {
				defaultBranch = "main"
			}
			repos = append(repos, Repository{
				Owner:         r.GetOwner().GetLogin(),
				Name:          r.GetName(),
				CloneURL:      r.GetCloneURL(),
				DefaultBranch: defaultBranch,
			})
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return repos, nil
}

// ListForms returns the names (without .yaml extension) of forms in .courtyard/forms/.
func (g *GitHubProvider) ListForms(ctx context.Context, token, owner, repo string) ([]string, error) {
	client := g.newClient(ctx, token)
	_, dirContents, _, err := client.Repositories.GetContents(ctx, owner, repo, ".courtyard/forms", nil)
	if err != nil {
		return nil, fmt.Errorf("ListForms: %w", err)
	}
	var forms []string
	for _, f := range dirContents {
		name := f.GetName()
		if strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml") {
			forms = append(forms, strings.TrimSuffix(strings.TrimSuffix(name, ".yaml"), ".yml"))
		}
	}
	return forms, nil
}

// GetFormConfig fetches and returns the raw YAML for a form spec.
func (g *GitHubProvider) GetFormConfig(ctx context.Context, token, owner, repo, formName string) (*FormConfig, error) {
	client := g.newClient(ctx, token)
	path := fmt.Sprintf(".courtyard/forms/%s.yaml", formName)
	fileContent, _, _, err := client.Repositories.GetContents(ctx, owner, repo, path, nil)
	if err != nil {
		return nil, fmt.Errorf("GetFormConfig: %w", err)
	}
	raw, err := fileContent.GetContent()
	if err != nil {
		return nil, fmt.Errorf("GetFormConfig: decode content: %w", err)
	}
	return &FormConfig{Raw: []byte(raw)}, nil
}

// GetTemplateFiles returns the raw contents of all template files for a form,
// keyed by path relative to .courtyard/templates/{formName}/.
func (g *GitHubProvider) GetTemplateFiles(ctx context.Context, token, owner, repo, formName string) (map[string][]byte, error) {
	client := g.newClient(ctx, token)
	treePath := fmt.Sprintf(".courtyard/templates/%s", formName)

	// Get the SHA for the HEAD commit to resolve the tree.
	ref, _, err := client.Git.GetRef(ctx, owner, repo, "heads/HEAD")
	if err != nil {
		// Fall back: try "heads/main"
		ref, _, err = client.Git.GetRef(ctx, owner, repo, "heads/main")
		if err != nil {
			return nil, fmt.Errorf("GetTemplateFiles: resolve HEAD: %w", err)
		}
	}
	headSHA := ref.GetObject().GetSHA()

	// Resolve the template directory tree SHA.
	commit, _, err := client.Git.GetCommit(ctx, owner, repo, headSHA)
	if err != nil {
		return nil, fmt.Errorf("GetTemplateFiles: get commit: %w", err)
	}

	// Walk the full recursive tree and filter to our template path.
	tree, _, err := client.Git.GetTree(ctx, owner, repo, commit.GetTree().GetSHA(), true)
	if err != nil {
		return nil, fmt.Errorf("GetTemplateFiles: get tree: %w", err)
	}

	prefix := treePath + "/"
	files := make(map[string][]byte)
	for _, entry := range tree.Entries {
		entryPath := entry.GetPath()
		if entry.GetType() != "blob" {
			continue
		}
		if !strings.HasPrefix(entryPath, prefix) {
			continue
		}
		if entry.GetSize() > maxTemplateBlobSize {
			return nil, fmt.Errorf("GetTemplateFiles: file %q exceeds 1MB limit", entryPath)
		}
		blob, _, err := client.Git.GetBlob(ctx, owner, repo, entry.GetSHA())
		if err != nil {
			return nil, fmt.Errorf("GetTemplateFiles: get blob %q: %w", entryPath, err)
		}
		content, err := base64.StdEncoding.DecodeString(blob.GetContent())
		if err != nil {
			// GitHub may omit newlines; try with padding stripped
			content, err = base64.StdEncoding.DecodeString(strings.ReplaceAll(blob.GetContent(), "\n", ""))
			if err != nil {
				return nil, fmt.Errorf("GetTemplateFiles: decode blob %q: %w", entryPath, err)
			}
		}
		relPath := strings.TrimPrefix(entryPath, prefix)
		files[relPath] = content
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("GetTemplateFiles: no templates found at %q", treePath)
	}
	return files, nil
}

// CreateBranchAndPullRequest creates (or reuses) a branch, commits all files,
// and opens a PR. branchMode is read from the FormSpec at a higher layer;
// here "fresh" means we return an error if the branch already exists.
func (g *GitHubProvider) CreateBranchAndPullRequest(
	ctx context.Context,
	token, owner, repo, baseBranch, branchName, commitMessage string,
	files []OutputFile,
) (string, error) {
	client := g.newClient(ctx, token)

	// 1. Get the base branch SHA.
	baseRef, _, err := client.Git.GetRef(ctx, owner, repo, "refs/heads/"+baseBranch)
	if err != nil {
		return "", fmt.Errorf("CreateBranchAndPullRequest: get base ref: %w", err)
	}
	baseSHA := baseRef.GetObject().GetSHA()

	// 2. Check if the target branch already exists.
	targetRefName := "refs/heads/" + branchName
	existingRef, _, refErr := client.Git.GetRef(ctx, owner, repo, "refs/heads/"+branchName)
	branchExists := refErr == nil

	var parentSHA string
	if branchExists {
		parentSHA = existingRef.GetObject().GetSHA()
	} else {
		parentSHA = baseSHA
		// Create the branch.
		newRef := &github.Reference{
			Ref:    github.String(targetRefName),
			Object: &github.GitObject{SHA: github.String(baseSHA)},
		}
		if _, _, err := client.Git.CreateRef(ctx, owner, repo, newRef); err != nil {
			return "", fmt.Errorf("CreateBranchAndPullRequest: create ref: %w", err)
		}
	}

	// 3. Build tree entries.
	var treeEntries []*github.TreeEntry
	for _, f := range files {
		content := string(f.Content)
		treeEntries = append(treeEntries, &github.TreeEntry{
			Path:    github.String(f.Path),
			Mode:    github.String("100644"),
			Type:    github.String("blob"),
			Content: github.String(content),
		})
	}

	// 4. Create the tree.
	tree, _, err := client.Git.CreateTree(ctx, owner, repo, baseSHA, treeEntries)
	if err != nil {
		return "", fmt.Errorf("CreateBranchAndPullRequest: create tree: %w", err)
	}

	// 5. Create the commit.
	newCommit, _, err := client.Git.CreateCommit(ctx, owner, repo, &github.Commit{
		Message: github.String(commitMessage),
		Tree:    &github.Tree{SHA: tree.SHA},
		Parents: []*github.Commit{{SHA: github.String(parentSHA)}},
	}, nil)
	if err != nil {
		return "", fmt.Errorf("CreateBranchAndPullRequest: create commit: %w", err)
	}

	// 6. Update the branch ref to the new commit.
	updateRef := &github.Reference{
		Ref:    github.String(targetRefName),
		Object: &github.GitObject{SHA: newCommit.SHA},
	}
	if _, _, err := client.Git.UpdateRef(ctx, owner, repo, updateRef, false); err != nil {
		return "", fmt.Errorf("CreateBranchAndPullRequest: update ref: %w", err)
	}

	// 7. Open a PR (skip if one already exists for this branch).
	prs, _, err := client.PullRequests.List(ctx, owner, repo, &github.PullRequestListOptions{
		Head:  owner + ":" + branchName,
		Base:  baseBranch,
		State: "open",
	})
	if err != nil {
		return "", fmt.Errorf("CreateBranchAndPullRequest: list PRs: %w", err)
	}
	if len(prs) > 0 {
		return prs[0].GetHTMLURL(), nil
	}

	pr, _, err := client.PullRequests.Create(ctx, owner, repo, &github.NewPullRequest{
		Title: github.String(commitMessage),
		Head:  github.String(branchName),
		Base:  github.String(baseBranch),
		Body:  github.String("Created by Courtyard"),
	})
	if err != nil {
		return "", fmt.Errorf("CreateBranchAndPullRequest: create PR: %w", err)
	}
	return pr.GetHTMLURL(), nil
}
