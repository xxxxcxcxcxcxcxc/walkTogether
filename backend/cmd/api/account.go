package main

import (
	"encoding/json"
	"net/http"
	"strings"

	"friendwalk/internal/auth"
)

func registerAccountRoutes(mux *http.ServeMux, service *auth.Service, cookieName string) {
	mux.HandleFunc("/api/me/profile", func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAPIUser(w, r, service, cookieName)
		if !ok {
			return
		}
		if r.Method != http.MethodPut {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"status": "error", "code": "method_not_allowed"})
			return
		}
		var input struct {
			DisplayName string `json:"display_name"`
		}
		if json.NewDecoder(r.Body).Decode(&input) != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"status": "error", "code": "invalid_request"})
			return
		}
		if err := service.UpdateDisplayName(user.ID, input.DisplayName); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"status": "error", "code": "invalid_profile", "message": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "success", "display_name": strings.TrimSpace(input.DisplayName)})
	})
	mux.HandleFunc("/api/me/notifications", func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAPIUser(w, r, service, cookieName)
		if !ok {
			return
		}
		switch r.Method {
		case http.MethodGet:
			settings, err := service.GetNotificationSettings(user.ID)
			if err != nil {
				writeFriendAPIError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"status": "success", "settings": settings})
		case http.MethodPut:
			var settings auth.NotificationSettings
			if json.NewDecoder(r.Body).Decode(&settings) != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"status": "error", "code": "invalid_request"})
				return
			}
			if err := service.UpdateNotificationSettings(user.ID, settings); err != nil {
				writeFriendAPIError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"status": "success", "settings": settings})
		default:
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"status": "error", "code": "method_not_allowed"})
		}
	})
}
