package auth

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrUnauthorized = errors.New("unauthorized")

var ErrInvitationRequired = &AuthError{Code: "invitation_required", Message: "registration requires a valid invitation"}

var ErrAccountBlocked = &AuthError{Code: "account_blocked", Message: "account is inactive or blocked"}

// AuthError represents a controlled authentication failure that should be reported to the client.
type AuthError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *AuthError) Error() string {
	if e == nil {
		return "unknown auth error"
	}
	return e.Message
}

// User is the internal user model used for authenticated API calls.
type User struct {
	ID          string    `json:"id"`
	TelegramID  int64     `json:"telegram_id"`
	DisplayName string    `json:"display_name"`
	Username    string    `json:"username,omitempty"`
	AvatarURL   string    `json:"avatar_url,omitempty"`
	Role        string    `json:"role"`
	IsAdmin     bool      `json:"is_admin"`
	IsActive    bool      `json:"is_active"`
	IsDeleted   bool      `json:"is_deleted"`
	CreatedAt   time.Time `json:"created_at"`
}

// AuthenticatedUser is the result of a successful Telegram login.
type AuthenticatedUser struct {
	User         User   `json:"user"`
	SessionToken string `json:"session_token,omitempty"`
}

// Service is the server-side authentication boundary for Telegram Mini App requests.
type Service struct {
	db             *sql.DB
	botToken       string
	botUsername    string
	initDataMaxAge time.Duration
	sessionTTL     time.Duration
	invitationTTL  time.Duration
	cookieName     string
}

func NewService(db *sql.DB, botToken string, initDataMaxAge, sessionTTL time.Duration, cookieName string, invitationTTL time.Duration, botUsername ...string) *Service {
	if cookieName == "" {
		cookieName = "friendwalk_session"
	}
	if invitationTTL <= 0 {
		invitationTTL = 7 * 24 * time.Hour
	}
	username := ""
	if len(botUsername) > 0 {
		username = strings.TrimSpace(botUsername[0])
	}
	return &Service{
		db:             db,
		botToken:       botToken,
		botUsername:    username,
		initDataMaxAge: initDataMaxAge,
		sessionTTL:     sessionTTL,
		invitationTTL:  invitationTTL,
		cookieName:     cookieName,
	}
}

func (s *Service) AuthenticateTelegram(initData string) (*AuthenticatedUser, error) {
	if s == nil || s.db == nil {
		return nil, &AuthError{Code: "service_unavailable", Message: "authentication service unavailable"}
	}

	verifiedUser, err := ValidateInitData(initData, s.botToken, s.initDataMaxAge)
	if err != nil {
		return nil, &AuthError{Code: "invalid_init_data", Message: err.Error()}
	}

	user, err := s.findUserByTelegramID(verifiedUser.ID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrInvitationRequired
	}
	if !user.IsActive || user.IsDeleted {
		return nil, ErrAccountBlocked
	}

	token, err := generateSessionToken()
	if err != nil {
		return nil, fmt.Errorf("generate session token: %w", err)
	}

	tokenHash := sha256Hex(token)
	expiresAt := time.Now().UTC().Add(s.sessionTTL)
	if _, err = s.db.Exec(
		`INSERT INTO sessions (user_id, token_hash, expires_at, created_at) VALUES ($1, $2, $3, NOW())`,
		user.ID,
		tokenHash,
		expiresAt,
	); err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}

	return &AuthenticatedUser{User: *user, SessionToken: token}, nil
}

func (s *Service) CurrentUserFromToken(token string) (*User, error) {
	if strings.TrimSpace(token) == "" {
		return nil, ErrUnauthorized
	}

	user, err := s.lookupUserBySessionToken(token)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrUnauthorized
	}
	if !user.IsActive || user.IsDeleted {
		return nil, ErrAccountBlocked
	}
	return user, nil
}

type NotificationSettings struct {
	FriendNotificationsEnabled bool `json:"friend_notifications_enabled"`
	WalkNotificationsEnabled   bool `json:"walk_notifications_enabled"`
}

func (s *Service) UpdateDisplayName(userID string, name string) error {
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 80 {
		return errors.New("display name must contain between 1 and 80 characters")
	}
	result, err := s.db.Exec(`UPDATE users SET display_name=$2,updated_at=NOW() WHERE id=$1 AND is_active=TRUE AND is_deleted=FALSE`, userID, name)
	if err != nil {
		return fmt.Errorf("update display name: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return ErrAccountBlocked
	}
	return nil
}

func (s *Service) GetNotificationSettings(userID string) (*NotificationSettings, error) {
	var settings NotificationSettings
	err := s.db.QueryRow(`SELECT friend_notifications_enabled,walk_notifications_enabled FROM users WHERE id=$1 AND is_active=TRUE AND is_deleted=FALSE`, userID).Scan(&settings.FriendNotificationsEnabled, &settings.WalkNotificationsEnabled)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrAccountBlocked
	}
	if err != nil {
		return nil, fmt.Errorf("load notification settings: %w", err)
	}
	return &settings, nil
}

func (s *Service) UpdateNotificationSettings(userID string, settings NotificationSettings) error {
	result, err := s.db.Exec(`UPDATE users SET friend_notifications_enabled=$2,walk_notifications_enabled=$3,updated_at=NOW() WHERE id=$1 AND is_active=TRUE AND is_deleted=FALSE`, userID, settings.FriendNotificationsEnabled, settings.WalkNotificationsEnabled)
	if err != nil {
		return fmt.Errorf("update notification settings: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return ErrAccountBlocked
	}
	return nil
}

func (s *Service) RevokeToken(token string) error {
	if strings.TrimSpace(token) == "" {
		return ErrUnauthorized
	}
	_, err := s.db.Exec(
		`UPDATE sessions SET revoked_at = NOW() WHERE token_hash = $1 AND revoked_at IS NULL`,
		sha256Hex(token),
	)
	if err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	return nil
}

func (s *Service) RevokeAllUserSessions(token string) error {
	user, err := s.CurrentUserFromToken(token)
	if err != nil {
		return err
	}

	_, err = s.db.Exec(
		`UPDATE sessions SET revoked_at = NOW() WHERE user_id = $1 AND revoked_at IS NULL`,
		user.ID,
	)
	if err != nil {
		return fmt.Errorf("revoke all sessions: %w", err)
	}
	return nil
}

func (s *Service) findUserByTelegramID(telegramID int64) (*User, error) {
	row := s.db.QueryRow(`
		SELECT id, telegram_id, display_name, COALESCE(username, ''), COALESCE(avatar_url, ''),
		       COALESCE(role, 'user') AS role,
		       is_admin, is_active, is_deleted, created_at
		FROM users
		WHERE telegram_id = $1
		LIMIT 1
	`, telegramID)

	var u User
	if err := row.Scan(
		&u.ID,
		&u.TelegramID,
		&u.DisplayName,
		&u.Username,
		&u.AvatarURL,
		&u.Role,
		&u.IsAdmin,
		&u.IsActive,
		&u.IsDeleted,
		&u.CreatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("load user by telegram id: %w", err)
	}

	if u.Role == "" {
		u.Role = "user"
	}
	if u.Role == "admin" {
		u.IsAdmin = true
	}

	return &u, nil
}

func (s *Service) lookupUserBySessionToken(token string) (*User, error) {
	row := s.db.QueryRow(`
		SELECT u.id, u.telegram_id, u.display_name, COALESCE(u.username, ''), COALESCE(u.avatar_url, ''),
		       COALESCE(u.role, 'user') AS role,
		       u.is_admin, u.is_active, u.is_deleted, u.created_at
		FROM sessions s
		JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = $1
		  AND s.revoked_at IS NULL
		  AND s.expires_at > NOW()
		LIMIT 1
	`, sha256Hex(token))

	var u User
	if err := row.Scan(
		&u.ID,
		&u.TelegramID,
		&u.DisplayName,
		&u.Username,
		&u.AvatarURL,
		&u.Role,
		&u.IsAdmin,
		&u.IsActive,
		&u.IsDeleted,
		&u.CreatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("lookup session user: %w", err)
	}

	if u.Role == "" {
		u.Role = "user"
	}
	if u.Role == "admin" {
		u.IsAdmin = true
	}
	return &u, nil
}

func (s *Service) CookieName() string {
	return s.cookieName
}
