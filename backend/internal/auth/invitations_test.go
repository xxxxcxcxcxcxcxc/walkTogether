package auth

import (
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

func TestServiceCreateInvitationRespectsFiveSlotLimit(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	svc := NewService(db, "token", 5*time.Minute, time.Hour, "friendwalk_session", 7*24*time.Hour)

	mock.ExpectQuery(`SELECT id, telegram_id, display_name, COALESCE\(username, ''\), COALESCE\(avatar_url, ''\), COALESCE\(role, 'user'\) AS role, is_admin, is_active, is_deleted, created_at FROM users WHERE id = \$1 LIMIT 1`).
		WithArgs("user-1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "telegram_id", "display_name", "username", "avatar_url", "role", "is_admin", "is_active", "is_deleted", "created_at"}).
			AddRow("user-1", int64(111), "Alice", "alice", "", "user", false, true, false, time.Now()))

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT is_active, is_deleted FROM users WHERE id = \$1 FOR UPDATE`).
		WithArgs("user-1").
		WillReturnRows(sqlmock.NewRows([]string{"is_active", "is_deleted"}).AddRow(true, false))
	mock.ExpectExec(`UPDATE invitations SET status = 'expired', updated_at = NOW\(\) WHERE sender_user_id = \$1 AND status = 'pending' AND expires_at IS NOT NULL AND expires_at <= NOW\(\)`).
		WithArgs("user-1").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM invitations WHERE sender_user_id = \$1 AND status IN \('pending', 'accepted'\) AND \(expires_at IS NULL OR expires_at > NOW\(\)\)`).
		WithArgs("user-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(5))
	mock.ExpectRollback()

	_, err = svc.CreateInvitationForUser("user-1")
	if err == nil {
		t.Fatal("expected slot limit error")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestServiceValidateInvitationTokenRejectsUsedInvitation(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	svc := NewService(db, "token", 5*time.Minute, time.Hour, "friendwalk_session", 7*24*time.Hour)

	mock.ExpectQuery(`SELECT id, sender_user_id, recipient_user_id, status, expires_at, created_at, updated_at, accepted_at, revoked_at FROM invitations WHERE token_hash = \$1 LIMIT 1`).
		WithArgs(sha256Hex("invite-token")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "sender_user_id", "recipient_user_id", "status", "expires_at", "created_at", "updated_at", "accepted_at", "revoked_at"}).
			AddRow("invite-1", "owner-1", nil, "accepted", time.Now().Add(30*time.Minute), time.Now(), time.Now(), time.Now(), nil))

	_, err = svc.ValidateInvitationToken("invite-token")
	if err == nil {
		t.Fatal("expected accepted invite to be rejected")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}
