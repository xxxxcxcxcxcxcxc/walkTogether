package auth

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

const maxInvitationSlots = 5

var ErrInvitationExpired = &AuthError{Code: "invitation_expired", Message: "invitation has expired"}
var ErrInvitationRevoked = &AuthError{Code: "invitation_revoked", Message: "invitation was revoked"}
var ErrInvitationAlreadyUsed = &AuthError{Code: "invitation_used", Message: "invitation has already been used"}
var ErrInviteLimitReached = &AuthError{Code: "invite_limit_reached", Message: "maximum number of active invitation slots has been reached"}
var ErrForbidden = errors.New("forbidden")

// Invitation represents a single user invite record.
type Invitation struct {
	ID              string     `json:"id"`
	SenderUserID    string     `json:"sender_user_id"`
	RecipientUserID *string    `json:"recipient_user_id,omitempty"`
	Status          string     `json:"status"`
	ExpiresAt       *time.Time `json:"expires_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	AcceptedAt      *time.Time `json:"accepted_at,omitempty"`
	RevokedAt       *time.Time `json:"revoked_at,omitempty"`
	DeepLink        string     `json:"deep_link,omitempty"`
}

func (s *Service) CreateInvitationForUser(userID string) (*Invitation, error) {
	if s == nil || s.db == nil {
		return nil, &AuthError{Code: "service_unavailable", Message: "invitation service unavailable"}
	}
	if strings.TrimSpace(userID) == "" {
		return nil, ErrUnauthorized
	}

	user, err := s.loadUserByID(userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrUnauthorized
	}
	if !user.IsActive || user.IsDeleted {
		return nil, ErrAccountBlocked
	}

	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("start invitation transaction: %w", err)
	}
	defer tx.Rollback()

	var isActive bool
	var isDeleted bool
	if err = tx.QueryRow(`SELECT is_active, is_deleted FROM users WHERE id = $1 FOR UPDATE`, userID).Scan(&isActive, &isDeleted); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUnauthorized
		}
		return nil, fmt.Errorf("lock invitation sender: %w", err)
	}
	if !isActive || isDeleted {
		return nil, ErrAccountBlocked
	}

	if err = s.expirePendingInvitationsTx(tx, userID); err != nil {
		return nil, err
	}

	usedSlots, err := s.countActiveInvitationSlotsTx(tx, userID)
	if err != nil {
		return nil, err
	}
	if usedSlots >= maxInvitationSlots {
		return nil, ErrInviteLimitReached
	}

	token, err := generateInviteToken()
	if err != nil {
		return nil, fmt.Errorf("generate invite token: %w", err)
	}
	tokenHash := sha256Hex(token)
	expiresAt := time.Now().UTC().Add(s.invitationTTLDuration())

	var invitationID string
	var createdAt time.Time
	var updatedAt time.Time
	if err = tx.QueryRow(`
		INSERT INTO invitations (sender_user_id, token_hash, status, expires_at, created_at, updated_at)
		VALUES ($1, $2, 'pending', $3, NOW(), NOW())
		RETURNING id, created_at, updated_at
	`, userID, tokenHash, expiresAt).Scan(&invitationID, &createdAt, &updatedAt); err != nil {
		return nil, fmt.Errorf("create invitation record: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit invitation transaction: %w", err)
	}

	return &Invitation{
		ID:           invitationID,
		SenderUserID: userID,
		Status:       "pending",
		ExpiresAt:    &expiresAt,
		CreatedAt:    createdAt,
		UpdatedAt:    updatedAt,
		DeepLink:     buildInviteDeepLink(s.botUsername, token),
	}, nil
}

func (s *Service) ListInvitationsForUser(userID string) ([]Invitation, error) {
	if s == nil || s.db == nil {
		return nil, &AuthError{Code: "service_unavailable", Message: "invitation service unavailable"}
	}
	if strings.TrimSpace(userID) == "" {
		return nil, ErrUnauthorized
	}

	if _, err := s.loadUserByID(userID); err != nil {
		return nil, err
	}
	if err := s.expirePendingInvitations(userID); err != nil {
		return nil, err
	}

	rows, err := s.db.Query(`
		SELECT id, sender_user_id, recipient_user_id, status, expires_at, created_at, updated_at, accepted_at, revoked_at
		FROM invitations
		WHERE sender_user_id = $1
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("load invitations: %w", err)
	}
	defer rows.Close()

	items := make([]Invitation, 0)
	for rows.Next() {
		var inv Invitation
		var recipientUserID sql.NullString
		var expiresAt sql.NullTime
		var acceptedAt sql.NullTime
		var revokedAt sql.NullTime
		if err := rows.Scan(&inv.ID, &inv.SenderUserID, &recipientUserID, &inv.Status, &expiresAt, &inv.CreatedAt, &inv.UpdatedAt, &acceptedAt, &revokedAt); err != nil {
			return nil, fmt.Errorf("scan invitation: %w", err)
		}
		if recipientUserID.Valid {
			v := recipientUserID.String
			inv.RecipientUserID = &v
		}
		if expiresAt.Valid {
			v := expiresAt.Time
			inv.ExpiresAt = &v
		}
		if acceptedAt.Valid {
			v := acceptedAt.Time
			inv.AcceptedAt = &v
		}
		if revokedAt.Valid {
			v := revokedAt.Time
			inv.RevokedAt = &v
		}
		items = append(items, inv)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate invitations: %w", err)
	}
	return items, nil
}

func (s *Service) RevokeInvitation(userID string, invitationID string) error {
	if s == nil || s.db == nil {
		return &AuthError{Code: "service_unavailable", Message: "invitation service unavailable"}
	}
	if strings.TrimSpace(userID) == "" || strings.TrimSpace(invitationID) == "" {
		return ErrUnauthorized
	}

	result, err := s.db.Exec(`
		UPDATE invitations
		SET status = 'revoked', revoked_at = NOW(), updated_at = NOW()
		WHERE sender_user_id = $1 AND id = $2 AND status = 'pending' AND (expires_at IS NULL OR expires_at > NOW())
	`, userID, invitationID)
	if err != nil {
		return fmt.Errorf("revoke invitation: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count revoked invitation: %w", err)
	}
	if count == 0 {
		return &AuthError{Code: "invitation_not_revocable", Message: "only unused active invitations can be revoked"}
	}
	return nil
}

func (s *Service) ValidateInvitationToken(token string) (*Invitation, error) {
	if strings.TrimSpace(token) == "" {
		return nil, ErrInvitationRequired
	}
	if s == nil || s.db == nil {
		return nil, &AuthError{Code: "service_unavailable", Message: "invitation service unavailable"}
	}

	var inv Invitation
	var recipientUserID sql.NullString
	var expiresAt sql.NullTime
	var acceptedAt sql.NullTime
	var revokedAt sql.NullTime
	if err := s.db.QueryRow(`
		SELECT id, sender_user_id, recipient_user_id, status, expires_at, created_at, updated_at, accepted_at, revoked_at
		FROM invitations
		WHERE token_hash = $1
		LIMIT 1
	`, sha256Hex(token)).Scan(
		&inv.ID, &inv.SenderUserID, &recipientUserID, &inv.Status, &expiresAt, &inv.CreatedAt, &inv.UpdatedAt, &acceptedAt, &revokedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrInvitationRequired
		}
		return nil, fmt.Errorf("load invitation by token: %w", err)
	}
	if recipientUserID.Valid {
		v := recipientUserID.String
		inv.RecipientUserID = &v
	}
	if expiresAt.Valid {
		v := expiresAt.Time
		inv.ExpiresAt = &v
	}
	if acceptedAt.Valid {
		v := acceptedAt.Time
		inv.AcceptedAt = &v
	}
	if revokedAt.Valid {
		v := revokedAt.Time
		inv.RevokedAt = &v
	}

	switch inv.Status {
	case "accepted":
		return nil, ErrInvitationAlreadyUsed
	case "revoked":
		return nil, ErrInvitationRevoked
	case "expired":
		return nil, ErrInvitationExpired
	case "rejected":
		return nil, ErrInvitationRequired
	case "pending":
		if inv.ExpiresAt != nil && inv.ExpiresAt.Before(time.Now().UTC()) {
			if _, err := s.db.Exec(`UPDATE invitations SET status = 'expired', updated_at = NOW() WHERE id = $1`, inv.ID); err != nil {
				return nil, fmt.Errorf("expire invitation: %w", err)
			}
			return nil, ErrInvitationExpired
		}
		return &inv, nil
	default:
		return nil, ErrInvitationRequired
	}
}

func (s *Service) RegisterUserWithInvitation(initData string, inviteToken string) (*AuthenticatedUser, error) {
	if s == nil || s.db == nil {
		return nil, &AuthError{Code: "service_unavailable", Message: "authentication service unavailable"}
	}

	verifiedUser, err := ValidateInitData(initData, s.botToken, s.initDataMaxAge)
	if err != nil {
		return nil, &AuthError{Code: "invalid_init_data", Message: err.Error()}
	}

	existingUser, err := s.findUserByTelegramID(verifiedUser.ID)
	if err != nil {
		return nil, err
	}
	if existingUser != nil {
		if !existingUser.IsActive || existingUser.IsDeleted {
			return nil, ErrAccountBlocked
		}
		return s.issueSessionForUser(existingUser.ID)
	}

	if strings.TrimSpace(inviteToken) == "" {
		return nil, ErrInvitationRequired
	}

	invitation, err := s.ValidateInvitationToken(inviteToken)
	if err != nil {
		return nil, err
	}

	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin registration: %w", err)
	}
	defer tx.Rollback()

	var userID string
	if err = tx.QueryRow(`
		INSERT INTO users (telegram_id, username, display_name, avatar_url, is_admin, is_active, is_deleted, created_at, updated_at)
		VALUES ($1, $2, $3, $4, FALSE, TRUE, FALSE, NOW(), NOW())
		RETURNING id
	`, verifiedUser.ID, verifiedUser.Username, buildDisplayName(verifiedUser), buildAvatarURL(verifiedUser)).Scan(&userID); err != nil {
		return nil, fmt.Errorf("create user record: %w", err)
	}

	result, err := tx.Exec(`
		UPDATE invitations
		SET recipient_user_id = $1, status = 'accepted', accepted_at = NOW(), updated_at = NOW()
		WHERE id = $2 AND status = 'pending' AND (expires_at IS NULL OR expires_at > NOW())
	`, userID, invitation.ID)
	if err != nil {
		return nil, fmt.Errorf("link invitation to user: %w", err)
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("check invitation registration: %w", err)
	}
	if updated != 1 {
		return nil, ErrInvitationAlreadyUsed
	}

	token, err := generateSessionToken()
	if err != nil {
		return nil, fmt.Errorf("generate session token: %w", err)
	}
	expiresAt := time.Now().UTC().Add(s.sessionTTL)
	if _, err = tx.Exec(`
		INSERT INTO sessions (user_id, token_hash, expires_at, created_at)
		VALUES ($1, $2, $3, NOW())
	`, userID, sha256Hex(token), expiresAt); err != nil {
		return nil, fmt.Errorf("create session in transaction: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit registration transaction: %w", err)
	}

	user, err := s.loadUserByID(userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrUnauthorized
	}

	return &AuthenticatedUser{User: *user, SessionToken: token}, nil
}

func (s *Service) ListUsersForAdmin(adminUserID string) ([]User, error) {
	if s == nil || s.db == nil {
		return nil, &AuthError{Code: "service_unavailable", Message: "user service unavailable"}
	}
	if !s.userIsAdmin(adminUserID) {
		return nil, &AuthError{Code: "forbidden", Message: "admin access required"}
	}

	rows, err := s.db.Query(`
		SELECT id, telegram_id, display_name, COALESCE(username, ''), COALESCE(avatar_url, ''), COALESCE(role, 'user') AS role,
		       is_admin, is_active, is_deleted, created_at
		FROM users
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	users := make([]User, 0)
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.TelegramID, &u.DisplayName, &u.Username, &u.AvatarURL, &u.Role, &u.IsAdmin, &u.IsActive, &u.IsDeleted, &u.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		if u.Role == "" {
			u.Role = "user"
		}
		if u.Role == "admin" {
			u.IsAdmin = true
		}
		users = append(users, u)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate users: %w", err)
	}
	return users, nil
}

func (s *Service) RevokeUserAccess(adminUserID string, targetUserID string) error {
	if s == nil || s.db == nil {
		return &AuthError{Code: "service_unavailable", Message: "user service unavailable"}
	}
	if !s.userIsAdmin(adminUserID) {
		return &AuthError{Code: "forbidden", Message: "admin access required"}
	}
	if strings.TrimSpace(targetUserID) == "" {
		return ErrUnauthorized
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin revoke user transaction: %w", err)
	}
	defer tx.Rollback()

	if _, err = tx.Exec(`UPDATE users SET is_active = FALSE, updated_at = NOW() WHERE id = $1`, targetUserID); err != nil {
		return fmt.Errorf("deactivate user: %w", err)
	}
	if _, err = tx.Exec(`UPDATE sessions SET revoked_at = NOW() WHERE user_id = $1 AND revoked_at IS NULL`, targetUserID); err != nil {
		return fmt.Errorf("revoke user sessions: %w", err)
	}
	if _, err = tx.Exec(`
		UPDATE friend_requests
		SET status = 'cancelled', ended_at = NOW(), updated_at = NOW()
		WHERE (requester_user_id = $1 OR recipient_user_id = $1) AND status IN ('pending', 'accepted')
	`, targetUserID); err != nil {
		return fmt.Errorf("end blocked user's friendships: %w", err)
	}
	if _, err = tx.Exec(`UPDATE invitations SET status = 'revoked', revoked_at = NOW(), updated_at = NOW() WHERE recipient_user_id = $1 AND status = 'accepted'`, targetUserID); err != nil {
		return fmt.Errorf("revoke accepted invitation: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit revoke user transaction: %w", err)
	}
	return nil
}

func (s *Service) RestoreUserAccess(adminUserID string, targetUserID string) error {
	if s == nil || s.db == nil {
		return &AuthError{Code: "service_unavailable", Message: "user service unavailable"}
	}
	if !s.userIsAdmin(adminUserID) {
		return &AuthError{Code: "forbidden", Message: "admin access required"}
	}
	if strings.TrimSpace(targetUserID) == "" {
		return ErrUnauthorized
	}

	var hasAcceptedInvite bool
	if err := s.db.QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM invitations WHERE recipient_user_id = $1 AND status IN ('accepted', 'revoked')
		)
	`, targetUserID).Scan(&hasAcceptedInvite); err != nil {
		return fmt.Errorf("check invite history: %w", err)
	}
	if !hasAcceptedInvite {
		return &AuthError{Code: "restore_denied", Message: "user has no valid invite history to restore"}
	}

	if _, err := s.db.Exec(`UPDATE users SET is_active = TRUE, updated_at = NOW() WHERE id = $1`, targetUserID); err != nil {
		return fmt.Errorf("restore user: %w", err)
	}
	if _, err := s.db.Exec(`UPDATE invitations SET status = 'accepted', accepted_at = NOW(), updated_at = NOW() WHERE recipient_user_id = $1 AND status = 'revoked'`, targetUserID); err != nil {
		return fmt.Errorf("restore invite status: %w", err)
	}
	return nil
}

func (s *Service) EnsurePrimaryAdmin(telegramID int64) error {
	if s == nil || s.db == nil || telegramID <= 0 {
		return nil
	}
	_, err := s.db.Exec(`
		UPDATE users
		SET is_admin = TRUE,
		    role = 'admin',
		    updated_at = NOW()
		WHERE telegram_id = $1
	`, telegramID)
	if err != nil {
		return fmt.Errorf("ensure primary admin: %w", err)
	}
	return nil
}

func (s *Service) issueSessionForUser(userID string) (*AuthenticatedUser, error) {
	user, err := s.loadUserByID(userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrUnauthorized
	}
	if !user.IsActive || user.IsDeleted {
		return nil, ErrAccountBlocked
	}

	token, err := generateSessionToken()
	if err != nil {
		return nil, fmt.Errorf("generate session token: %w", err)
	}
	expiresAt := time.Now().UTC().Add(s.sessionTTL)
	if _, err = s.db.Exec(`
		INSERT INTO sessions (user_id, token_hash, expires_at, created_at)
		VALUES ($1, $2, $3, NOW())
	`, userID, sha256Hex(token), expiresAt); err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}
	return &AuthenticatedUser{User: *user, SessionToken: token}, nil
}

func (s *Service) userIsAdmin(userID string) bool {
	if strings.TrimSpace(userID) == "" {
		return false
	}
	var isAdmin bool
	if err := s.db.QueryRow(`SELECT COALESCE(is_admin, FALSE) OR COALESCE(role = 'admin', FALSE) FROM users WHERE id = $1 LIMIT 1`, userID).Scan(&isAdmin); err != nil {
		return false
	}
	return isAdmin
}

func (s *Service) loadUserByID(userID string) (*User, error) {
	row := s.db.QueryRow(`
		SELECT id, telegram_id, display_name, COALESCE(username, ''), COALESCE(avatar_url, ''),
		       COALESCE(role, 'user') AS role,
		       is_admin, is_active, is_deleted, created_at
		FROM users
		WHERE id = $1
		LIMIT 1
	`, userID)

	var u User
	if err := row.Scan(&u.ID, &u.TelegramID, &u.DisplayName, &u.Username, &u.AvatarURL, &u.Role, &u.IsAdmin, &u.IsActive, &u.IsDeleted, &u.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("load user by id: %w", err)
	}
	if u.Role == "" {
		u.Role = "user"
	}
	if u.Role == "admin" {
		u.IsAdmin = true
	}
	return &u, nil
}

func (s *Service) countActiveInvitationSlotsTx(tx *sql.Tx, userID string) (int, error) {
	var count int
	if err := tx.QueryRow(`
		SELECT COUNT(*)
		FROM invitations
		WHERE sender_user_id = $1 AND status IN ('pending', 'accepted')
		  AND (expires_at IS NULL OR expires_at > NOW())
	`, userID).Scan(&count); err != nil {
		return 0, fmt.Errorf("count active invitation slots: %w", err)
	}
	return count, nil
}

func (s *Service) expirePendingInvitations(userID string) error {
	_, err := s.db.Exec(`
		UPDATE invitations
		SET status = 'expired', updated_at = NOW()
		WHERE sender_user_id = $1 AND status = 'pending' AND expires_at IS NOT NULL AND expires_at <= NOW()
	`, userID)
	if err != nil {
		return fmt.Errorf("expire pending invitations: %w", err)
	}
	return nil
}

func (s *Service) expirePendingInvitationsTx(tx *sql.Tx, userID string) error {
	_, err := tx.Exec(`
		UPDATE invitations
		SET status = 'expired', updated_at = NOW()
		WHERE sender_user_id = $1 AND status = 'pending' AND expires_at IS NOT NULL AND expires_at <= NOW()
	`, userID)
	if err != nil {
		return fmt.Errorf("expire pending invitations in transaction: %w", err)
	}
	return nil
}

func (s *Service) invitationTTLDuration() time.Duration {
	if s == nil || s.invitationTTL <= 0 {
		return 7 * 24 * time.Hour
	}
	return s.invitationTTL
}

func buildInviteDeepLink(botUsername string, token string) string {
	username := strings.TrimSpace(botUsername)
	if username == "" {
		username = "friendwalk_bot"
	}
	return fmt.Sprintf("https://t.me/%s?start=%s", username, token)
}

func buildDisplayName(user TelegramUser) string {
	if strings.TrimSpace(user.FirstName) != "" && strings.TrimSpace(user.LastName) != "" {
		return strings.TrimSpace(user.FirstName) + " " + strings.TrimSpace(user.LastName)
	}
	if strings.TrimSpace(user.FirstName) != "" {
		return strings.TrimSpace(user.FirstName)
	}
	if strings.TrimSpace(user.LastName) != "" {
		return strings.TrimSpace(user.LastName)
	}
	if strings.TrimSpace(user.Username) != "" {
		return strings.TrimSpace(user.Username)
	}
	return "Telegram User"
}

func buildAvatarURL(user TelegramUser) string {
	return strings.TrimSpace(user.PhotoURL)
}

func generateInviteToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
