package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRenderFriendNotification(t *testing.T) {
	tests := []struct {
		typeName string
		want     string
	}{
		{typeName: "friend_request_received", want: "Анна отправил(а) вам запрос в друзья."},
		{typeName: "friend_request_accepted", want: "Анна принял(а) ваш запрос в друзья."},
		{typeName: "friend_request_rejected", want: "Анна отклонил(а) ваш запрос в друзья."},
		{typeName: "walk_invitation", want: "Анна приглашает вас на прогулку «Вечерняя прогулка». Откройте WalkTogether, чтобы ответить."},
		{typeName: "walk_accepted", want: "Анна принял(а) приглашение на прогулку «Вечерняя прогулка»."},
		{typeName: "walk_confirmed", want: "Прогулка «Вечерняя прогулка» подтверждена."},
		{typeName: "walk_reminder", want: "Напоминание: скоро начнётся прогулка «Вечерняя прогулка»."},
		{typeName: "walk_expired", want: "Предложение прогулки «Вечерняя прогулка» истекло."},
	}
	for _, test := range tests {
		message, err := renderFriendNotification(test.typeName, []byte(`{"actor_name":"Анна","title":"Вечерняя прогулка"}`))
		if err != nil || message != test.want {
			t.Errorf("render %s: got %q, err=%v", test.typeName, message, err)
		}
	}
	if _, err := renderFriendNotification("unknown", []byte(`{"actor_name":"Анна"}`)); err == nil {
		t.Fatal("unsupported notification type should fail")
	}
	if _, err := renderFriendNotification("friend_request_received", []byte(`{}`)); err == nil {
		t.Fatal("notification without actor name should fail")
	}
}

func TestSendTelegramMessageUsesBotAPI(t *testing.T) {
	var requestBody map[string]any
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/bot123456:test-token/sendMessage") {
			t.Errorf("unexpected Telegram request: %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Errorf("decode Telegram request: %v", err)
		}
		_, _ = io.WriteString(w, `{"ok":true,"result":{"message_id":1}}`)
	}))
	defer api.Close()

	if err := sendTelegramMessageToAPI(context.Background(), api.Client(), api.URL, "123456:test-token", 987654, "friend notification"); err != nil {
		t.Fatalf("send Telegram notification: %v", err)
	}
	if requestBody["chat_id"] != float64(987654) || requestBody["text"] != "friend notification" {
		t.Fatalf("unexpected Telegram request payload: %#v", requestBody)
	}
}

func TestSendTelegramMessageRejectsBotAPIError(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"ok":false,"description":"blocked"}`)
	}))
	defer api.Close()
	if err := sendTelegramMessageToAPI(context.Background(), api.Client(), api.URL, "123456:test-token", 987654, "text"); err == nil {
		t.Fatal("Bot API error should be retried by the worker")
	}
}
