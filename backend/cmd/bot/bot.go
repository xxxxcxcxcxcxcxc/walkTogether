package main

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const telegramAPIBaseURL = "https://api.telegram.org"
const processedUpdateLimit = 4096

type telegramBot struct {
	token       string
	username    string
	miniAppURL  string
	apiBaseURL  string
	httpClient  *http.Client
	updateMu    sync.Mutex
	processed   map[int64]struct{}
	updateOrder []int64
}

type telegramUpdate struct {
	UpdateID int64            `json:"update_id"`
	Message  *telegramMessage `json:"message"`
}

type telegramMessage struct {
	Text string       `json:"text"`
	Chat telegramChat `json:"chat"`
}

type telegramChat struct {
	ID int64 `json:"id"`
}

type telegramAPIResponse struct {
	OK          bool            `json:"ok"`
	Result      json.RawMessage `json:"result"`
	Description string          `json:"description"`
}

type sendMessageRequest struct {
	ChatID      int64           `json:"chat_id"`
	Text        string          `json:"text"`
	ReplyMarkup *inlineKeyboard `json:"reply_markup,omitempty"`
}

type inlineKeyboard struct {
	InlineKeyboard [][]inlineKeyboardButton `json:"inline_keyboard"`
}

type inlineKeyboardButton struct {
	Text string `json:"text"`
	URL  string `json:"url"`
}

func newTelegramBot(token string, username string, miniAppURL string) (*telegramBot, error) {
	if strings.TrimSpace(token) == "" {
		return nil, errors.New("TELEGRAM_BOT_TOKEN must be configured")
	}
	username = strings.TrimPrefix(strings.TrimSpace(username), "@")
	if username == "" {
		return nil, errors.New("TELEGRAM_BOT_USERNAME must be configured")
	}
	if _, err := validateMiniAppURL(miniAppURL, username); err != nil {
		return nil, err
	}
	return &telegramBot{
		token:      token,
		username:   username,
		miniAppURL: miniAppURL,
		apiBaseURL: telegramAPIBaseURL,
		httpClient: &http.Client{Timeout: 45 * time.Second},
		processed:  make(map[int64]struct{}),
	}, nil
}

func (b *telegramBot) processUpdate(ctx context.Context, update telegramUpdate) error {
	if update.UpdateID <= 0 {
		return b.handleUpdate(ctx, update)
	}

	b.updateMu.Lock()
	defer b.updateMu.Unlock()
	if _, exists := b.processed[update.UpdateID]; exists {
		return nil
	}
	if err := b.handleUpdate(ctx, update); err != nil {
		return err
	}
	b.processed[update.UpdateID] = struct{}{}
	b.updateOrder = append(b.updateOrder, update.UpdateID)
	if len(b.updateOrder) > processedUpdateLimit {
		delete(b.processed, b.updateOrder[0])
		b.updateOrder = b.updateOrder[1:]
	}
	return nil
}

func (b *telegramBot) handleUpdate(ctx context.Context, update telegramUpdate) error {
	if update.Message == nil || update.Message.Chat.ID == 0 {
		return nil
	}

	fields := strings.Fields(update.Message.Text)
	if len(fields) == 0 {
		return b.sendMessage(ctx, update.Message.Chat.ID, "Чтобы присоединиться к FriendWalk, откройте ссылку-приглашение.", nil)
	}
	command := strings.ToLower(strings.SplitN(fields[0], "@", 2)[0])
	if command != "/start" {
		return b.sendMessage(ctx, update.Message.Chat.ID, "Неизвестная команда. Откройте ссылку-приглашение, чтобы продолжить.", nil)
	}
	if len(fields) == 1 {
		return b.sendMessage(ctx, update.Message.Chat.ID, "Добро пожаловать в FriendWalk. Для регистрации откройте действующую ссылку-приглашение.", nil)
	}
	if len(fields) != 2 || !validInvitePayload(fields[1]) {
		return b.sendMessage(ctx, update.Message.Chat.ID, "Ссылка-приглашение некорректна или повреждена.", nil)
	}

	miniAppLink, err := buildMiniAppLink(b.miniAppURL, b.username, fields[1])
	if err != nil {
		return err
	}
	markup := &inlineKeyboard{
		InlineKeyboard: [][]inlineKeyboardButton{{{
			Text: "Открыть FriendWalk",
			URL:  miniAppLink,
		}}},
	}
	return b.sendMessage(ctx, update.Message.Chat.ID, "Вас пригласили в FriendWalk. Откройте Mini App, чтобы продолжить регистрацию.", markup)
}

func validInvitePayload(payload string) bool {
	if len(payload) == 0 || len(payload) > 64 {
		return false
	}
	for _, char := range payload {
		if (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') && (char < '0' || char > '9') && char != '_' && char != '-' {
			return false
		}
	}
	return true
}

func validateMiniAppURL(rawURL string, username string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Scheme != "https" || !strings.EqualFold(parsed.Hostname(), "t.me") || parsed.User != nil || parsed.Fragment != "" {
		return nil, errors.New("TELEGRAM_MINI_APP_URL must be an HTTPS Telegram Mini App direct link on t.me")
	}
	path := strings.Trim(parsed.EscapedPath(), "/")
	segments := strings.Split(path, "/")
	if len(segments) == 0 || len(segments) > 2 || !strings.EqualFold(segments[0], strings.TrimPrefix(username, "@")) {
		return nil, errors.New("TELEGRAM_MINI_APP_URL must point to the configured bot username")
	}
	return parsed, nil
}

func buildMiniAppLink(baseURL string, username string, payload string) (string, error) {
	if !validInvitePayload(payload) {
		return "", errors.New("invalid invitation payload")
	}
	parsed, err := validateMiniAppURL(baseURL, username)
	if err != nil {
		return "", err
	}
	query := parsed.Query()
	query.Set("startapp", payload)
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func (b *telegramBot) sendMessage(ctx context.Context, chatID int64, text string, markup *inlineKeyboard) error {
	return b.call(ctx, "sendMessage", sendMessageRequest{ChatID: chatID, Text: text, ReplyMarkup: markup}, nil)
}

func (b *telegramBot) setWebhook(ctx context.Context, webhookURL string, secret string) error {
	if err := validateWebhookConfig(webhookURL, secret); err != nil {
		return err
	}
	return b.call(ctx, "setWebhook", map[string]any{
		"url":             webhookURL,
		"secret_token":    secret,
		"allowed_updates": []string{"message"},
	}, nil)
}

func validateWebhookConfig(webhookURL string, secret string) error {
	parsed, err := url.Parse(strings.TrimSpace(webhookURL))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" || parsed.RawQuery != "" || parsed.Path == "" || parsed.Path == "/" {
		return errors.New("TELEGRAM_WEBHOOK_URL must be an HTTPS URL with a specific path and no query or fragment")
	}
	if len(secret) == 0 || len(secret) > 256 {
		return errors.New("TELEGRAM_WEBHOOK_SECRET must contain 1 to 256 characters")
	}
	for _, char := range secret {
		if (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') && (char < '0' || char > '9') && char != '_' && char != '-' {
			return errors.New("TELEGRAM_WEBHOOK_SECRET may contain only letters, digits, underscores, and hyphens")
		}
	}
	return nil
}

func (b *telegramBot) webhookHandler(secret string, expectedPath string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if r.URL.Path != expectedPath {
			http.NotFound(w, r)
			return
		}
		provided := r.Header.Get("X-Telegram-Bot-Api-Secret-Token")
		if provided == "" || subtle.ConstantTimeCompare([]byte(provided), []byte(secret)) != 1 {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		var update telegramUpdate
		if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
			http.Error(w, "invalid update", http.StatusBadRequest)
			return
		}
		if err := b.processUpdate(r.Context(), update); err != nil {
			http.Error(w, "update processing failed", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
}

func (b *telegramBot) runPolling(ctx context.Context) error {
	if err := b.call(ctx, "deleteWebhook", map[string]bool{"drop_pending_updates": false}, nil); err != nil {
		return fmt.Errorf("remove existing Telegram webhook: %w", err)
	}

	var offset int64
	for ctx.Err() == nil {
		var updates []telegramUpdate
		if err := b.call(ctx, "getUpdates", map[string]any{
			"offset":          offset,
			"timeout":         30,
			"allowed_updates": []string{"message"},
		}, &updates); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			log.Printf("Telegram getUpdates failed: %v", err)
			if err = waitForRetry(ctx); err != nil {
				return nil
			}
			continue
		}

		for _, update := range updates {
			if err := b.processUpdate(ctx, update); err != nil {
				log.Printf("Telegram update %d failed: %v", update.UpdateID, err)
				if waitErr := waitForRetry(ctx); waitErr != nil {
					return nil
				}
				break
			}
			offset = update.UpdateID + 1
		}
	}
	return nil
}

func waitForRetry(ctx context.Context) error {
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (b *telegramBot) call(ctx context.Context, method string, payload any, result any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode Telegram %s request: %w", method, err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(b.apiBaseURL, "/")+"/bot"+b.token+"/"+method, strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("create Telegram %s request: %w", method, err)
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := b.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("call Telegram %s: %w", method, err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("read Telegram %s response: %w", method, err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("Telegram %s returned HTTP %d", method, response.StatusCode)
	}

	var apiResponse telegramAPIResponse
	if err = json.Unmarshal(responseBody, &apiResponse); err != nil {
		return fmt.Errorf("decode Telegram %s response: %w", method, err)
	}
	if !apiResponse.OK {
		return fmt.Errorf("Telegram %s failed: %s", method, apiResponse.Description)
	}
	if result != nil && len(apiResponse.Result) > 0 {
		if err = json.Unmarshal(apiResponse.Result, result); err != nil {
			return fmt.Errorf("decode Telegram %s result: %w", method, err)
		}
	}
	return nil
}
