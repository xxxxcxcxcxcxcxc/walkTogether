package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// Session represents an authenticated user session stored in the database.
type Session struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	TokenHash string    `json:"token_hash"`
	ExpiresAt time.Time `json:"expires_at"`
	RevokedAt time.Time `json:"revoked_at,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// SessionStore is the contract for session persistence and validation.
type SessionStore interface {
	CreateSession(userID string, expiresAt time.Time) (Session, string, error)
	GetSession(token string, now time.Time) (Session, error)
	RevokeSession(id string) error
	RevokeAllUserSessions(userID string) error
}

// MemorySessionStore implements a simple in-memory store for tests and local validation.
type MemorySessionStore struct {
	mu       sync.Mutex
	sessions map[string]Session
	byUser   map[string]map[string]Session
}

func NewMemorySessionStore() *MemorySessionStore {
	return &MemorySessionStore{
		sessions: make(map[string]Session),
		byUser:   make(map[string]map[string]Session),
	}
}

func (m *MemorySessionStore) CreateSession(userID string, expiresAt time.Time) (Session, string, error) {
	token, err := generateSessionToken()
	if err != nil {
		return Session{}, "", fmt.Errorf("generate session token: %w", err)
	}

	session := Session{
		ID:        token[:12],
		UserID:    userID,
		TokenHash: sha256Hex(token),
		ExpiresAt: expiresAt,
		CreatedAt: time.Now().UTC(),
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[token] = session
	if _, ok := m.byUser[userID]; !ok {
		m.byUser[userID] = make(map[string]Session)
	}
	m.byUser[userID][session.ID] = session

	return session, token, nil
}

func (m *MemorySessionStore) GetSession(token string, now time.Time) (Session, error) {
	if token == "" {
		return Session{}, fmt.Errorf("session token is empty")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.sessions[token]
	if !ok {
		return Session{}, fmt.Errorf("session not found")
	}
	if !session.RevokedAt.IsZero() {
		return Session{}, fmt.Errorf("session revoked")
	}
	if session.ExpiresAt.Before(now) || session.ExpiresAt.Equal(now) {
		return Session{}, fmt.Errorf("session expired")
	}

	return session, nil
}

func (m *MemorySessionStore) RevokeSession(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for token, session := range m.sessions {
		if session.ID == id {
			session.RevokedAt = time.Now().UTC()
			m.sessions[token] = session
			return nil
		}
	}

	return fmt.Errorf("session not found")
}

func (m *MemorySessionStore) RevokeAllUserSessions(userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for token, session := range m.sessions {
		if session.UserID == userID {
			session.RevokedAt = time.Now().UTC()
			m.sessions[token] = session
		}
	}

	delete(m.byUser, userID)
	return nil
}

func generateSessionToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func sha256Hex(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
