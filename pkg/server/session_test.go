package server

import (
	"testing"
	"time"
)

func TestSessionStore_CreateAndGet(t *testing.T) {
	store := NewSessionStore()

	t.Run("create and retrieve", func(t *testing.T) {
		sess, err := store.Create("tok123", "github")
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if sess.Token != "tok123" {
			t.Errorf("token: got %q, want %q", sess.Token, "tok123")
		}
		if sess.Provider != "github" {
			t.Errorf("provider: got %q, want %q", sess.Provider, "github")
		}

		got := store.Get(sess.ID)
		if got == nil {
			t.Fatal("Get returned nil for valid session")
		}
		if got.Token != sess.Token {
			t.Errorf("token mismatch after Get")
		}
	})

	t.Run("missing session returns nil", func(t *testing.T) {
		if got := store.Get("nonexistent"); got != nil {
			t.Errorf("expected nil for missing session, got %+v", got)
		}
	})

	t.Run("expired session returns nil", func(t *testing.T) {
		sess, err := store.Create("expired-tok", "github")
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		// Force expiry.
		store.mu.Lock()
		store.sessions[sess.ID].ExpiresAt = time.Now().Add(-1 * time.Second)
		store.mu.Unlock()

		if got := store.Get(sess.ID); got != nil {
			t.Errorf("expected nil for expired session, got %+v", got)
		}
	})

	t.Run("delete removes session", func(t *testing.T) {
		sess, err := store.Create("delete-tok", "bitbucket")
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		store.Delete(sess.ID)
		if got := store.Get(sess.ID); got != nil {
			t.Errorf("expected nil after Delete, got %+v", got)
		}
	})
}
