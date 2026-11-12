package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

var ErrInvalidInitData = errors.New("invalid telegram init data")

// TelegramUser is the set of trusted fields extracted from the verified Telegram initData payload.
type TelegramUser struct {
	ID           int64  `json:"id"`
	FirstName    string `json:"first_name,omitempty"`
	LastName     string `json:"last_name,omitempty"`
	Username     string `json:"username,omitempty"`
	LanguageCode string `json:"language_code,omitempty"`
	PhotoURL     string `json:"photo_url,omitempty"`
}

func ValidateInitData(initData string, botToken string, maxAge time.Duration) (TelegramUser, error) {
	if strings.TrimSpace(initData) == "" {
		return TelegramUser{}, ErrInvalidInitData
	}
	if strings.TrimSpace(botToken) == "" {
		return TelegramUser{}, errors.New("telegram bot token is not configured")
	}

	values, err := url.ParseQuery(initData)
	if err != nil {
		return TelegramUser{}, fmt.Errorf("%w: parse init data: %v", ErrInvalidInitData, err)
	}

	hashValue, ok := values["hash"]
	if !ok || len(hashValue) != 1 || strings.TrimSpace(hashValue[0]) == "" {
		return TelegramUser{}, fmt.Errorf("%w: missing hash", ErrInvalidInitData)
	}

	for key, raw := range values {
		if len(raw) > 1 {
			return TelegramUser{}, fmt.Errorf("%w: duplicate parameter %q", ErrInvalidInitData, key)
		}
	}

	dataCheckString, err := buildDataCheckString(values)
	if err != nil {
		return TelegramUser{}, err
	}

	if !isValidHash(hashValue[0], dataCheckString, botToken) {
		return TelegramUser{}, fmt.Errorf("%w: signature mismatch", ErrInvalidInitData)
	}

	authDateValue, ok := values["auth_date"]
	if !ok || len(authDateValue) != 1 {
		return TelegramUser{}, fmt.Errorf("%w: missing auth_date", ErrInvalidInitData)
	}

	authDateUnix, err := strconv.ParseInt(authDateValue[0], 10, 64)
	if err != nil {
		return TelegramUser{}, fmt.Errorf("%w: invalid auth_date", ErrInvalidInitData)
	}

	authDate := time.Unix(authDateUnix, 0).UTC()
	now := time.Now().UTC()
	if authDate.After(now.Add(5*time.Minute)) || now.Sub(authDate) > maxAge {
		return TelegramUser{}, fmt.Errorf("%w: expired auth_date", ErrInvalidInitData)
	}

	userJSON, ok := values["user"]
	if !ok || len(userJSON) != 1 || strings.TrimSpace(userJSON[0]) == "" {
		return TelegramUser{}, fmt.Errorf("%w: missing user payload", ErrInvalidInitData)
	}

	var user TelegramUser
	if err := json.Unmarshal([]byte(userJSON[0]), &user); err != nil {
		return TelegramUser{}, fmt.Errorf("%w: invalid user payload", ErrInvalidInitData)
	}
	if user.ID <= 0 {
		return TelegramUser{}, fmt.Errorf("%w: invalid telegram user id", ErrInvalidInitData)
	}
	if strings.TrimSpace(user.FirstName) == "" && strings.TrimSpace(user.Username) == "" {
		return TelegramUser{}, fmt.Errorf("%w: user data incomplete", ErrInvalidInitData)
	}

	return user, nil
}

func buildDataCheckString(values url.Values) (string, error) {
	keys := make([]string, 0, len(values))
	for key := range values {
		if key == "hash" {
			continue
		}
		keys = append(keys, key)
	}
	if len(keys) == 0 {
		return "", fmt.Errorf("%w: no init data payload", ErrInvalidInitData)
	}

	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		rawValue, ok := values[key]
		if !ok || len(rawValue) != 1 {
			return "", fmt.Errorf("%w: duplicate or malformed parameter %q", ErrInvalidInitData, key)
		}
		parts = append(parts, key+"="+rawValue[0])
	}

	return strings.Join(parts, "\n"), nil
}

func isValidHash(receivedHash string, dataCheckString string, botToken string) bool {
	secretKey := hmac.New(sha256.New, []byte("WebAppData"))
	secretKey.Write([]byte(botToken))
	computed := hmac.New(sha256.New, secretKey.Sum(nil))
	computed.Write([]byte(dataCheckString))

	expectedHash := hex.EncodeToString(computed.Sum(nil))
	return hmac.Equal([]byte(receivedHash), []byte(expectedHash))
}
