package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"friendwalk/internal/auth"
	"friendwalk/internal/config"
	"friendwalk/internal/db"
	mapprovider "friendwalk/internal/maps"
	"friendwalk/internal/places"
	"friendwalk/internal/schedule"
	"friendwalk/internal/walks"
)

func main() {
	cfg := config.Load()

	if strings.TrimSpace(cfg.PostgresDSN) == "" {
		log.Fatal("POSTGRES_DSN must be configured")
	}
	migrationsDir := db.ResolveMigrationsDir()
	if migrationsDir == "" {
		log.Fatal("database migrations directory not found; run the API from the project or backend directory")
	}
	if err := db.ApplyMigrations(cfg.PostgresDSN, migrationsDir); err != nil {
		log.Fatalf("apply database migrations: %v", err)
	}
	connection, err := db.Connect(cfg.PostgresDSN)
	if err != nil {
		log.Fatalf("connect postgres: %v", err)
	}
	defer connection.Close()
	log.Printf("database migrations applied from %s", migrationsDir)

	authService := auth.NewService(connection, cfg.TelegramBotToken, cfg.InitDataMaxAge(), cfg.SessionTTL(), cfg.SessionCookieName, cfg.InvitationTTL(), cfg.TelegramBotUsername)
	if cfg.MainAdminTelegramID > 0 {
		if err := authService.EnsurePrimaryAdmin(cfg.MainAdminTelegramID); err != nil {
			log.Printf("warning: ensure primary admin: %v", err)
		}
	}

	mux := http.NewServeMux()
	registerFriendRoutes(mux, authService, cfg.SessionCookieName)
	registerScheduleRoutes(mux, schedule.NewService(connection), authService, cfg.SessionCookieName)
	registerWalkRoutes(mux, walks.NewService(connection), authService, cfg.SessionCookieName)
	registerAccountRoutes(mux, authService, cfg.SessionCookieName)
	registerPlaceRoutes(mux, places.NewService(connection), mapprovider.NewNominatimProvider(os.Getenv("MAP_GEOCODING_URL"), os.Getenv("MAP_GEOCODING_USER_AGENT")), authService, cfg.SessionCookieName)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		resp := map[string]any{
			"status":    "ok",
			"app_env":   cfg.AppEnv,
			"timestamp": time.Now().UTC().Format(time.RFC3339),
			"version":   "0.1.0",
			"service":   "friendwalk-api",
		}
		writeJSON(w, http.StatusOK, resp)
	})

	mux.HandleFunc("/api/v1/health", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "friendwalk-api"})
	})

	mux.HandleFunc("/api/auth/telegram", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var payload struct {
			InitData    string `json:"initData"`
			InviteToken string `json:"inviteToken"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"status": "error", "code": "invalid_request", "message": "invalid json payload"})
			return
		}
		if strings.TrimSpace(payload.InitData) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"status": "error", "code": "missing_init_data", "message": "initData is required"})
			return
		}

		result, err := authService.RegisterUserWithInvitation(payload.InitData, payload.InviteToken)
		if err != nil {
			var authErr *auth.AuthError
			if errors.As(err, &authErr) {
				switch authErr.Code {
				case "invalid_init_data":
					writeJSON(w, http.StatusUnauthorized, map[string]any{"status": "error", "code": authErr.Code, "message": authErr.Message})
					return
				case "invitation_expired":
					writeJSON(w, http.StatusForbidden, map[string]any{"status": "error", "code": authErr.Code, "message": authErr.Message})
					return
				case "invitation_used":
					writeJSON(w, http.StatusForbidden, map[string]any{"status": "error", "code": authErr.Code, "message": authErr.Message})
					return
				case "invitation_revoked":
					writeJSON(w, http.StatusForbidden, map[string]any{"status": "error", "code": authErr.Code, "message": authErr.Message})
					return
				case "invitation_required":
					writeJSON(w, http.StatusForbidden, map[string]any{"status": "error", "code": authErr.Code, "message": authErr.Message})
					return
				case "account_blocked":
					writeJSON(w, http.StatusForbidden, map[string]any{"status": "error", "code": authErr.Code, "message": authErr.Message})
					return
				default:
					writeJSON(w, http.StatusUnauthorized, map[string]any{"status": "error", "code": authErr.Code, "message": authErr.Message})
					return
				}
			}
			writeJSON(w, http.StatusInternalServerError, map[string]any{"status": "error", "code": "server_error", "message": "authentication failed"})
			return
		}

		setSessionCookie(w, cfg.SessionCookieName, result.SessionToken, cfg.SessionTTL(), cfg.AppEnv == "production")
		writeJSON(w, http.StatusOK, map[string]any{"status": "success", "user": result.User})
	})

	mux.HandleFunc("/api/invitations", func(w http.ResponseWriter, r *http.Request) {
		token, err := readSessionCookie(r, cfg.SessionCookieName)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"status": "error", "code": "unauthorized", "message": "authentication required"})
			return
		}

		user, err := authService.CurrentUserFromToken(token)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"status": "error", "code": "unauthorized", "message": err.Error()})
			return
		}

		switch r.Method {
		case http.MethodGet:
			items, err := authService.ListInvitationsForUser(user.ID)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]any{"status": "error", "code": "server_error", "message": err.Error()})
				return
			}
			payload := map[string]any{
				"status":      "success",
				"slots_used":  countInviteSlots(items),
				"slots_total": 5,
				"items":       items,
			}
			writeJSON(w, http.StatusOK, payload)
		case http.MethodPost:
			invitation, err := authService.CreateInvitationForUser(user.ID)
			if err != nil {
				var authErr *auth.AuthError
				if errors.As(err, &authErr) {
					writeJSON(w, http.StatusForbidden, map[string]any{"status": "error", "code": authErr.Code, "message": authErr.Message})
					return
				}
				writeJSON(w, http.StatusInternalServerError, map[string]any{"status": "error", "code": "server_error", "message": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"status": "success", "invitation": invitation})
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/invitations/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		token, err := readSessionCookie(r, cfg.SessionCookieName)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"status": "error", "code": "unauthorized", "message": "authentication required"})
			return
		}

		user, err := authService.CurrentUserFromToken(token)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"status": "error", "code": "unauthorized", "message": err.Error()})
			return
		}

		prefix := strings.TrimPrefix(r.URL.Path, "/api/invitations/")
		if strings.TrimSpace(prefix) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"status": "error", "code": "invalid_invitation_id", "message": "invitation id is required"})
			return
		}
		invitationID := strings.TrimPrefix(prefix, "/")
		if strings.Contains(invitationID, "/") {
			invitationID = strings.SplitN(invitationID, "/", 2)[0]
		}
		if err = authService.RevokeInvitation(user.ID, invitationID); err != nil {
			var authErr *auth.AuthError
			if errors.As(err, &authErr) {
				writeJSON(w, http.StatusForbidden, map[string]any{"status": "error", "code": authErr.Code, "message": authErr.Message})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]any{"status": "error", "code": "server_error", "message": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "success", "message": "invitation revoked"})
	})

	mux.HandleFunc("/api/admin/users", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		token, err := readSessionCookie(r, cfg.SessionCookieName)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"status": "error", "code": "unauthorized", "message": "authentication required"})
			return
		}

		user, err := authService.CurrentUserFromToken(token)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"status": "error", "code": "unauthorized", "message": err.Error()})
			return
		}

		users, err := authService.ListUsersForAdmin(user.ID)
		if err != nil {
			var authErr *auth.AuthError
			if errors.As(err, &authErr) {
				writeJSON(w, http.StatusForbidden, map[string]any{"status": "error", "code": authErr.Code, "message": authErr.Message})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]any{"status": "error", "code": "server_error", "message": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "success", "users": users})
	})

	mux.HandleFunc("/api/admin/users/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		token, err := readSessionCookie(r, cfg.SessionCookieName)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"status": "error", "code": "unauthorized", "message": "authentication required"})
			return
		}

		actor, err := authService.CurrentUserFromToken(token)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"status": "error", "code": "unauthorized", "message": err.Error()})
			return
		}

		prefix := strings.TrimPrefix(r.URL.Path, "/api/admin/users/")
		if strings.TrimSpace(prefix) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"status": "error", "code": "invalid_user_id", "message": "user id is required"})
			return
		}
		segments := strings.Split(prefix, "/")
		if len(segments) == 0 || strings.TrimSpace(segments[0]) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"status": "error", "code": "invalid_user_id", "message": "user id is required"})
			return
		}
		targetUserID := segments[0]
		var action string
		if len(segments) >= 2 {
			action = segments[1]
		}

		switch action {
		case "revoke":
			if err = authService.RevokeUserAccess(actor.ID, targetUserID); err != nil {
				var authErr *auth.AuthError
				if errors.As(err, &authErr) {
					writeJSON(w, http.StatusForbidden, map[string]any{"status": "error", "code": authErr.Code, "message": authErr.Message})
					return
				}
				writeJSON(w, http.StatusInternalServerError, map[string]any{"status": "error", "code": "server_error", "message": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"status": "success", "message": "user access revoked"})
		case "restore":
			if err = authService.RestoreUserAccess(actor.ID, targetUserID); err != nil {
				var authErr *auth.AuthError
				if errors.As(err, &authErr) {
					writeJSON(w, http.StatusForbidden, map[string]any{"status": "error", "code": authErr.Code, "message": authErr.Message})
					return
				}
				writeJSON(w, http.StatusInternalServerError, map[string]any{"status": "error", "code": "server_error", "message": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"status": "success", "message": "user access restored"})
		default:
			writeJSON(w, http.StatusBadRequest, map[string]any{"status": "error", "code": "invalid_action", "message": "supported actions: revoke or restore"})
		}
	})

	mux.HandleFunc("/api/me", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		token, err := readSessionCookie(r, cfg.SessionCookieName)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"status": "error", "code": "unauthorized", "message": "authentication required"})
			return
		}

		user, err := authService.CurrentUserFromToken(token)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"status": "error", "code": "unauthorized", "message": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"user": user})
	})

	mux.HandleFunc("/api/auth/logout", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		token, err := readSessionCookie(r, cfg.SessionCookieName)
		if err == nil {
			_ = authService.RevokeToken(token)
		}
		clearSessionCookie(w, cfg.SessionCookieName)
		writeJSON(w, http.StatusOK, map[string]any{"status": "success", "message": "logged out"})
	})

	mux.HandleFunc("/api/auth/logout-all", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		token, err := readSessionCookie(r, cfg.SessionCookieName)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"status": "error", "code": "unauthorized", "message": "authentication required"})
			return
		}
		if err = authService.RevokeAllUserSessions(token); err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"status": "error", "code": "unauthorized", "message": err.Error()})
			return
		}
		clearSessionCookie(w, cfg.SessionCookieName)
		writeJSON(w, http.StatusOK, map[string]any{"status": "success", "message": "all sessions revoked"})
	})

	server := &http.Server{
		Addr:              cfg.Addr(),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       30 * time.Second,
	}

	log.Printf("starting FriendWalk API on %s", server.Addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("api server failed: %v", err)
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func readSessionCookie(r *http.Request, name string) (string, error) {
	cookie, err := r.Cookie(name)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(cookie.Value) == "" {
		return "", fmt.Errorf("cookie is empty")
	}
	return cookie.Value, nil
}

func setSessionCookie(w http.ResponseWriter, name string, token string, ttl time.Duration, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
		MaxAge:   int(ttl.Seconds()),
		Expires:  time.Now().UTC().Add(ttl),
	})
}

func clearSessionCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
		Expires:  time.Unix(0, 0).UTC(),
	})
}

func countInviteSlots(items []auth.Invitation) int {
	count := 0
	for _, item := range items {
		if item.Status == "pending" || item.Status == "accepted" {
			count++
		}
	}
	return count
}
