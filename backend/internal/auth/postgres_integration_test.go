package auth

import (
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	appdb "friendwalk/internal/db"
	"friendwalk/internal/testdb"
)

func TestPostgresAuthAndInvitationIntegration(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set TEST_POSTGRES_DSN to an isolated PostgreSQL database whose name contains 'test'")
	}
	if dsn == os.Getenv("POSTGRES_DSN") {
		t.Fatal("TEST_POSTGRES_DSN must not point to POSTGRES_DSN")
	}
	parsedDSN, err := url.Parse(dsn)
	if err != nil || (parsedDSN.Scheme != "postgres" && parsedDSN.Scheme != "postgresql") {
		t.Fatal("TEST_POSTGRES_DSN must be a PostgreSQL URL")
	}
	databaseName := strings.TrimPrefix(parsedDSN.Path, "/")
	if !strings.Contains(strings.ToLower(databaseName), "test") {
		t.Fatalf("refusing to run destructive integration setup against database %q; name must contain 'test'", databaseName)
	}

	migrationDir, err := filepath.Abs(filepath.Join("..", "..", "migrations"))
	if err != nil {
		t.Fatal(err)
	}
	connection, err := appdb.Connect(dsn)
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	testdb.LockSerializesDestructiveIntegrationTests(t, connection)
	if err = appdb.ApplyMigrations(dsn, migrationDir); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	if err = appdb.ApplyMigrations(dsn, migrationDir); err != nil {
		t.Fatalf("reapply migrations: %v", err)
	}
	assertPostgresSchema(t, connection)
	if _, err = connection.Exec(`TRUNCATE users CASCADE`); err != nil {
		t.Fatalf("clear isolated test users: %v", err)
	}

	var adminID string
	if err = connection.QueryRow(`
		INSERT INTO users (telegram_id, display_name, is_admin, role)
		VALUES ($1, 'Integration Admin', TRUE, 'admin') RETURNING id
	`, int64(900001)).Scan(&adminID); err != nil {
		t.Fatalf("create test admin: %v", err)
	}

	const botToken = "123456:INTEGRATION_TEST_TOKEN"
	service := NewService(connection, botToken, 5*time.Minute, time.Hour, "friendwalk_session", 2*time.Hour, "friendwalk_bot")
	created := make(chan *Invitation, 12)
	failed := make(chan error, 12)
	var workers sync.WaitGroup
	for i := 0; i < cap(created); i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			invitation, createErr := service.CreateInvitationForUser(adminID)
			if createErr != nil {
				failed <- createErr
				return
			}
			created <- invitation
		}()
	}
	workers.Wait()
	close(created)
	close(failed)

	invitations := make([]*Invitation, 0, maxInvitationSlots)
	for invitation := range created {
		invitations = append(invitations, invitation)
	}
	createErrors := make([]error, 0)
	for createErr := range failed {
		createErrors = append(createErrors, createErr)
	}
	if len(invitations) != maxInvitationSlots {
		t.Fatalf("concurrent creation produced %d invitations, want %d; errors: %v", len(invitations), maxInvitationSlots, createErrors)
	}
	if invitations[0].ExpiresAt == nil || time.Until(*invitations[0].ExpiresAt) <= time.Hour || time.Until(*invitations[0].ExpiresAt) > 2*time.Hour {
		t.Fatalf("invitation expiration does not honor the configured two-hour TTL: %v", invitations[0].ExpiresAt)
	}
	for _, createErr := range createErrors {
		if !hasAuthCode(createErr, ErrInviteLimitReached.Code) {
			t.Fatalf("unexpected concurrent invite error: %v", createErr)
		}
	}
	if _, err = service.CreateInvitationForUser(adminID); !hasAuthCode(err, ErrInviteLimitReached.Code) {
		t.Fatalf("sixth invitation should be rejected, got %v", err)
	}

	link, err := url.Parse(invitations[0].DeepLink)
	if err != nil {
		t.Fatalf("parse generated Telegram deep link: %v", err)
	}
	startPayload := link.Query().Get("start")
	if link.Host != "t.me" || len(startPayload) == 0 || len(startPayload) > 64 {
		t.Fatalf("unexpected Telegram deep link or start payload length: %s", invitations[0].DeepLink)
	}
	if _, err = service.ValidateInvitationToken("missing-integration-token"); !hasAuthCode(err, ErrInvitationRequired.Code) {
		t.Fatalf("unknown token should be rejected, got %v", err)
	}
	if _, err = service.RegisterUserWithInvitation(integrationInitData(t, botToken, 900010), ""); !hasAuthCode(err, ErrInvitationRequired.Code) {
		t.Fatalf("registration without invitation should be rejected, got %v", err)
	}

	var otherUserID string
	if err = connection.QueryRow(`
		INSERT INTO users (telegram_id, display_name) VALUES ($1, 'Other User') RETURNING id
	`, int64(900002)).Scan(&otherUserID); err != nil {
		t.Fatalf("create second test user: %v", err)
	}
	if err = service.RevokeInvitation(otherUserID, invitations[0].ID); err == nil {
		t.Fatal("another user revoked an invitation they do not own")
	}
	if err = service.RevokeInvitation(adminID, invitations[0].ID); err != nil {
		t.Fatalf("owner revoke invitation: %v", err)
	}
	replacement, err := service.CreateInvitationForUser(adminID)
	if err != nil {
		t.Fatalf("revoked slot was not freed: %v", err)
	}

	registered, err := service.RegisterUserWithInvitation(integrationInitData(t, botToken, 900003), tokenFromLink(t, replacement.DeepLink))
	if err != nil {
		t.Fatalf("register invited user: %v", err)
	}
	var senderID string
	var recipientID sql.NullString
	var status string
	if err = connection.QueryRow(`SELECT sender_user_id, recipient_user_id, status FROM invitations WHERE id = $1`, replacement.ID).Scan(&senderID, &recipientID, &status); err != nil {
		t.Fatalf("load consumed invitation: %v", err)
	}
	if senderID != adminID || !recipientID.Valid || recipientID.String != registered.User.ID || status != "accepted" {
		t.Fatalf("invitation linkage mismatch: sender=%s recipient=%v status=%s", senderID, recipientID, status)
	}
	if _, err = service.CreateInvitationForUser(adminID); !hasAuthCode(err, ErrInviteLimitReached.Code) {
		t.Fatalf("registering an invitee should leave its slot occupied, got %v", err)
	}
	var persistedTelegramID int64
	var sessionCount int
	if err = connection.QueryRow(`SELECT telegram_id FROM users WHERE id = $1`, registered.User.ID).Scan(&persistedTelegramID); err != nil || persistedTelegramID != 900003 {
		t.Fatalf("registered Telegram user was not persisted: id=%d err=%v", persistedTelegramID, err)
	}
	if err = connection.QueryRow(`SELECT COUNT(*) FROM sessions WHERE user_id = $1`, registered.User.ID).Scan(&sessionCount); err != nil || sessionCount != 1 {
		t.Fatalf("registration session not persisted: count=%d err=%v", sessionCount, err)
	}
	if _, err = service.CurrentUserFromToken(registered.SessionToken); err != nil {
		t.Fatalf("new session did not grant access: %v", err)
	}
	if _, err = service.RegisterUserWithInvitation(integrationInitData(t, botToken, 900004), tokenFromLink(t, replacement.DeepLink)); !hasAuthCode(err, ErrInvitationAlreadyUsed.Code) {
		t.Fatalf("used invitation should be rejected for another Telegram ID, got %v", err)
	}
	assertConcurrentInvitationRedemption(t, connection, service, botToken, invitations[2])
	if err = connection.QueryRow(`SELECT COUNT(*) FROM users WHERE telegram_id = $1`, int64(900003)).Scan(&sessionCount); err != nil || sessionCount != 1 {
		t.Fatalf("existing Telegram ID produced duplicate users: count=%d err=%v", sessionCount, err)
	}

	assertInvitationRejected(t, connection, service, adminID, "expired-integration-token", "pending", time.Now().Add(-time.Minute), ErrInvitationExpired.Code)
	assertInvitationRejected(t, connection, service, adminID, "revoked-integration-token", "revoked", time.Now().Add(time.Hour), ErrInvitationRevoked.Code)
	assertInvitationRejected(t, connection, service, otherUserID, "accepted-integration-token", "accepted", time.Now().Add(time.Hour), ErrInvitationAlreadyUsed.Code)

	if _, err = service.RegisterUserWithInvitation(integrationInitData(t, botToken, 900003), "tampered-token"); err != nil {
		t.Fatalf("existing user login unexpectedly required an invite: %v", err)
	}
	if _, err = service.RegisterUserWithInvitation("user=%7B%22id%22%3A999999%7D", tokenFromLink(t, invitations[1].DeepLink)); err == nil {
		t.Fatal("unverified initData was accepted")
	}

	if _, err = service.RegisterUserWithInvitation(integrationInitData(t, botToken, 900005), tokenFromLink(t, invitations[1].DeepLink)); err != nil {
		t.Fatalf("register second invited user: %v", err)
	}
	var secondUserID string
	if err = connection.QueryRow(`SELECT id FROM users WHERE telegram_id = $1`, int64(900005)).Scan(&secondUserID); err != nil {
		t.Fatalf("load second invited user: %v", err)
	}
	secondSession, err := service.AuthenticateTelegram(integrationInitData(t, botToken, 900005))
	if err != nil {
		t.Fatalf("create second session: %v", err)
	}
	if err = service.RevokeAllUserSessions(secondSession.SessionToken); err != nil {
		t.Fatalf("revoke all own sessions: %v", err)
	}
	if _, err = service.CurrentUserFromToken(secondSession.SessionToken); err == nil {
		t.Fatal("revoked session remained usable")
	}
	if err = connection.QueryRow(`SELECT COUNT(*) FROM sessions WHERE user_id = $1 AND revoked_at IS NOT NULL`, secondUserID).Scan(&sessionCount); err != nil || sessionCount != 2 {
		t.Fatalf("revoke-all did not revoke both sessions: count=%d err=%v", sessionCount, err)
	}

	if err = service.RevokeUserAccess(adminID, registered.User.ID); err != nil {
		t.Fatalf("admin revoke user access: %v", err)
	}
	if _, err = service.CurrentUserFromToken(registered.SessionToken); err == nil {
		t.Fatal("blocked user retained access")
	}
	if _, err = service.RegisterUserWithInvitation(integrationInitData(t, botToken, 900003), ""); !hasAuthCode(err, ErrAccountBlocked.Code) {
		t.Fatalf("blocked user login should be rejected, got %v", err)
	}
	if err = connection.QueryRow(`SELECT COUNT(*) FROM sessions WHERE user_id = $1 AND revoked_at IS NOT NULL`, registered.User.ID).Scan(&sessionCount); err != nil || sessionCount != 2 {
		t.Fatalf("admin revoke did not revoke every session: count=%d err=%v", sessionCount, err)
	}
	if err = connection.QueryRow(`SELECT status FROM invitations WHERE id = $1`, replacement.ID).Scan(&status); err != nil || status != "revoked" {
		t.Fatalf("access revoke did not free accepted invite slot: status=%q err=%v", status, err)
	}
	if _, err = service.CreateInvitationForUser(adminID); err != nil {
		t.Fatalf("revoked access slot was not freed: %v", err)
	}
}

func assertPostgresSchema(t *testing.T, connection *sql.DB) {
	t.Helper()
	var foreignKeys, indexes, uniqueIndexes, roleChecks, inviteTimestamps, friendTableCount, friendIndexes, privacyChecks int
	err := connection.QueryRow(`
		SELECT
			(SELECT COUNT(*) FROM pg_constraint WHERE contype = 'f' AND conrelid IN ('sessions'::regclass, 'invitations'::regclass) AND confrelid = 'users'::regclass),
			(SELECT COUNT(*) FROM pg_indexes WHERE indexname IN ('ux_sessions_token_hash', 'idx_sessions_user_revoked', 'idx_users_role')),
			(SELECT COUNT(*) FROM pg_indexes WHERE indexname IN ('users_telegram_id_key', 'invitations_token_hash_key')),
			(SELECT COUNT(*) FROM pg_constraint WHERE conname = 'users_role_check'),
			(SELECT COUNT(*) FROM information_schema.columns WHERE table_name = 'invitations' AND column_name IN ('accepted_at', 'revoked_at')),
			(SELECT COUNT(*) FROM information_schema.tables WHERE table_name = 'friend_requests' AND table_schema = 'public'),
			(SELECT COUNT(*) FROM pg_indexes WHERE indexname IN ('ux_users_public_id', 'ux_friend_requests_active_pair', 'idx_friend_requests_recipient_status', 'idx_friend_requests_requester_status')),
			(SELECT COUNT(*) FROM pg_constraint WHERE conname = 'users_privacy_mode_check')
	`).Scan(&foreignKeys, &indexes, &uniqueIndexes, &roleChecks, &inviteTimestamps, &friendTableCount, &friendIndexes, &privacyChecks)
	if err != nil {
		t.Fatalf("inspect PostgreSQL schema: %v", err)
	}
	if foreignKeys != 3 || indexes != 3 || uniqueIndexes != 2 || roleChecks != 1 || inviteTimestamps != 2 || friendTableCount != 1 || friendIndexes != 4 || privacyChecks != 1 {
		t.Fatalf("incomplete PostgreSQL schema: foreign keys=%d indexes=%d unique indexes=%d role checks=%d invite timestamps=%d friend table=%d friend indexes=%d privacy checks=%d", foreignKeys, indexes, uniqueIndexes, roleChecks, inviteTimestamps, friendTableCount, friendIndexes, privacyChecks)
	}
}

func assertInvitationRejected(t *testing.T, connection *sql.DB, service *Service, senderID string, rawToken string, status string, expiresAt time.Time, wantCode string) {
	t.Helper()
	if _, err := connection.Exec(`
		INSERT INTO invitations (sender_user_id, token_hash, status, expires_at) VALUES ($1, $2, $3, $4)
	`, senderID, sha256Hex(rawToken), status, expiresAt); err != nil {
		t.Fatalf("create %s test invitation: %v", status, err)
	}
	if _, err := service.ValidateInvitationToken(rawToken); !hasAuthCode(err, wantCode) {
		t.Fatalf("%s invitation should be rejected with %s, got %v", status, wantCode, err)
	}
}

func assertConcurrentInvitationRedemption(t *testing.T, connection *sql.DB, service *Service, botToken string, invitation *Invitation) {
	t.Helper()
	token := tokenFromLink(t, invitation.DeepLink)
	initData := []string{
		integrationInitData(t, botToken, 900006),
		integrationInitData(t, botToken, 900007),
	}
	type result struct {
		user *AuthenticatedUser
		err  error
	}
	results := make(chan result, len(initData))
	var workers sync.WaitGroup
	for _, data := range initData {
		workers.Add(1)
		go func(data string) {
			defer workers.Done()
			user, err := service.RegisterUserWithInvitation(data, token)
			results <- result{user: user, err: err}
		}(data)
	}
	workers.Wait()
	close(results)

	successes := 0
	failures := 0
	for item := range results {
		if item.err == nil {
			successes++
		} else if hasAuthCode(item.err, ErrInvitationAlreadyUsed.Code) {
			failures++
		} else {
			t.Fatalf("unexpected concurrent redemption error: %v", item.err)
		}
	}
	if successes != 1 || failures != 1 {
		t.Fatalf("concurrent redemption results: successes=%d used-errors=%d, want 1 each", successes, failures)
	}
	var createdUsers int
	if err := connection.QueryRow(`SELECT COUNT(*) FROM users WHERE telegram_id IN (900006, 900007)`).Scan(&createdUsers); err != nil || createdUsers != 1 {
		t.Fatalf("concurrent redemption created %d users, want exactly 1 (err=%v)", createdUsers, err)
	}
}

func integrationInitData(t *testing.T, botToken string, telegramID int64) string {
	t.Helper()
	userJSON := fmt.Sprintf(`{"id":%d,"first_name":"Integration","username":"test_%d"}`, telegramID, telegramID)
	return buildInitData(t, botToken, map[string]string{"user": userJSON}, time.Now())
}

func tokenFromLink(t *testing.T, deepLink string) string {
	t.Helper()
	parsed, err := url.Parse(deepLink)
	if err != nil {
		t.Fatal(err)
	}
	return parsed.Query().Get("start")
}

func hasAuthCode(err error, code string) bool {
	var authErr *AuthError
	return errors.As(err, &authErr) && authErr.Code == code
}
