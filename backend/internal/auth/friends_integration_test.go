package auth

import (
	"database/sql"
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

func TestPostgresFriendsAndPrivacyIntegration(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set TEST_POSTGRES_DSN to an isolated PostgreSQL database whose name contains 'test'")
	}
	if dsn == os.Getenv("POSTGRES_DSN") {
		t.Fatal("TEST_POSTGRES_DSN must not point to POSTGRES_DSN")
	}
	parsed, err := url.Parse(dsn)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") || len(parsed.Path) < 2 || !containsTest(parsed.Path) {
		t.Fatal("TEST_POSTGRES_DSN must name an isolated PostgreSQL database containing 'test'")
	}
	migrationDir, err := filepath.Abs(filepath.Join("..", "..", "migrations"))
	if err != nil {
		t.Fatal(err)
	}
	connection, err := appdb.Connect(dsn)
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	connection.SetMaxOpenConns(12)
	t.Cleanup(func() { _ = connection.Close() })
	testdb.LockSerializesDestructiveIntegrationTests(t, connection)
	if err = appdb.ApplyMigrations(dsn, migrationDir); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	if _, err = connection.Exec(`TRUNCATE users CASCADE`); err != nil {
		t.Fatalf("clear isolated test users: %v", err)
	}

	alice := createFriendTestUser(t, connection, 910001, "Alice Walker", "alice")
	bob := createFriendTestUser(t, connection, 910002, "Bob Walker", "bob")
	carol := createFriendTestUser(t, connection, 910003, "Carol Private", "carol")
	dan := createFriendTestUser(t, connection, 910004, "Dan Hidden", "dan")
	emma := createFriendTestUser(t, connection, 910005, "Emma Disabled", "emma")
	frank := createFriendTestUser(t, connection, 910006, "Frank Race", "frank")
	admin := createFriendTestUser(t, connection, 910000, "Integration Admin", "friend_admin")
	if _, err = connection.Exec(`UPDATE users SET is_admin = TRUE, role = 'admin' WHERE id = $1`, admin.ID); err != nil {
		t.Fatalf("promote integration admin: %v", err)
	}
	service := NewService(connection, "123456:INTEGRATION_TEST_TOKEN", 5*time.Minute, time.Hour, "friendwalk_session", 2*time.Hour)

	if err = connection.QueryRow(`SELECT privacy_mode FROM users WHERE id = $1`, alice.ID).Scan(&alice.Privacy); err != nil || alice.Privacy != PrivacyAll {
		t.Fatalf("new user default visibility should be all, got %q (err=%v)", alice.Privacy, err)
	}
	results, err := service.SearchFriendsUsers(alice.ID, "bo")
	if err != nil || len(results) != 1 || results[0].PublicID != bob.PublicID || results[0].RelationshipStatus != FriendStatusNone {
		t.Fatalf("name/username search failed: results=%+v err=%v", results, err)
	}
	for _, profile := range results {
		if profile.PublicID == bob.ID || profile.Username != "bob" {
			t.Fatalf("search returned an internal identifier or omitted the username: %+v", profile)
		}
	}
	if _, err = service.CreateFriendRequest(alice.ID, alice.PublicID); !hasAuthCode(err, "user_not_found") {
		t.Fatalf("self-friend request was not rejected: %v", err)
	}
	if _, err = service.SearchFriendsUsers(alice.ID, "a"); !hasAuthCode(err, "invalid_search_query") {
		t.Fatalf("one-character query should be rejected: %v", err)
	}

	firstRequest, err := service.CreateFriendRequest(alice.ID, bob.PublicID)
	if err != nil || firstRequest.Status != "pending" {
		t.Fatalf("send friend request: request=%+v err=%v", firstRequest, err)
	}
	if _, err = service.CreateFriendRequest(alice.ID, bob.PublicID); !hasAuthCode(err, "friend_request_pending") {
		t.Fatalf("duplicate outgoing request was accepted: %v", err)
	}
	var outboxCount int
	if err = connection.QueryRow(`SELECT COUNT(*) FROM notification_jobs WHERE user_id = $1 AND notification_type = 'friend_request_received'`, bob.ID).Scan(&outboxCount); err != nil || outboxCount != 1 {
		t.Fatalf("duplicate request created a duplicate notification job: count=%d err=%v", outboxCount, err)
	}
	incoming, err := service.ListFriendRequests(bob.ID, "incoming")
	if err != nil || len(incoming) != 1 || incoming[0].ID != firstRequest.ID {
		t.Fatalf("incoming request listing failed: %+v err=%v", incoming, err)
	}
	outgoing, err := service.ListFriendRequests(alice.ID, "outgoing")
	if err != nil || len(outgoing) != 1 || outgoing[0].ID != firstRequest.ID {
		t.Fatalf("outgoing request listing failed: %+v err=%v", outgoing, err)
	}
	if err = service.RespondFriendRequest(alice.ID, firstRequest.ID, "accept"); !hasAuthCode(err, "friend_request_not_found") {
		t.Fatalf("requester accepted their own request: %v", err)
	}
	if err = service.RespondFriendRequest(bob.ID, firstRequest.ID, "accept"); err != nil {
		t.Fatalf("accept request: %v", err)
	}
	if err = connection.QueryRow(`SELECT COUNT(*) FROM notification_jobs WHERE user_id = $1 AND notification_type = 'friend_request_accepted'`, alice.ID).Scan(&outboxCount); err != nil || outboxCount != 1 {
		t.Fatalf("accept notification was not queued exactly once: count=%d err=%v", outboxCount, err)
	}
	aliceFriends, err := service.ListFriends(alice.ID)
	if err != nil || len(aliceFriends) != 1 || aliceFriends[0].PublicID != bob.PublicID {
		t.Fatalf("accepted friend missing from Alice's list: %+v err=%v", aliceFriends, err)
	}
	bobFriends, err := service.ListFriends(bob.ID)
	if err != nil || len(bobFriends) != 1 || bobFriends[0].PublicID != alice.PublicID {
		t.Fatalf("accepted friend missing from Bob's list: %+v err=%v", bobFriends, err)
	}
	profile, err := service.GetFriendProfile(alice.ID, bob.PublicID)
	if err != nil || profile.RelationshipStatus != FriendStatusAccepted {
		t.Fatalf("friend profile relationship status incorrect: %+v err=%v", profile, err)
	}
	var relationshipRows int
	if err = connection.QueryRow(`SELECT COUNT(*) FROM friend_requests WHERE user_low_id = LEAST($1::uuid, $2::uuid) AND user_high_id = GREATEST($1::uuid, $2::uuid)`, alice.ID, bob.ID).Scan(&relationshipRows); err != nil || relationshipRows != 1 {
		t.Fatalf("a pair must be represented by one historical row: count=%d err=%v", relationshipRows, err)
	}
	lowID, highID := orderedUserIDs(alice.ID, bob.ID)
	if _, err = connection.Exec(`INSERT INTO friend_requests (user_low_id, user_high_id, requester_user_id, recipient_user_id, status) VALUES ($1, $2, $3, $4, 'accepted')`, lowID, highID, alice.ID, bob.ID); err == nil {
		t.Fatal("database allowed a second active relationship row for the same pair")
	}

	if err = service.UpdateFriendPrivacySettings(bob.ID, FriendPrivacySettings{Visibility: PrivacyFriends, FriendNotificationsEnabled: true}); err != nil {
		t.Fatalf("set friends-only visibility: %v", err)
	}
	if _, err = service.GetFriendProfile(carol.ID, bob.PublicID); !hasAuthCode(err, "user_not_found") {
		t.Fatalf("non-friend saw friends-only profile: %v", err)
	}
	if results, err = service.SearchFriendsUsers(carol.ID, "Bob"); err != nil || len(results) != 0 {
		t.Fatalf("non-friend discovered friends-only profile: results=%+v err=%v", results, err)
	}
	if results, err = service.SearchFriendsUsers(alice.ID, "Bob"); err != nil || len(results) != 1 || results[0].RelationshipStatus != FriendStatusAccepted {
		t.Fatalf("friend could not discover friends-only profile: results=%+v err=%v", results, err)
	}
	if _, err = service.CreateFriendRequest(alice.ID, bob.PublicID); !hasAuthCode(err, "already_friends") {
		t.Fatalf("existing friend request against a friends-only profile was not recognized: %v", err)
	}
	if err = service.RemoveFriend(alice.ID, bob.PublicID); err != nil {
		t.Fatalf("remove friend: %v", err)
	}
	if friends, err := service.ListFriends(alice.ID); err != nil || len(friends) != 0 {
		t.Fatalf("removed friend is still active: %+v err=%v", friends, err)
	}
	var historyStatus string
	if err = connection.QueryRow(`SELECT status FROM friend_requests WHERE id = $1`, firstRequest.ID).Scan(&historyStatus); err != nil || historyStatus != "cancelled" {
		t.Fatalf("friend removal did not preserve history as cancelled: status=%q err=%v", historyStatus, err)
	}
	if err = service.UpdateFriendPrivacySettings(bob.ID, FriendPrivacySettings{Visibility: PrivacyAll, FriendNotificationsEnabled: true}); err != nil {
		t.Fatalf("restore participant visibility for a new request: %v", err)
	}
	secondRequest, err := service.CreateFriendRequest(alice.ID, bob.PublicID)
	if err != nil || secondRequest.ID == firstRequest.ID {
		t.Fatalf("request after removal should create new history row: request=%+v err=%v", secondRequest, err)
	}
	if err = service.RespondFriendRequest(bob.ID, secondRequest.ID, "reject"); err != nil {
		t.Fatalf("reject request: %v", err)
	}
	thirdRequest, err := service.CreateFriendRequest(alice.ID, bob.PublicID)
	if err != nil || thirdRequest.ID == secondRequest.ID {
		t.Fatalf("request after rejection should create a new row: request=%+v err=%v", thirdRequest, err)
	}
	if err = service.RespondFriendRequest(alice.ID, thirdRequest.ID, "cancel"); err != nil {
		t.Fatalf("cancel outgoing request: %v", err)
	}
	profile, err = service.GetFriendProfile(alice.ID, bob.PublicID)
	if err != nil || profile.RelationshipStatus != FriendStatusCancelled {
		t.Fatalf("cancelled relationship state was not preserved in profile: %+v err=%v", profile, err)
	}
	if err = connection.QueryRow(`SELECT COUNT(*) FROM friend_requests WHERE id IN ($1, $2, $3) AND status IN ('cancelled', 'rejected')`, firstRequest.ID, secondRequest.ID, thirdRequest.ID).Scan(&relationshipRows); err != nil || relationshipRows != 3 {
		t.Fatalf("friendship history was not preserved: count=%d err=%v", relationshipRows, err)
	}
	if err = service.RespondFriendRequest(alice.ID, secondRequest.ID, "accept"); !hasAuthCode(err, "friend_request_not_found") {
		t.Fatalf("non-recipient changed another user's request: %v", err)
	}

	if err = service.UpdateFriendPrivacySettings(dan.ID, FriendPrivacySettings{Visibility: PrivacyHidden, FriendNotificationsEnabled: true}); err != nil {
		t.Fatalf("set hidden visibility: %v", err)
	}
	if results, err = service.SearchFriendsUsers(alice.ID, "Dan"); err != nil || len(results) != 0 {
		t.Fatalf("hidden user appeared in search: results=%+v err=%v", results, err)
	}
	if _, err = service.GetFriendProfile(alice.ID, dan.PublicID); !hasAuthCode(err, "user_not_found") {
		t.Fatalf("hidden profile was available directly: %v", err)
	}
	if _, err = service.CreateFriendRequest(alice.ID, dan.PublicID); !hasAuthCode(err, "user_not_found") {
		t.Fatalf("hidden user accepted a direct request bypassing discovery: %v", err)
	}

	if err = service.UpdateFriendPrivacySettings(emma.ID, FriendPrivacySettings{Visibility: PrivacyAll, FriendNotificationsEnabled: false}); err != nil {
		t.Fatalf("disable friend notifications: %v", err)
	}
	if _, err = service.CreateFriendRequest(alice.ID, emma.PublicID); err != nil {
		t.Fatalf("send request to notifications-disabled user: %v", err)
	}
	var queuedNotifications int
	if err = connection.QueryRow(`SELECT COUNT(*) FROM notification_jobs WHERE user_id = $1 AND notification_type = 'friend_request_received'`, emma.ID).Scan(&queuedNotifications); err != nil || queuedNotifications != 0 {
		t.Fatalf("disabled friend notifications were queued: count=%d err=%v", queuedNotifications, err)
	}
	if _, err = service.CreateFriendRequest(alice.ID, emma.PublicID); !hasAuthCode(err, "friend_request_pending") {
		t.Fatalf("pending relationship was duplicated after rejection flow: %v", err)
	}

	concurrentRequests := make(chan error, 2)
	var workers sync.WaitGroup
	for _, pair := range []struct{ actor, target string }{{alice.ID, frank.PublicID}, {frank.ID, alice.PublicID}} {
		workers.Add(1)
		go func(actor string, target string) {
			defer workers.Done()
			_, createErr := service.CreateFriendRequest(actor, target)
			concurrentRequests <- createErr
		}(pair.actor, pair.target)
	}
	workers.Wait()
	close(concurrentRequests)
	successes := 0
	failures := 0
	for createErr := range concurrentRequests {
		if createErr == nil {
			successes++
		} else if hasAuthCode(createErr, "incoming_request_exists") || hasAuthCode(createErr, "friend_request_pending") {
			failures++
		} else {
			t.Fatalf("unexpected concurrent request error: %v", createErr)
		}
	}
	if successes != 1 || failures != 1 {
		t.Fatalf("concurrent requests produced successes=%d expected-state-errors=%d", successes, failures)
	}
	if err = connection.QueryRow(`SELECT COUNT(*) FROM friend_requests WHERE user_low_id = LEAST($1::uuid, $2::uuid) AND user_high_id = GREATEST($1::uuid, $2::uuid) AND status = 'pending'`, alice.ID, frank.ID).Scan(&relationshipRows); err != nil || relationshipRows != 1 {
		t.Fatalf("concurrent requests created duplicate active rows: count=%d err=%v", relationshipRows, err)
	}

	carolRequest, err := service.CreateFriendRequest(carol.ID, alice.PublicID)
	if err != nil {
		t.Fatalf("create request before admin block: %v", err)
	}
	if err = service.RevokeUserAccess(admin.ID, carol.ID); err != nil {
		t.Fatalf("admin block friendship user: %v", err)
	}
	if _, err = service.SearchFriendsUsers(carol.ID, "Alice"); !hasAuthCode(err, ErrAccountBlocked.Code) {
		t.Fatalf("blocked user could search: %v", err)
	}
	if _, err = service.CreateFriendRequest(carol.ID, alice.PublicID); !hasAuthCode(err, "user_not_found") {
		t.Fatalf("blocked user could send request: %v", err)
	}
	if _, err = service.ListFriends(carol.ID); !hasAuthCode(err, ErrAccountBlocked.Code) {
		t.Fatalf("blocked user could list friends: %v", err)
	}
	if err = connection.QueryRow(`SELECT status FROM friend_requests WHERE id = $1`, carolRequest.ID).Scan(&historyStatus); err != nil || historyStatus != "cancelled" {
		t.Fatalf("admin block did not close active friend request: status=%q err=%v", historyStatus, err)
	}
}

type friendTestUser struct {
	ID       string
	PublicID string
	Privacy  string
}

func createFriendTestUser(t *testing.T, connection *sql.DB, telegramID int64, name string, username string) friendTestUser {
	t.Helper()
	var user friendTestUser
	err := connection.QueryRow(`
		INSERT INTO users (telegram_id, display_name, username)
		VALUES ($1, $2, $3) RETURNING id, public_id, privacy_mode
	`, telegramID, name, username).Scan(&user.ID, &user.PublicID, &user.Privacy)
	if err != nil {
		t.Fatalf("create friend integration user: %v", err)
	}
	return user
}

func containsTest(value string) bool {
	return strings.Contains(strings.ToLower(value), "test")
}
