package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config keeps the application configuration for the API runtime.
type Config struct {
	AppEnv                string
	Port                  string
	JWTSecret             string
	PostgresDSN           string
	TelegramBotToken      string
	TelegramBotUsername   string
	TelegramBotMode       string
	TelegramMiniAppURL    string
	TelegramWebhookURL    string
	TelegramWebhookSecret string
	TelegramBotListenAddr string
	MainAdminTelegramID   int64
	LogLevel              string
	InitDataMaxAgeSeconds int
	SessionTTLSeconds     int
	InvitationTTLHours    int
	SessionCookieName     string
}

func Load() Config {
	cfg := Config{
		AppEnv:                getEnv("APP_ENV", "development"),
		Port:                  getEnv("PORT", "8080"),
		JWTSecret:             getEnv("JWT_SECRET", "change-me"),
		PostgresDSN:           getEnv("POSTGRES_DSN", "postgres://postgres:postgres@localhost:5432/friendwalk?sslmode=disable"),
		TelegramBotToken:      getEnv("TELEGRAM_BOT_TOKEN", ""),
		TelegramBotUsername:   getEnv("TELEGRAM_BOT_USERNAME", "friendwalk_bot"),
		TelegramBotMode:       getEnv("TELEGRAM_BOT_MODE", "polling"),
		TelegramMiniAppURL:    getEnv("TELEGRAM_MINI_APP_URL", "https://t.me/friendwalk_bot"),
		TelegramWebhookURL:    getEnv("TELEGRAM_WEBHOOK_URL", ""),
		TelegramWebhookSecret: getEnv("TELEGRAM_WEBHOOK_SECRET", ""),
		TelegramBotListenAddr: getEnv("TELEGRAM_BOT_LISTEN_ADDR", ":8081"),
		MainAdminTelegramID:   getEnvInt64("MAIN_ADMIN_TELEGRAM_ID", 0),
		LogLevel:              getEnv("LOG_LEVEL", "info"),
		InitDataMaxAgeSeconds: getEnvInt("INIT_DATA_MAX_AGE_SECONDS", 300),
		SessionTTLSeconds:     getEnvInt("SESSION_TTL_SECONDS", 30*24*60*60),
		InvitationTTLHours:    getEnvInt("INVITATION_TTL_HOURS", 168),
		SessionCookieName:     getEnv("SESSION_COOKIE_NAME", "friendwalk_session"),
	}

	if cfg.JWTSecret == "change-me" && cfg.AppEnv == "production" {
		panic("JWT_SECRET must be set in production")
	}

	return cfg
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok && value != "" {
		return value
	}
	return fallback
}

func (c Config) Addr() string {
	return fmt.Sprintf(":%s", c.Port)
}

func (c Config) InitDataMaxAge() time.Duration {
	return time.Duration(c.InitDataMaxAgeSeconds) * time.Second
}

func (c Config) SessionTTL() time.Duration {
	return time.Duration(c.SessionTTLSeconds) * time.Second
}

func (c Config) InvitationTTL() time.Duration {
	return time.Duration(c.InvitationTTLHours) * time.Hour
}

func getEnvInt(key string, fallback int) int {
	value, ok := os.LookupEnv(key)
	if !ok || value == "" {
		return fallback
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getEnvInt64(key string, fallback int64) int64 {
	value, ok := os.LookupEnv(key)
	if !ok || value == "" {
		return fallback
	}

	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return fallback
	}
	return parsed
}
