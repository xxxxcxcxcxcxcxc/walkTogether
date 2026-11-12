package auth

import (
	"testing"
	"time"
)

func TestSessionCreateAndValidate(t *testing.T) {
	store := NewMemorySessionStore()
	userID := "user-123"

	session, token, err := store.CreateSession(userID, time.Now().Add(30*time.Minute))
	if err != nil {
		t.Fatalf("expected create session to succeed: %v", err)
	}
	if token == "" {
		t.Fatal("expected token to be generated")
	}
	if session.UserID != userID {
		t.Fatalf("expected user id %q, got %q", userID, session.UserID)
	}

	loaded, err := store.GetSession(token, time.Now())
	if err != nil {
		t.Fatalf("expected valid session to resolve: %v", err)
	}
	if loaded.UserID != userID {
		t.Fatalf("expected user id %q, got %q", userID, loaded.UserID)
	}
}

func TestSessionRejectsInvalidAndExpiredTokens(t *testing.T) {
	store := NewMemorySessionStore()
	_, err := store.GetSession("invalid-token", time.Now())
	if err == nil {
		t.Fatal("expected invalid token to be rejected")
	}

	_, token, err := store.CreateSession("user-456", time.Now().Add(-1*time.Minute))
	if err != nil {
		t.Fatalf("expected session creation to succeed: %v", err)
	}

	if _, err = store.GetSession(token, time.Now()); err == nil {
		t.Fatal("expected expired session to be rejected")
	}
}

func TestSessionRevokeAndLogoutAll(t *testing.T) {
	store := NewMemorySessionStore()
	first, token1, err := store.CreateSession("user-789", time.Now().Add(1*time.Hour))
	if err != nil {
		t.Fatalf("expected first session to be created: %v", err)
	}
	_, token2, err := store.CreateSession("user-789", time.Now().Add(1*time.Hour))
	if err != nil {
		t.Fatalf("expected second session to be created: %v", err)
	}

	if err = store.RevokeSession(first.ID); err != nil {
		t.Fatalf("expected session revoke to work: %v", err)
	}
	if _, err = store.GetSession(token1, time.Now()); err == nil {
		t.Fatal("expected revoked session to fail validation")
	}
	if _, err = store.GetSession(token2, time.Now()); err != nil {
		t.Fatal("expected other active session to remain valid")
	}

	if err = store.RevokeAllUserSessions("user-789"); err != nil {
		t.Fatalf("expected logout-all to work: %v", err)
	}
	if _, err = store.GetSession(token2, time.Now()); err == nil {
		t.Fatal("expected all user sessions to be revoked")
	}
}
