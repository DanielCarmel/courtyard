package server

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

const (
	sessionTTL   = 8 * time.Hour
	sweepPeriod  = 15 * time.Minute
)

// Session holds data for an authenticated user session.
type Session struct {
	ID        string
	Token     string // OAuth access token
	Provider  string // "github" or "bitbucket"
	ExpiresAt time.Time
}

// SessionStore is a concurrency-safe in-memory session store.
// It has no external dependencies and survives only for the process lifetime.
type SessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*Session
}

// NewSessionStore creates a SessionStore and starts the background sweep goroutine.
func NewSessionStore() *SessionStore {
	s := &SessionStore{
		sessions: make(map[string]*Session),
	}
	go s.sweep()
	return s
}

// Create mints a new random session ID, stores the session, and returns it.
func (s *SessionStore) Create(token, provider string) (*Session, error) {
	id, err := randomHex(32)
	if err != nil {
		return nil, err
	}
	sess := &Session{
		ID:        id,
		Token:     token,
		Provider:  provider,
		ExpiresAt: time.Now().Add(sessionTTL),
	}
	s.mu.Lock()
	s.sessions[id] = sess
	s.mu.Unlock()
	return sess, nil
}

// Get returns the session for id, or nil if not found or expired.
func (s *SessionStore) Get(id string) *Session {
	s.mu.RLock()
	sess, ok := s.sessions[id]
	s.mu.RUnlock()
	if !ok || time.Now().After(sess.ExpiresAt) {
		return nil
	}
	return sess
}

// Delete removes the session for id.
func (s *SessionStore) Delete(id string) {
	s.mu.Lock()
	delete(s.sessions, id)
	s.mu.Unlock()
}

// sweep periodically removes expired sessions.
func (s *SessionStore) sweep() {
	ticker := time.NewTicker(sweepPeriod)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now()
		s.mu.Lock()
		for id, sess := range s.sessions {
			if now.After(sess.ExpiresAt) {
				delete(s.sessions, id)
			}
		}
		s.mu.Unlock()
	}
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
