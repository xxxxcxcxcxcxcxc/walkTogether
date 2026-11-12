package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"testing"
	"time"
)

func buildInitData(t *testing.T, botToken string, params map[string]string, authDate time.Time) string {
	t.Helper()

	pairs := make([]string, 0, len(params)+2)
	for key, value := range params {
		pairs = append(pairs, fmt.Sprintf("%s=%s", key, value))
	}
	pairs = append(pairs, fmt.Sprintf("auth_date=%d", authDate.Unix()))

	sort.Strings(pairs)
	dataCheckString := strings.Join(pairs, "\n")
	secretKey := hmac.New(sha256.New, []byte("WebAppData"))
	secretKey.Write([]byte(botToken))
	mac := hmac.New(sha256.New, secretKey.Sum(nil))
	mac.Write([]byte(dataCheckString))

	query := make(url.Values)
	for key, value := range params {
		query.Set(key, value)
	}
	query.Set("auth_date", fmt.Sprintf("%d", authDate.Unix()))
	query.Set("hash", hex.EncodeToString(mac.Sum(nil)))

	return query.Encode()
}

func TestValidateInitDataSuccess(t *testing.T) {
	botToken := "123456:ABCDEF"
	userJSON := `{"id":12345,"first_name":"Ivan","last_name":"Petrov","username":"ivan","language_code":"ru"}`
	initData := buildInitData(t, botToken, map[string]string{"user": userJSON}, time.Now().Add(-5*time.Second))

	user, err := ValidateInitData(initData, botToken, 5*time.Minute)
	if err != nil {
		t.Fatalf("expected valid init data, got error: %v", err)
	}
	if user.ID != 12345 {
		t.Fatalf("expected telegram id 12345, got %d", user.ID)
	}
	if user.Username != "ivan" {
		t.Fatalf("expected username ivan, got %q", user.Username)
	}
}

func TestValidateInitDataRejectsBadSignature(t *testing.T) {
	botToken := "123456:ABCDEF"
	userJSON := `{"id":12345,"first_name":"Ivan","last_name":"Petrov","username":"ivan","language_code":"ru"}`
	initData := buildInitData(t, botToken, map[string]string{"user": userJSON}, time.Now())
	if !strings.Contains(initData, "hash=") {
		t.Fatal("expected hash parameter in init data")
	}
	initData = strings.Replace(initData, "hash=", "hash=bad-", 1)

	if _, err := ValidateInitData(initData, botToken, 5*time.Minute); err == nil {
		t.Fatal("expected invalid signature to be rejected")
	}
}

func TestValidateInitDataRejectsChangedTelegramID(t *testing.T) {
	botToken := "123456:ABCDEF"
	validUserJSON := `{"id":12345,"first_name":"Ivan","last_name":"Petrov","username":"ivan","language_code":"ru"}`
	initData := buildInitData(t, botToken, map[string]string{"user": validUserJSON}, time.Now())

	mutatedUserJSON := `{"id":99999,"first_name":"Ivan","last_name":"Petrov","username":"ivan","language_code":"ru"}`
	mutated := strings.Replace(initData, url.QueryEscape(validUserJSON), url.QueryEscape(mutatedUserJSON), 1)

	if _, err := ValidateInitData(mutated, botToken, 5*time.Minute); err == nil {
		t.Fatal("expected changed telegram id to be rejected")
	}
}

func TestValidateInitDataRejectsExpiredAuthDate(t *testing.T) {
	botToken := "123456:ABCDEF"
	userJSON := `{"id":12345,"first_name":"Ivan","last_name":"Petrov","username":"ivan","language_code":"ru"}`
	initData := buildInitData(t, botToken, map[string]string{"user": userJSON}, time.Now().Add(-10*time.Minute))

	if _, err := ValidateInitData(initData, botToken, 5*time.Minute); err == nil {
		t.Fatal("expected expired auth date to be rejected")
	}
}

func TestValidateInitDataRejectsMissingHash(t *testing.T) {
	botToken := "123456:ABCDEF"
	userJSON := `{"id":12345,"first_name":"Ivan","last_name":"Petrov","username":"ivan","language_code":"ru"}`
	initData := buildInitData(t, botToken, map[string]string{"user": userJSON}, time.Now())
	initData = strings.Replace(initData, "&hash=", "", 1)

	if _, err := ValidateInitData(initData, botToken, 5*time.Minute); err == nil {
		t.Fatal("expected missing hash to be rejected")
	}
}

func TestValidateInitDataRejectsMalformedString(t *testing.T) {
	botToken := "123456:ABCDEF"
	if _, err := ValidateInitData("not-a-valid-init-data", botToken, 5*time.Minute); err == nil {
		t.Fatal("expected malformed init data to be rejected")
	}
}

func TestValidateInitDataRejectsDuplicateQueryKeys(t *testing.T) {
	botToken := "123456:ABCDEF"
	userJSON := `{"id":12345,"first_name":"Ivan","last_name":"Petrov","username":"ivan","language_code":"ru"}`
	initData := buildInitData(t, botToken, map[string]string{"user": userJSON}, time.Now())
	initData += "&user=%7B%22id%22%3A99999%7D"

	if _, err := ValidateInitData(initData, botToken, 5*time.Minute); err == nil {
		t.Fatal("expected duplicate key to be rejected")
	}
}
