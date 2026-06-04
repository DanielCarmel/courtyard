package git

import "fmt"

// Registry maps provider names to their GitProvider implementation.
type Registry struct {
	providers map[string]GitProvider
}

// NewRegistry creates a Registry pre-loaded with the given providers.
func NewRegistry(providers map[string]GitProvider) *Registry {
	return &Registry{providers: providers}
}

// Get returns the GitProvider for the given name ("github", "bitbucket").
// Returns an error if the provider is not registered.
func (r *Registry) Get(name string) (GitProvider, error) {
	p, ok := r.providers[name]
	if !ok {
		return nil, fmt.Errorf("Get: unknown provider %q", name)
	}
	return p, nil
}
