package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"friendwalk/internal/config"
	"friendwalk/internal/db"
)

func main() {
	cfg := config.Load()
	if cfg.TelegramBotToken == "" {
		log.Fatal("TELEGRAM_BOT_TOKEN must be configured to send friend notifications")
	}
	connection, err := db.Connect(cfg.PostgresDSN)
	if err != nil {
		log.Fatalf("connect worker database: %v", err)
	}
	defer connection.Close()

	concurrency := workerConcurrency()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	client := &http.Client{Timeout: 10 * time.Second}
	log.Printf("starting FriendWalk notification worker with concurrency %d", concurrency)
	for workerID := 0; workerID < concurrency; workerID++ {
		go workerLoop(ctx, connection, client, cfg.TelegramBotToken, workerID+1)
	}
	<-ctx.Done()
}

func workerConcurrency() int {
	concurrency := 5
	if raw := os.Getenv("WORKER_CONCURRENCY"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 32 {
			log.Fatal("WORKER_CONCURRENCY must be an integer from 1 to 32")
		}
		concurrency = parsed
	}
	return concurrency
}

func workerLoop(ctx context.Context, connection *sql.DB, client *http.Client, botToken string, workerID int) {
	for ctx.Err() == nil {
		processed, err := processOneNotification(ctx, connection, client, botToken)
		if err != nil {
			log.Printf("notification worker %d: %v", workerID, err)
		}
		if !processed {
			timer := time.NewTimer(time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
	}
}

type notificationJob struct {
	ID               string
	UserID           string
	NotificationType string
	Payload          []byte
	Retries          int
}

func processOneNotification(ctx context.Context, connection *sql.DB, client *http.Client, botToken string) (bool, error) {
	tx, err := connection.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	var job notificationJob
	err = tx.QueryRowContext(ctx, `
		WITH candidate AS (
			SELECT id
			FROM notification_jobs
			WHERE notification_type IN ('friend_request_received', 'friend_request_accepted', 'friend_request_rejected',
			  'walk_invitation', 'walk_accepted', 'walk_rejected', 'walk_confirmed', 'walk_rescheduled', 'walk_cancelled', 'walk_reminder', 'walk_expired')
			  AND ((status = 'queued' AND (scheduled_for IS NULL OR scheduled_for <= NOW()))
			       OR (status = 'processing' AND updated_at < NOW() - INTERVAL '5 minutes'))
			ORDER BY created_at, id
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		UPDATE notification_jobs AS jobs
		SET status = 'processing', updated_at = NOW()
		FROM candidate
		WHERE jobs.id = candidate.id
		RETURNING jobs.id, jobs.user_id, jobs.notification_type, jobs.payload, jobs.retries
	`).Scan(&job.ID, &job.UserID, &job.NotificationType, &job.Payload, &job.Retries)
	if errors.Is(err, sql.ErrNoRows) {
		_ = tx.Rollback()
		return false, nil
	}
	if err != nil {
		_ = tx.Rollback()
		return false, err
	}
	if err = tx.Commit(); err != nil {
		return false, err
	}

	var telegramID int64
	var notificationsEnabled bool
	var walkNotificationsEnabled bool
	var isActive bool
	err = connection.QueryRowContext(ctx, `
		SELECT telegram_id, friend_notifications_enabled, walk_notifications_enabled, is_active AND NOT is_deleted
		FROM users WHERE id = $1
	`, job.UserID).Scan(&telegramID, &notificationsEnabled, &walkNotificationsEnabled, &isActive)
	allowed := notificationsEnabled
	if strings.HasPrefix(job.NotificationType, "walk_") {
		allowed = walkNotificationsEnabled
	}
	if errors.Is(err, sql.ErrNoRows) || (err == nil && (!allowed || !isActive)) {
		return true, finishNotification(ctx, connection, job.ID, "cancelled")
	}
	if err != nil {
		return true, retryNotification(ctx, connection, job)
	}

	message, err := renderFriendNotification(job.NotificationType, job.Payload)
	if err == nil {
		err = sendTelegramMessage(ctx, client, botToken, telegramID, message)
	}
	if err != nil {
		return true, retryNotification(ctx, connection, job)
	}
	return true, finishNotification(ctx, connection, job.ID, "sent")
}

func renderFriendNotification(notificationType string, payload []byte) (string, error) {
	var data struct {
		ActorName string `json:"actor_name"`
		Title     string `json:"title"`
	}
	if err := json.Unmarshal(payload, &data); err != nil || data.ActorName == "" {
		return "", errors.New("friend notification payload is invalid")
	}
	switch notificationType {
	case "friend_request_received":
		return data.ActorName + " отправил(а) вам запрос в друзья.", nil
	case "friend_request_accepted":
		return data.ActorName + " принял(а) ваш запрос в друзья.", nil
	case "friend_request_rejected":
		return data.ActorName + " отклонил(а) ваш запрос в друзья.", nil
	case "walk_invitation":
		return data.ActorName + " приглашает вас на прогулку «" + data.Title + "». Откройте WalkTogether, чтобы ответить.", nil
	case "walk_accepted":
		return data.ActorName + " принял(а) приглашение на прогулку «" + data.Title + "».", nil
	case "walk_rejected":
		return data.ActorName + " отклонил(а) приглашение на прогулку «" + data.Title + "».", nil
	case "walk_confirmed":
		return "Прогулка «" + data.Title + "» подтверждена.", nil
	case "walk_rescheduled":
		return data.ActorName + " перенёс(ла) прогулку «" + data.Title + "». Откройте WalkTogether, чтобы подтвердить время.", nil
	case "walk_cancelled":
		return data.ActorName + " отменил(а) прогулку «" + data.Title + "».", nil
	case "walk_reminder":
		return "Напоминание: скоро начнётся прогулка «" + data.Title + "».", nil
	case "walk_expired":
		return "Предложение прогулки «" + data.Title + "» истекло.", nil
	default:
		return "", errors.New("unsupported friend notification type")
	}
}

func finishNotification(ctx context.Context, connection *sql.DB, jobID string, status string) error {
	_, err := connection.ExecContext(ctx, `
		UPDATE notification_jobs SET status = $2, updated_at = NOW()
		WHERE id = $1 AND status = 'processing'
	`, jobID, status)
	return err
}

func retryNotification(ctx context.Context, connection *sql.DB, job notificationJob) error {
	_, err := connection.ExecContext(ctx, `
		UPDATE notification_jobs
		SET retries = retries + 1,
		    status = CASE WHEN retries + 1 >= 5 THEN 'failed' ELSE 'queued' END,
		    scheduled_for = NOW() + (LEAST(3600, POWER(2, retries + 1)) * INTERVAL '1 second'),
		    updated_at = NOW()
		WHERE id = $1 AND status = 'processing'
	`, job.ID)
	return err
}

func sendTelegramMessage(ctx context.Context, client *http.Client, botToken string, chatID int64, text string) error {
	return sendTelegramMessageToAPI(ctx, client, "https://api.telegram.org", botToken, chatID, text)
}

func sendTelegramMessageToAPI(ctx context.Context, client *http.Client, apiBaseURL string, botToken string, chatID int64, text string) error {
	requestBody, err := json.Marshal(map[string]any{"chat_id": chatID, "text": text})
	if err != nil {
		return errors.New("encode Telegram notification")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(apiBaseURL, "/")+"/bot"+botToken+"/sendMessage", strings.NewReader(string(requestBody)))
	if err != nil {
		return errors.New("create Telegram notification request")
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return errors.New("Telegram notification request failed")
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return errors.New("read Telegram notification response")
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("Telegram notification returned HTTP %d", response.StatusCode)
	}
	var result struct {
		OK bool `json:"ok"`
	}
	if err = json.Unmarshal(responseBody, &result); err != nil || !result.OK {
		return errors.New("Telegram rejected the notification")
	}
	return nil
}
