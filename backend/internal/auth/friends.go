package auth

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

const (
	FriendStatusNone            = "none"
	FriendStatusOutgoingPending = "outgoing_pending"
	FriendStatusIncomingPending = "incoming_pending"
	FriendStatusAccepted        = "accepted"
	FriendStatusRejected        = "rejected"
	FriendStatusCancelled       = "cancelled"
	PrivacyAll                  = "all"
	PrivacyFriends              = "friends"
	PrivacyHidden               = "hidden"
)

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

type FriendProfile struct {
	PublicID           string    `json:"public_id"`
	DisplayName        string    `json:"display_name"`
	Username           string    `json:"username,omitempty"`
	AvatarURL          string    `json:"avatar_url,omitempty"`
	JoinedAt           time.Time `json:"joined_at"`
	RelationshipStatus string    `json:"relationship_status"`
	RequestID          string    `json:"request_id,omitempty"`
}

type FriendRequest struct {
	ID        string        `json:"id"`
	Status    string        `json:"status"`
	CreatedAt time.Time     `json:"created_at"`
	User      FriendProfile `json:"user"`
}

type FriendPrivacySettings struct {
	Visibility                 string `json:"visibility"`
	FriendNotificationsEnabled bool   `json:"friend_notifications_enabled"`
}

type friendDBUser struct {
	ID          string
	PublicID    string
	DisplayName string
	PrivacyMode string
}

func (s *Service) SearchFriendsUsers(userID string, query string) ([]FriendProfile, error) {
	if s == nil || s.db == nil {
		return nil, &AuthError{Code: "service_unavailable", Message: "friend service unavailable"}
	}
	if err := s.requireActiveFriendUser(userID); err != nil {
		return nil, err
	}
	query = strings.TrimSpace(query)
	if len([]rune(query)) < 2 || len([]rune(query)) > 80 {
		return nil, &AuthError{Code: "invalid_search_query", Message: "search query must contain 2 to 80 characters"}
	}
	pattern := "%" + strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(query) + "%"
	rows, err := s.db.Query(`
		SELECT u.public_id, u.display_name, COALESCE(u.username, ''), COALESCE(u.avatar_url, ''), u.created_at,
		       COALESCE(fr.id::text, ''),
		       CASE
		           WHEN fr.status = 'accepted' THEN 'accepted'
		           WHEN fr.status = 'pending' AND fr.requester_user_id = $1 THEN 'outgoing_pending'
		           WHEN fr.status = 'pending' AND fr.recipient_user_id = $1 THEN 'incoming_pending'
		           WHEN fr.status = 'rejected' THEN 'rejected'
		           WHEN fr.status = 'cancelled' THEN 'cancelled'
		           ELSE 'none'
		       END
		FROM users u
		LEFT JOIN LATERAL (
			SELECT request.id, request.status, request.requester_user_id, request.recipient_user_id
			FROM friend_requests request
			WHERE request.user_low_id = LEAST(u.id, $1::uuid)
			  AND request.user_high_id = GREATEST(u.id, $1::uuid)
			ORDER BY CASE WHEN request.status IN ('pending', 'accepted') THEN 0 ELSE 1 END,
			         request.created_at DESC
			LIMIT 1
		) fr ON TRUE
		WHERE u.id <> $1
		  AND u.is_active = TRUE AND u.is_deleted = FALSE
		  AND (u.privacy_mode = 'all' OR (u.privacy_mode = 'friends' AND fr.status = 'accepted'))
			  AND (u.display_name ILIKE $2 ESCAPE '!' OR COALESCE(u.username, '') ILIKE $2 ESCAPE '!')
		ORDER BY u.display_name, u.created_at
		LIMIT 30
	`, userID, pattern)
	if err != nil {
		return nil, fmt.Errorf("search users: %w", err)
	}
	defer rows.Close()

	profiles := make([]FriendProfile, 0)
	for rows.Next() {
		var profile FriendProfile
		if err = rows.Scan(&profile.PublicID, &profile.DisplayName, &profile.Username, &profile.AvatarURL, &profile.JoinedAt, &profile.RequestID, &profile.RelationshipStatus); err != nil {
			return nil, fmt.Errorf("scan search result: %w", err)
		}
		profiles = append(profiles, profile)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate search results: %w", err)
	}
	return profiles, nil
}

func (s *Service) GetFriendProfile(viewerID string, targetPublicID string) (*FriendProfile, error) {
	if s == nil || s.db == nil {
		return nil, &AuthError{Code: "service_unavailable", Message: "friend service unavailable"}
	}
	if err := s.requireActiveFriendUser(viewerID); err != nil {
		return nil, err
	}
	if !uuidPattern.MatchString(targetPublicID) {
		return nil, friendNotFound()
	}
	var profile FriendProfile
	err := s.db.QueryRow(`
		SELECT u.public_id, u.display_name, COALESCE(u.username, ''), COALESCE(u.avatar_url, ''), u.created_at,
		       COALESCE(fr.id::text, ''),
		       CASE
		           WHEN u.id = $1 THEN 'none'
		           WHEN fr.status = 'accepted' THEN 'accepted'
		           WHEN fr.status = 'pending' AND fr.requester_user_id = $1 THEN 'outgoing_pending'
		           WHEN fr.status = 'pending' AND fr.recipient_user_id = $1 THEN 'incoming_pending'
		           WHEN fr.status = 'rejected' THEN 'rejected'
		           WHEN fr.status = 'cancelled' THEN 'cancelled'
		           ELSE 'none'
		       END
		FROM users u
		LEFT JOIN LATERAL (
			SELECT request.id, request.status, request.requester_user_id, request.recipient_user_id
			FROM friend_requests request
			WHERE request.user_low_id = LEAST(u.id, $1::uuid)
			  AND request.user_high_id = GREATEST(u.id, $1::uuid)
			ORDER BY CASE WHEN request.status IN ('pending', 'accepted') THEN 0 ELSE 1 END,
			         request.created_at DESC
			LIMIT 1
		) fr ON TRUE
		WHERE u.public_id = $2 AND u.is_active = TRUE AND u.is_deleted = FALSE
		  AND (u.id = $1 OR u.privacy_mode = 'all' OR (u.privacy_mode = 'friends' AND fr.status = 'accepted'))
	`, viewerID, targetPublicID).Scan(&profile.PublicID, &profile.DisplayName, &profile.Username, &profile.AvatarURL, &profile.JoinedAt, &profile.RequestID, &profile.RelationshipStatus)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, friendNotFound()
	}
	if err != nil {
		return nil, fmt.Errorf("load friend profile: %w", err)
	}
	return &profile, nil
}

func (s *Service) ListFriends(userID string) ([]FriendProfile, error) {
	if s == nil || s.db == nil {
		return nil, &AuthError{Code: "service_unavailable", Message: "friend service unavailable"}
	}
	if err := s.requireActiveFriendUser(userID); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`
		SELECT u.public_id, u.display_name, COALESCE(u.username, ''), COALESCE(u.avatar_url, ''), u.created_at,
		       fr.id::text, 'accepted'
		FROM friend_requests fr
		JOIN users u ON u.id = CASE WHEN fr.requester_user_id = $1 THEN fr.recipient_user_id ELSE fr.requester_user_id END
		WHERE (fr.requester_user_id = $1 OR fr.recipient_user_id = $1)
		  AND fr.status = 'accepted' AND u.is_active = TRUE AND u.is_deleted = FALSE
		ORDER BY u.display_name, fr.created_at
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("load friends: %w", err)
	}
	defer rows.Close()
	profiles := make([]FriendProfile, 0)
	for rows.Next() {
		var profile FriendProfile
		if err = rows.Scan(&profile.PublicID, &profile.DisplayName, &profile.Username, &profile.AvatarURL, &profile.JoinedAt, &profile.RequestID, &profile.RelationshipStatus); err != nil {
			return nil, fmt.Errorf("scan friend: %w", err)
		}
		profiles = append(profiles, profile)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate friends: %w", err)
	}
	return profiles, nil
}

func (s *Service) ListFriendRequests(userID string, direction string) ([]FriendRequest, error) {
	if s == nil || s.db == nil {
		return nil, &AuthError{Code: "service_unavailable", Message: "friend service unavailable"}
	}
	if err := s.requireActiveFriendUser(userID); err != nil {
		return nil, err
	}
	var otherUserColumn string
	var ownerColumn string
	switch direction {
	case "incoming":
		otherUserColumn = "fr.requester_user_id"
		ownerColumn = "fr.recipient_user_id"
	case "outgoing":
		otherUserColumn = "fr.recipient_user_id"
		ownerColumn = "fr.requester_user_id"
	default:
		return nil, &AuthError{Code: "invalid_request_direction", Message: "direction must be incoming or outgoing"}
	}
	rows, err := s.db.Query(fmt.Sprintf(`
		SELECT fr.id, fr.status, fr.created_at, u.public_id, u.display_name,
		       COALESCE(u.username, ''), COALESCE(u.avatar_url, ''), u.created_at
		FROM friend_requests fr
		JOIN users u ON u.id = %s
		WHERE %s = $1 AND fr.status = 'pending'
		  AND u.is_active = TRUE AND u.is_deleted = FALSE
		ORDER BY fr.created_at DESC
	`, otherUserColumn, ownerColumn), userID)
	if err != nil {
		return nil, fmt.Errorf("load %s friend requests: %w", direction, err)
	}
	defer rows.Close()
	requests := make([]FriendRequest, 0)
	for rows.Next() {
		var request FriendRequest
		if err = rows.Scan(&request.ID, &request.Status, &request.CreatedAt, &request.User.PublicID, &request.User.DisplayName, &request.User.Username, &request.User.AvatarURL, &request.User.JoinedAt); err != nil {
			return nil, fmt.Errorf("scan friend request: %w", err)
		}
		if direction == "incoming" {
			request.User.RelationshipStatus = FriendStatusIncomingPending
		} else {
			request.User.RelationshipStatus = FriendStatusOutgoingPending
		}
		request.User.RequestID = request.ID
		requests = append(requests, request)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate friend requests: %w", err)
	}
	return requests, nil
}

func (s *Service) CreateFriendRequest(requesterID string, targetPublicID string) (*FriendRequest, error) {
	if s == nil || s.db == nil {
		return nil, &AuthError{Code: "service_unavailable", Message: "friend service unavailable"}
	}
	if !uuidPattern.MatchString(targetPublicID) {
		return nil, friendNotFound()
	}

	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin friend request: %w", err)
	}
	defer tx.Rollback()

	target, err := lockFriendTargetTx(tx, requesterID, targetPublicID)
	if err != nil {
		return nil, err
	}
	if target.PrivacyMode == PrivacyHidden {
		return nil, friendNotFound()
	}
	if err = lockFriendPairTx(tx, requesterID, target.ID); err != nil {
		return nil, err
	}
	if err = ensureFriendPairActiveTx(tx, requesterID, target.ID); err != nil {
		return nil, err
	}

	var currentStatus string
	var existingRequester string
	err = tx.QueryRow(`
		SELECT status, requester_user_id
		FROM friend_requests
		WHERE user_low_id = LEAST($1::uuid, $2::uuid) AND user_high_id = GREATEST($1::uuid, $2::uuid)
		  AND status IN ('pending', 'accepted')
		LIMIT 1
	`, requesterID, target.ID).Scan(&currentStatus, &existingRequester)
	if err == nil {
		switch currentStatus {
		case "accepted":
			return nil, &AuthError{Code: "already_friends", Message: "you are already friends"}
		case "pending":
			if existingRequester == requesterID {
				return nil, &AuthError{Code: "friend_request_pending", Message: "friend request is already pending"}
			}
			return nil, &AuthError{Code: "incoming_request_exists", Message: "this user has already sent you a friend request"}
		}
		if target.PrivacyMode == PrivacyFriends {
			return nil, friendNotFound()
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("check active friend request: %w", err)
	}
	if target.PrivacyMode == PrivacyFriends {
		return nil, friendNotFound()
	}

	lowID, highID := orderedUserIDs(requesterID, target.ID)
	var request FriendRequest
	request.Status = "pending"
	err = tx.QueryRow(`
		INSERT INTO friend_requests (user_low_id, user_high_id, requester_user_id, recipient_user_id, status)
		VALUES ($1, $2, $3, $4, 'pending')
		RETURNING id, status, created_at
	`, lowID, highID, requesterID, target.ID).Scan(&request.ID, &request.Status, &request.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create friend request: %w", err)
	}
	request.User = FriendProfile{
		PublicID:           target.PublicID,
		DisplayName:        target.DisplayName,
		RelationshipStatus: FriendStatusOutgoingPending,
		RequestID:          request.ID,
	}
	if err = queueFriendNotificationTx(tx, target.ID, "friend_request_received", requesterID); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit friend request: %w", err)
	}
	return &request, nil
}

func (s *Service) RespondFriendRequest(actorID string, requestID string, action string) error {
	if s == nil || s.db == nil {
		return &AuthError{Code: "service_unavailable", Message: "friend service unavailable"}
	}
	if !uuidPattern.MatchString(requestID) {
		return friendRequestNotFound()
	}
	if action != "accept" && action != "reject" && action != "cancel" {
		return &AuthError{Code: "invalid_friend_action", Message: "supported actions: accept, reject, cancel"}
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin friend request response: %w", err)
	}
	defer tx.Rollback()

	var lowID string
	var highID string
	var requesterID string
	var recipientID string
	var status string
	err = tx.QueryRow(`
		SELECT user_low_id, user_high_id, requester_user_id, recipient_user_id, status
		FROM friend_requests WHERE id = $1
	`, requestID).Scan(&lowID, &highID, &requesterID, &recipientID, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return friendRequestNotFound()
	}
	if err != nil {
		return fmt.Errorf("load friend request: %w", err)
	}
	if err = lockFriendPairTx(tx, requesterID, recipientID); err != nil {
		return err
	}
	if err = ensureFriendPairActiveTx(tx, requesterID, recipientID); err != nil {
		return err
	}
	err = tx.QueryRow(`
		SELECT user_low_id, user_high_id, requester_user_id, recipient_user_id, status
		FROM friend_requests WHERE id = $1 FOR UPDATE
	`, requestID).Scan(&lowID, &highID, &requesterID, &recipientID, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return friendRequestNotFound()
	}
	if err != nil {
		return fmt.Errorf("lock friend request: %w", err)
	}
	ownerID := recipientID
	if action == "cancel" {
		ownerID = requesterID
	}
	if ownerID != actorID {
		return friendRequestNotFound()
	}
	if status != "pending" {
		return &AuthError{Code: "friend_request_not_pending", Message: "friend request is no longer pending"}
	}
	var nextStatus string
	var notificationType string
	switch action {
	case "accept":
		nextStatus = "accepted"
		notificationType = "friend_request_accepted"
	case "reject":
		nextStatus = "rejected"
		notificationType = "friend_request_rejected"
	case "cancel":
		nextStatus = "cancelled"
	}
	if _, err = tx.Exec(`
		UPDATE friend_requests
		SET status = $1, updated_at = NOW(), responded_at = CASE WHEN $1 IN ('accepted', 'rejected') THEN NOW() ELSE responded_at END,
		    ended_at = CASE WHEN $1 = 'cancelled' THEN NOW() ELSE ended_at END
		WHERE id = $2 AND status = 'pending'
	`, nextStatus, requestID); err != nil {
		return fmt.Errorf("update friend request: %w", err)
	}
	if notificationType != "" {
		if err = queueFriendNotificationTx(tx, requesterID, notificationType, recipientID); err != nil {
			return err
		}
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit friend request response: %w", err)
	}
	return nil
}

func (s *Service) RemoveFriend(userID string, targetPublicID string) error {
	if s == nil || s.db == nil {
		return &AuthError{Code: "service_unavailable", Message: "friend service unavailable"}
	}
	if !uuidPattern.MatchString(targetPublicID) {
		return friendNotFound()
	}
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin remove friend: %w", err)
	}
	defer tx.Rollback()
	target, err := lockFriendTargetTx(tx, userID, targetPublicID)
	if err != nil {
		return err
	}
	if err = lockFriendPairTx(tx, userID, target.ID); err != nil {
		return err
	}
	if err = ensureFriendPairActiveTx(tx, userID, target.ID); err != nil {
		return err
	}
	result, err := tx.Exec(`
		UPDATE friend_requests
		SET status = 'cancelled', ended_at = NOW(), updated_at = NOW()
		WHERE user_low_id = LEAST($1::uuid, $2::uuid) AND user_high_id = GREATEST($1::uuid, $2::uuid)
		  AND status = 'accepted'
	`, userID, target.ID)
	if err != nil {
		return fmt.Errorf("remove friend: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check removed friend: %w", err)
	}
	if changed != 1 {
		return friendNotFound()
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit friend removal: %w", err)
	}
	return nil
}

func (s *Service) GetFriendPrivacySettings(userID string) (*FriendPrivacySettings, error) {
	if s == nil || s.db == nil {
		return nil, &AuthError{Code: "service_unavailable", Message: "privacy settings unavailable"}
	}
	var settings FriendPrivacySettings
	err := s.db.QueryRow(`
		SELECT privacy_mode, friend_notifications_enabled FROM users
		WHERE id = $1 AND is_active = TRUE AND is_deleted = FALSE
	`, userID).Scan(&settings.Visibility, &settings.FriendNotificationsEnabled)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrAccountBlocked
	}
	if err != nil {
		return nil, fmt.Errorf("load friend privacy settings: %w", err)
	}
	return &settings, nil
}

func (s *Service) requireActiveFriendUser(userID string) error {
	var active bool
	err := s.db.QueryRow(`
		SELECT EXISTS(SELECT 1 FROM users WHERE id = $1 AND is_active = TRUE AND is_deleted = FALSE)
	`, userID).Scan(&active)
	if err != nil {
		return fmt.Errorf("check friend user access: %w", err)
	}
	if !active {
		return ErrAccountBlocked
	}
	return nil
}

func (s *Service) UpdateFriendPrivacySettings(userID string, settings FriendPrivacySettings) error {
	if s == nil || s.db == nil {
		return &AuthError{Code: "service_unavailable", Message: "privacy settings unavailable"}
	}
	if settings.Visibility != PrivacyAll && settings.Visibility != PrivacyFriends && settings.Visibility != PrivacyHidden {
		return &AuthError{Code: "invalid_privacy_mode", Message: "visibility must be all, friends, or hidden"}
	}
	result, err := s.db.Exec(`
		UPDATE users
		SET privacy_mode = $2, friend_notifications_enabled = $3, updated_at = NOW()
		WHERE id = $1 AND is_active = TRUE AND is_deleted = FALSE
	`, userID, settings.Visibility, settings.FriendNotificationsEnabled)
	if err != nil {
		return fmt.Errorf("update friend privacy settings: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check updated privacy settings: %w", err)
	}
	if changed != 1 {
		return ErrAccountBlocked
	}
	return nil
}

func lockFriendTargetTx(tx *sql.Tx, actorID string, targetPublicID string) (*friendDBUser, error) {
	var target friendDBUser
	err := tx.QueryRow(`
		SELECT id, public_id, display_name, privacy_mode
		FROM users WHERE public_id = $1 AND id <> $2
	`, targetPublicID, actorID).Scan(&target.ID, &target.PublicID, &target.DisplayName, &target.PrivacyMode)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, friendNotFound()
	}
	if err != nil {
		return nil, fmt.Errorf("load friend target: %w", err)
	}
	if err = lockFriendPairTx(tx, actorID, target.ID); err != nil {
		return nil, err
	}
	if err = tx.QueryRow(`
		SELECT id, public_id, display_name, privacy_mode
		FROM users WHERE id = $1 AND is_active = TRUE AND is_deleted = FALSE
	`, target.ID).Scan(&target.ID, &target.PublicID, &target.DisplayName, &target.PrivacyMode); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, friendNotFound()
		}
		return nil, fmt.Errorf("reload friend target: %w", err)
	}
	return &target, nil
}

func lockFriendPairTx(tx *sql.Tx, firstUserID string, secondUserID string) error {
	rows, err := tx.Query(`
		SELECT id FROM users WHERE id IN ($1, $2) ORDER BY id FOR UPDATE
	`, firstUserID, secondUserID)
	if err != nil {
		return fmt.Errorf("lock friend users: %w", err)
	}
	defer rows.Close()
	locked := 0
	for rows.Next() {
		var userID string
		if err = rows.Scan(&userID); err != nil {
			return fmt.Errorf("scan locked friend user: %w", err)
		}
		locked++
	}
	if err = rows.Err(); err != nil {
		return fmt.Errorf("iterate locked friend users: %w", err)
	}
	if locked != 2 {
		return friendNotFound()
	}
	return nil
}

func ensureFriendPairActiveTx(tx *sql.Tx, firstUserID string, secondUserID string) error {
	var activeUsers int
	if err := tx.QueryRow(`
		SELECT COUNT(*) FROM users
		WHERE id IN ($1, $2) AND is_active = TRUE AND is_deleted = FALSE
	`, firstUserID, secondUserID).Scan(&activeUsers); err != nil {
		return fmt.Errorf("check friend user access: %w", err)
	}
	if activeUsers != 2 {
		return friendNotFound()
	}
	return nil
}

func orderedUserIDs(firstUserID string, secondUserID string) (string, string) {
	if firstUserID < secondUserID {
		return firstUserID, secondUserID
	}
	return secondUserID, firstUserID
}

func queueFriendNotificationTx(tx *sql.Tx, userID string, notificationType string, actorID string) error {
	var actorName string
	if err := tx.QueryRow(`SELECT display_name FROM users WHERE id = $1`, actorID).Scan(&actorName); err != nil {
		return fmt.Errorf("load friend notification actor: %w", err)
	}
	payload, err := json.Marshal(map[string]string{"actor_name": actorName})
	if err != nil {
		return fmt.Errorf("encode friend notification: %w", err)
	}
	if _, err = tx.Exec(`
		INSERT INTO notification_jobs (user_id, notification_type, status, payload)
		SELECT id, $2, 'queued', $3::jsonb
		FROM users
		WHERE id = $1 AND friend_notifications_enabled = TRUE AND is_active = TRUE AND is_deleted = FALSE
	`, userID, notificationType, string(payload)); err != nil {
		return fmt.Errorf("queue friend notification: %w", err)
	}
	return nil
}

func friendNotFound() *AuthError {
	return &AuthError{Code: "user_not_found", Message: "user is not available"}
}

func friendRequestNotFound() *AuthError {
	return &AuthError{Code: "friend_request_not_found", Message: "friend request is not available"}
}
