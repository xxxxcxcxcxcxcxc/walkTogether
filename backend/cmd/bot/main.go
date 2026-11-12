package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"friendwalk/internal/config"
	"friendwalk/internal/db"
)

const telegramBotProcessLockID int64 = 72845190234117

func main() {
	cfg := config.Load()
	bot, err := newTelegramBot(cfg.TelegramBotToken, cfg.TelegramBotUsername, cfg.TelegramMiniAppURL)
	if err != nil {
		log.Fatal(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	connection, err := db.Connect(cfg.PostgresDSN)
	if err != nil {
		log.Fatalf("connect bot database: %v", err)
	}
	defer connection.Close()
	lockConnection, err := connection.Conn(ctx)
	if err != nil {
		log.Fatalf("reserve bot process lock connection: %v", err)
	}
	defer lockConnection.Close()
	var lockAcquired bool
	if err = lockConnection.QueryRowContext(ctx, `SELECT pg_try_advisory_lock($1)`, telegramBotProcessLockID).Scan(&lockAcquired); err != nil {
		log.Fatalf("acquire Telegram bot process lock: %v", err)
	}
	if !lockAcquired {
		log.Fatal("another Telegram bot process is already running; polling and webhook modes must not run concurrently")
	}

	switch strings.ToLower(strings.TrimSpace(cfg.TelegramBotMode)) {
	case "polling":
		if err = bot.runPolling(ctx); err != nil && !errors.Is(err, context.Canceled) {
			log.Fatalf("telegram polling stopped: %v", err)
		}
	case "webhook":
		if err = validateWebhookConfig(cfg.TelegramWebhookURL, cfg.TelegramWebhookSecret); err != nil {
			log.Fatal(err)
		}
		if err = bot.setWebhook(ctx, cfg.TelegramWebhookURL, cfg.TelegramWebhookSecret); err != nil {
			log.Fatalf("configure Telegram webhook: %v", err)
		}

		webhookURL, _ := url.Parse(cfg.TelegramWebhookURL)
		webhookPath := webhookURL.Path
		mux := http.NewServeMux()
		mux.Handle(webhookPath, bot.webhookHandler(cfg.TelegramWebhookSecret, webhookPath))
		server := &http.Server{
			Addr:              cfg.TelegramBotListenAddr,
			Handler:           mux,
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       10 * time.Second,
			WriteTimeout:      15 * time.Second,
			IdleTimeout:       30 * time.Second,
		}
		serverErrors := make(chan error, 1)
		go func() {
			log.Printf("starting Telegram webhook listener on %s", server.Addr)
			serverErrors <- server.ListenAndServe()
		}()

		select {
		case <-ctx.Done():
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err = server.Shutdown(shutdownCtx); err != nil {
				log.Printf("shutdown Telegram webhook listener: %v", err)
			}
		case err = <-serverErrors:
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Fatalf("Telegram webhook listener failed: %v", err)
			}
		}
	default:
		log.Fatalf("unsupported TELEGRAM_BOT_MODE %q; expected polling or webhook", cfg.TelegramBotMode)
	}
}
