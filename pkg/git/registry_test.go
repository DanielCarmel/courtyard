package git

import (
	"context"
	"fmt"
	"testing"
)

// stubProvider is a no-op GitProvider implementation used in tests.
type stubProvider struct{}

func (s *stubProvider) GetRepositories(_ context.Context, _ string) ([]Repository, error) {
	return nil, nil
}
func (s *stubProvider) ListForms(_ context.Context, _ string, _ string, _ string) ([]string, error) {
	return nil, nil
}
func (s *stubProvider) GetFormConfig(_ context.Context, _ string, _ string, _ string, _ string) (*FormConfig, error) {
	return nil, nil
}
func (s *stubProvider) GetTemplateFiles(_ context.Context, _ string, _ string, _ string, _ string) (map[string][]byte, error) {
	return nil, nil
}
func (s *stubProvider) CreateBranchAndPullRequest(_ context.Context, _ string, _ string, _ string, _ string, _ string, _ string, _ []OutputFile) (string, error) {
	return "", nil
}

func TestRegistry_Get(t *testing.T) {
	stub := &stubProvider{}
	reg := NewRegistry(map[string]GitProvider{
		"github": stub,
	})

	t.Run("known provider", func(t *testing.T) {
		p, err := reg.Get("github")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p != GitProvider(stub) {
			t.Fatal("returned wrong provider")
		}
	})

	t.Run("unknown provider", func(t *testing.T) {
		_, err := reg.Get("unknown")
		if err == nil {
			t.Fatal("expected error for unknown provider")
		}
		want := fmt.Sprintf("Get: unknown provider %q", "unknown")
		if err.Error() != want {
			t.Fatalf("got %q, want %q", err.Error(), want)
		}
	})
}
