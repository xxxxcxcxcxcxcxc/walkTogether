package config

import "testing"

func TestLoadUsesDefaults(t *testing.T) {
	t.Setenv("APP_ENV", "")
	t.Setenv("PORT", "")
	t.Setenv("JWT_SECRET", "")
	t.Setenv("POSTGRES_DSN", "")
	t.Setenv("TELEGRAM_BOT_TOKEN", "")
	t.Setenv("TELEGRAM_BOT_MODE", "")
	t.Setenv("TELEGRAM_MINI_APP_URL", "")
	t.Setenv("TELEGRAM_WEBHOOK_URL", "")
	t.Setenv("TELEGRAM_WEBHOOK_SECRET", "")
	t.Setenv("LOG_LEVEL", "")
	t.Setenv("INIT_DATA_MAX_AGE_SECONDS", "")
	t.Setenv("SESSION_TTL_SECONDS", "")
	t.Setenv("SESSION_COOKIE_NAME", "")

	cfg := Load()

	if cfg.AppEnv != "development" {
		t.Fatalf("expected development environment, got %q", cfg.AppEnv)
	}
	if cfg.Port != "8080" {
		t.Fatalf("expected default port 8080, got %q", cfg.Port)
	}
	if cfg.JWTSecret != "change-me" {
		t.Fatalf("expected default JWT secret, got %q", cfg.JWTSecret)
	}
	if cfg.PostgresDSN == "" {
		t.Fatal("expected default postgres dsn to be populated")
	}
	if cfg.TelegramBotMode != "polling" || cfg.TelegramMiniAppURL != "https://t.me/friendwalk_bot" || cfg.TelegramBotListenAddr != ":8081" {
		t.Fatalf("unexpected Telegram bot defaults: mode=%q mini_app_url=%q listen_addr=%q", cfg.TelegramBotMode, cfg.TelegramMiniAppURL, cfg.TelegramBotListenAddr)
	}
	if cfg.InitDataMaxAgeSeconds != 300 {
		t.Fatalf("expected init data max age to default to 300, got %d", cfg.InitDataMaxAgeSeconds)
	}
	if cfg.SessionTTLSeconds != 30*24*60*60 {
		t.Fatalf("expected session ttl default to 30 days, got %d", cfg.SessionTTLSeconds)
	}
	if cfg.SessionCookieName != "friendwalk_session" {
		t.Fatalf("expected default cookie name, got %q", cfg.SessionCookieName)
	}
}

func TestLoadUsesEnvironmentOverrides(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("PORT", "9000")
	t.Setenv("JWT_SECRET", "super-secret")
	t.Setenv("POSTGRES_DSN", "postgres://user:pass@localhost:5432/friendwalk")
	t.Setenv("TELEGRAM_BOT_TOKEN", "bot-token")
	t.Setenv("TELEGRAM_BOT_MODE", "webhook")
	t.Setenv("TELEGRAM_MINI_APP_URL", "https://t.me/friendwalk_bot/friendwalk")
	t.Setenv("TELEGRAM_WEBHOOK_URL", "https://example.test/telegram/webhook")
	t.Setenv("TELEGRAM_WEBHOOK_SECRET", "hook-secret")
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("INIT_DATA_MAX_AGE_SECONDS", "600")
	t.Setenv("SESSION_TTL_SECONDS", "7200")
	t.Setenv("SESSION_COOKIE_NAME", "app_session")

	cfg := Load()

	if cfg.AppEnv != "production" {
		t.Fatalf("expected production environment, got %q", cfg.AppEnv)
	}
	if cfg.Port != "9000" {
		t.Fatalf("expected port 9000, got %q", cfg.Port)
	}
	if cfg.JWTSecret != "super-secret" {
		t.Fatalf("expected env JWT secret, got %q", cfg.JWTSecret)
	}
	if cfg.TelegramBotToken != "bot-token" {
		t.Fatalf("expected bot token override, got %q", cfg.TelegramBotToken)
	}
	if cfg.TelegramBotMode != "webhook" || cfg.TelegramMiniAppURL != "https://t.me/friendwalk_bot/friendwalk" || cfg.TelegramWebhookURL != "https://example.test/telegram/webhook" || cfg.TelegramWebhookSecret != "hook-secret" {
		t.Fatalf("unexpected Telegram bot config: mode=%q mini_app_url=%q webhook_url=%q", cfg.TelegramBotMode, cfg.TelegramMiniAppURL, cfg.TelegramWebhookURL)
	}
	if cfg.LogLevel != "debug" {
		t.Fatalf("expected debug log level, got %q", cfg.LogLevel)
	}
	if cfg.InitDataMaxAgeSeconds != 600 {
		t.Fatalf("expected init data max age override, got %d", cfg.InitDataMaxAgeSeconds)
	}
	if cfg.SessionTTLSeconds != 7200 {
		t.Fatalf("expected session ttl override, got %d", cfg.SessionTTLSeconds)
	}
	if cfg.SessionCookieName != "app_session" {
		t.Fatalf("expected session cookie override, got %q", cfg.SessionCookieName)
	}
}
