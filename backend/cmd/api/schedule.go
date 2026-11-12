package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"friendwalk/internal/auth"
	"friendwalk/internal/schedule"
)

func registerScheduleRoutes(mux *http.ServeMux, service *schedule.Service, authService *auth.Service, cookieName string) {
	mux.HandleFunc("/api/availability", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"status": "error", "code": "method_not_allowed", "message": "method not allowed"})
			return
		}
		user, ok := requireAPIUser(w, r, authService, cookieName)
		if !ok {
			return
		}
		from := r.URL.Query().Get("from")
		to := r.URL.Query().Get("to")
		days, err := service.GetCalendar(user.ID, from, to)
		if err != nil {
			writeScheduleError(w, err)
			return
		}
		timezone, err := service.GetTimezone(user.ID)
		if err != nil {
			writeScheduleError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "success", "timezone": timezone, "days": days})
	})

	mux.HandleFunc("/api/availability/days/", func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAPIUser(w, r, authService, cookieName)
		if !ok {
			return
		}
		parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/availability/days/"), "/"), "/")
		if len(parts) == 1 && parts[0] != "" && r.Method == http.MethodPut {
			var payload struct {
				State string `json:"state"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"status": "error", "code": "invalid_request", "message": "invalid json payload"})
				return
			}
			if err := service.SetDayState(user.ID, parts[0], payload.State); err != nil {
				writeScheduleError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"status": "success"})
			return
		}
		if len(parts) == 2 && parts[0] != "" && parts[1] == "intervals" && r.Method == http.MethodPost {
			var payload struct {
				StartTime string `json:"start_time"`
				EndTime   string `json:"end_time"`
				Kind      string `json:"kind"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"status": "error", "code": "invalid_request", "message": "invalid json payload"})
				return
			}
			interval, err := service.AddDayInterval(user.ID, parts[0], payload.StartTime, payload.EndTime, payload.Kind)
			if err != nil {
				writeScheduleError(w, err)
				return
			}
			writeJSON(w, http.StatusCreated, map[string]any{"status": "success", "interval": interval})
			return
		}
		writeJSON(w, http.StatusNotFound, map[string]any{"status": "error", "code": "not_found", "message": "availability route not found"})
	})

	mux.HandleFunc("/api/availability/intervals/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete && r.Method != http.MethodPut {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"status": "error", "code": "method_not_allowed", "message": "method not allowed"})
			return
		}
		user, ok := requireAPIUser(w, r, authService, cookieName)
		if !ok {
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/api/availability/intervals/")
		if id == "" || strings.Contains(id, "/") {
			writeScheduleError(w, errors.New("availability interval not found"))
			return
		}
		if r.Method == http.MethodPut {
			var payload struct {
				StartTime string `json:"start_time"`
				EndTime   string `json:"end_time"`
				Kind      string `json:"kind"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"status": "error", "code": "invalid_request", "message": "invalid json payload"})
				return
			}
			interval, err := service.UpdateDayInterval(user.ID, id, payload.StartTime, payload.EndTime, payload.Kind)
			if err != nil {
				writeScheduleError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"status": "success", "interval": interval})
			return
		}
		if err := service.DeleteDayInterval(user.ID, id); err != nil {
			writeScheduleError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "success"})
	})

	mux.HandleFunc("/api/availability/rules", func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAPIUser(w, r, authService, cookieName)
		if !ok {
			return
		}
		switch r.Method {
		case http.MethodGet:
			rules, err := service.ListRules(user.ID)
			if err != nil {
				writeScheduleError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"status": "success", "rules": rules})
		case http.MethodPost:
			var rule schedule.Rule
			if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"status": "error", "code": "invalid_request", "message": "invalid json payload"})
				return
			}
			created, err := service.AddRule(user.ID, rule)
			if err != nil {
				writeScheduleError(w, err)
				return
			}
			writeJSON(w, http.StatusCreated, map[string]any{"status": "success", "rule": created})
		default:
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"status": "error", "code": "method_not_allowed", "message": "method not allowed"})
		}
	})

	mux.HandleFunc("/api/availability/rules/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"status": "error", "code": "method_not_allowed", "message": "method not allowed"})
			return
		}
		user, ok := requireAPIUser(w, r, authService, cookieName)
		if !ok {
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/api/availability/rules/")
		if id == "" || strings.Contains(id, "/") {
			writeJSON(w, http.StatusNotFound, map[string]any{"status": "error", "code": "not_found", "message": "availability rule not found"})
			return
		}
		if err := service.DeleteRule(user.ID, id); err != nil {
			writeScheduleError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "success"})
	})

	mux.HandleFunc("/api/availability/friends", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"status": "error", "code": "method_not_allowed", "message": "method not allowed"})
			return
		}
		user, ok := requireAPIUser(w, r, authService, cookieName)
		if !ok {
			return
		}
		publicIDs := strings.Split(r.URL.Query().Get("user_ids"), ",")
		availability, err := service.FriendsAvailability(user.ID, r.URL.Query().Get("date"), publicIDs)
		if err != nil {
			writeScheduleError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "success", "friends": availability})
	})

	mux.HandleFunc("/api/me/availability-privacy", func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAPIUser(w, r, authService, cookieName)
		if !ok {
			return
		}
		switch r.Method {
		case http.MethodGet:
			settings, err := service.GetPrivacySettings(user.ID)
			if err != nil {
				writeScheduleError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"status": "success", "settings": settings})
		case http.MethodPut:
			var settings schedule.PrivacySettings
			if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"status": "error", "code": "invalid_request", "message": "invalid json payload"})
				return
			}
			if err := service.UpdatePrivacySettings(user.ID, settings); err != nil {
				writeScheduleError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"status": "success", "settings": settings})
		default:
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"status": "error", "code": "method_not_allowed", "message": "method not allowed"})
		}
	})
}

func writeScheduleError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	code := "server_error"
	message := "schedule request failed"
	if errors.Is(err, sql.ErrNoRows) || strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "not available") {
		status = http.StatusNotFound
		code = "not_found"
		message = "schedule resource is not available"
	} else if strings.Contains(err.Error(), "must ") || strings.Contains(err.Error(), "invalid") || strings.Contains(err.Error(), "select between") || strings.Contains(err.Error(), "timezone") || strings.Contains(err.Error(), "overlap") {
		status = http.StatusBadRequest
		code = "invalid_schedule_request"
		message = err.Error()
	}
	writeJSON(w, status, map[string]any{"status": "error", "code": code, "message": message})
}
