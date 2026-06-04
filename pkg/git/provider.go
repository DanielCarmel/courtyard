package git

import "context"

// GitProvider defines the contract for interacting with a hosted Git service.
// Every method receives the end-user OAuth token — no admin tokens are used.
type GitProvider interface {
	// GetRepositories returns all repos accessible to the authenticated user.
	GetRepositories(ctx context.Context, token string) ([]Repository, error)

	// ListForms returns the names of forms available in a repository's .courtyard/forms/ directory.
	ListForms(ctx context.Context, token string, owner string, repo string) ([]string, error)

	// GetFormConfig fetches and parses a single form spec YAML file.
	GetFormConfig(ctx context.Context, token string, owner string, repo string, formName string) (*FormConfig, error)

	// GetTemplateFiles returns the raw contents of all template files for a form,
	// keyed by their path relative to .courtyard/templates/{formName}/.
	GetTemplateFiles(ctx context.Context, token string, owner string, repo string, formName string) (map[string][]byte, error)

	// CreateBranchAndPullRequest commits all output files to a new (or existing) branch
	// and opens a pull request. Returns the URL of the created PR.
	CreateBranchAndPullRequest(
		ctx context.Context,
		token string,
		owner string,
		repo string,
		baseBranch string,
		branchName string,
		commitMessage string,
		files []OutputFile,
	) (prURL string, err error)
}

// FormConfig is the parsed representation of a .courtyard/forms/*.yaml file.
// It is defined here to avoid an import cycle between pkg/git and pkg/engine.
type FormConfig struct {
	Raw []byte // raw YAML bytes, parsed by pkg/engine
}
