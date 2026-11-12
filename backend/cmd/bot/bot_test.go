package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

const testInviteToken = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestStartCommandHandling(t *testing.T) {
	tests := []struct {
		name         string
		text         string
		wantMessage  string
		wantStartApp string
	}{
		{name: "start without payload", text: "/start", wantMessage: "ссылку-приглашение"},
		{name: "valid invitation", text: "/start " + testInviteToken, wantMessage: "Mini App", wantStartApp: testInviteToken},
		{name: "malformed payload", text: "/start bad!token", wantMessage: "некорректна"},
		{name: "extra payload fields", text: "/start " + testInviteToken + " extra", wantMessage: "некорректна"},
		{name: "unknown command", text: "/help", wantMessage: "Неизвестная команда"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var requestBody map[string]any
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !strings.HasSuffix(r.URL.Path, "/sendMessage") {
					t.Errorf("unexpected Telegram method: %s", r.URL.Path)
				}
				if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
					t.Errorf("decode sendMessage request: %v", err)
				}
				_, _ = io.WriteString(w, `{"ok":true,"result":true}`)
			}))
			defer api.Close()

			bot := testBot(t, api.URL)
			if err := bot.handleUpdate(context.Background(), telegramUpdate{Message: &telegramMessage{Text: test.text, Chat: telegramChat{ID: 42}}}); err != nil {
				t.Fatalf("handle update: %v", err)
			}
			if requestBody["chat_id"] != float64(42) || !strings.Contains(requestBody["text"].(string), test.wantMessage) {
				t.Fatalf("unexpected sendMessage payload: %#v", requestBody)
			}

			markup, hasMarkup := requestBody["reply_markup"].(map[string]any)
			if test.wantStartApp == "" {
				if hasMarkup {
					t.Fatalf("unexpected Mini App button: %#v", markup)
				}
				return
			}
			if !hasMarkup {
				t.Fatal("valid invitation did not receive a Mini App button")
			}
			keyboard := markup["inline_keyboard"].([]any)
			button := keyboard[0].([]any)[0].(map[string]any)
			buttonURL := button["url"].(string)
			if !strings.Contains(buttonURL, "https://t.me/friendwalk_bot/friendwalk?") || !strings.Contains(buttonURL, "startapp="+test.wantStartApp) {
				t.Fatalf("button does not point to the Telegram Mini App with startapp: %s", buttonURL)
			}
			if _, isWebAppButton := button["web_app"]; isWebAppButton {
				t.Fatal("expected Telegram direct-link URL button to preserve start_param")
			}
			if _, err := validateMiniAppURL(buttonURL, "friendwalk_bot"); err != nil {
				t.Fatalf("button URL is not a valid Mini App direct link: %v", err)
			}
		})
	}
}

func TestWebhookRequiresSecretAndProcessesUpdate(t *testing.T) {
	var apiCalls int
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiCalls++
		_, _ = io.WriteString(w, `{"ok":true,"result":true}`)
	}))
	defer api.Close()
	bot := testBot(t, api.URL)
	handler := bot.webhookHandler("webhook-test-secret", "/telegram/webhook")
	update := `{"update_id":1,"message":{"text":"/start ` + testInviteToken + `","chat":{"id":42}}}`

	unauthorized := httptest.NewRequest(http.MethodPost, "/telegram/webhook", strings.NewReader(update))
	unauthorizedResponse := httptest.NewRecorder()
	handler.ServeHTTP(unauthorizedResponse, unauthorized)
	if unauthorizedResponse.Code != http.StatusForbidden {
		t.Fatalf("missing webhook secret returned status %d", unauthorizedResponse.Code)
	}
	if apiCalls != 0 {
		t.Fatal("unauthorized webhook request was processed")
	}

	request := httptest.NewRequest(http.MethodPost, "/telegram/webhook", strings.NewReader(update))
	request.Header.Set("X-Telegram-Bot-Api-Secret-Token", "webhook-test-secret")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || apiCalls != 1 {
		t.Fatalf("valid webhook was not processed: status=%d api_calls=%d", response.Code, apiCalls)
	}

	duplicate := httptest.NewRequest(http.MethodPost, "/telegram/webhook", strings.NewReader(update))
	duplicate.Header.Set("X-Telegram-Bot-Api-Secret-Token", "webhook-test-secret")
	duplicateResponse := httptest.NewRecorder()
	handler.ServeHTTP(duplicateResponse, duplicate)
	if duplicateResponse.Code != http.StatusOK || apiCalls != 1 {
		t.Fatalf("duplicate webhook update was processed twice: status=%d api_calls=%d", duplicateResponse.Code, apiCalls)
	}

	wrongPath := httptest.NewRequest(http.MethodPost, "/other/path", strings.NewReader(update))
	wrongPath.Header.Set("X-Telegram-Bot-Api-Secret-Token", "webhook-test-secret")
	wrongPathResponse := httptest.NewRecorder()
	handler.ServeHTTP(wrongPathResponse, wrongPath)
	if wrongPathResponse.Code != http.StatusNotFound {
		t.Fatalf("webhook accepted a non-configured path: %d", wrongPathResponse.Code)
	}
}

func TestWebhookConfigValidation(t *testing.T) {
	if err := validateWebhookConfig("http://example.test/telegram/webhook", "valid_secret"); err == nil {
		t.Fatal("non-HTTPS webhook URL should be rejected")
	}
	if err := validateWebhookConfig("https://example.test/", "valid_secret"); err == nil {
		t.Fatal("webhook URL without a path should be rejected")
	}
	if err := validateWebhookConfig("https://example.test/telegram/webhook", "bad secret"); err == nil {
		t.Fatal("invalid webhook secret characters should be rejected")
	}
	if err := validateWebhookConfig("https://example.test/telegram/webhook", "valid_secret-123"); err != nil {
		t.Fatalf("valid webhook config was rejected: %v", err)
	}
}

func TestSetWebhookUsesConfiguredURLAndSecret(t *testing.T) {
	var payload map[string]any
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/setWebhook") {
			t.Errorf("unexpected Telegram method: %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode setWebhook request: %v", err)
		}
		_, _ = io.WriteString(w, `{"ok":true,"result":true}`)
	}))
	defer api.Close()
	bot := testBot(t, api.URL)

	if err := bot.setWebhook(context.Background(), "https://example.test/telegram/webhook", "webhook-secret-123"); err != nil {
		t.Fatalf("set webhook: %v", err)
	}
	if payload["url"] != "https://example.test/telegram/webhook" || payload["secret_token"] != "webhook-secret-123" {
		t.Fatalf("webhook URL or secret was not configured: %#v", payload)
	}
	allowedUpdates := payload["allowed_updates"].([]any)
	if len(allowedUpdates) != 1 || allowedUpdates[0] != "message" {
		t.Fatalf("unexpected allowed updates: %#v", allowedUpdates)
	}
}

func TestMiniAppURLMustMatchConfiguredBot(t *testing.T) {
	if _, err := validateMiniAppURL("https://example.test/app", "friendwalk_bot"); err == nil {
		t.Fatal("ordinary website URL should not be accepted as a Telegram Mini App link")
	}
	if _, err := validateMiniAppURL("https://t.me/some_other_bot", "friendwalk_bot"); err == nil {
		t.Fatal("Mini App link for another bot should be rejected")
	}
}

func TestBuildMiniAppLinkSupportsMainAndDirectLinkApps(t *testing.T) {
	for _, baseURL := range []string{
		"https://t.me/friendwalk_bot",
		"https://t.me/friendwalk_bot/friendwalk",
	} {
		link, err := buildMiniAppLink(baseURL, "friendwalk_bot", testInviteToken)
		if err != nil {
			t.Fatalf("build Mini App link from %s: %v", baseURL, err)
		}
		parsed, err := url.Parse(link)
		if err != nil || parsed.Query().Get("startapp") != testInviteToken {
			t.Fatalf("Mini App link lost the startapp token: %s (err=%v)", link, err)
		}
	}
	if _, err := validateMiniAppURL("https://t.me/friendwalk_bot/app/extra", "friendwalk_bot"); err == nil {
		t.Fatal("unsupported Mini App path should be rejected")
	}
}

func testBot(t *testing.T, apiBaseURL string) *telegramBot {
	t.Helper()
	bot, err := newTelegramBot("123456:test-token", "friendwalk_bot", "https://t.me/friendwalk_bot/friendwalk")
	if err != nil {
		t.Fatal(err)
	}
	bot.apiBaseURL = apiBaseURL
	bot.httpClient = http.DefaultClient
	return bot
}
