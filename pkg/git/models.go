package git

// Repository represents a Git repository.
type Repository struct {
	Owner         string
	Name          string
	CloneURL      string
	DefaultBranch string
}

// OutputFile is a rendered file to be committed to Git.
type OutputFile struct {
	Path    string
	Content []byte
}

// UserInfo holds the authenticated user's profile information from a Git provider.
type UserInfo struct {
	Username  string `json:"username"`
	AvatarURL string `json:"avatar_url"`
	Provider  string `json:"provider"`
}
