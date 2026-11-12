package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"friendwalk/internal/auth"
	"friendwalk/internal/walks"
)

func registerWalkRoutes(mux *http.ServeMux, service *walks.Service, authService *auth.Service, cookieName string) {
	mux.HandleFunc("/api/walks/slots", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"status": "error", "code": "method_not_allowed"})
			return
		}
		user, ok := requireAPIUser(w, r, authService, cookieName)
		if !ok {
			return
		}
		var input struct {
			Date         string   `json:"date"`
			Duration     int      `json:"duration_minutes"`
			Participants []string `json:"participants"`
		}
		if json.NewDecoder(r.Body).Decode(&input) != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"status": "error", "code": "invalid_request"})
			return
		}
		slots, err := service.CommonSlots(user.ID, input.Date, input.Participants, input.Duration)
		if err != nil {
			writeWalkError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "success", "slots": slots})
	})
	mux.HandleFunc("/api/walks", func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAPIUser(w, r, authService, cookieName)
		if !ok {
			return
		}
		switch r.Method {
		case http.MethodGet:
			items, err := service.List(user.ID, r.URL.Query().Get("status"))
			if err != nil {
				writeWalkError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"status": "success", "walks": items})
		case http.MethodPost:
			var input walks.CreateInput
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"status": "error", "code": "invalid_request", "message": "invalid json payload"})
				return
			}
			created, err := service.Create(user.ID, input)
			if err != nil {
				writeWalkError(w, err)
				return
			}
			writeJSON(w, http.StatusCreated, map[string]any{"status": "success", "walk": created})
		default:
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"status": "error", "code": "method_not_allowed", "message": "method not allowed"})
		}
	})
	mux.HandleFunc("/api/walks/", func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAPIUser(w, r, authService, cookieName)
		if !ok {
			return
		}
		parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/walks/"), "/"), "/")
		if len(parts) == 0 || parts[0] == "" {
			writeWalkError(w, errors.New("walk is not available"))
			return
		}
		id := parts[0]
		if len(parts) == 1 && r.Method == http.MethodGet {
			item, err := service.Get(user.ID, id)
			if err != nil {
				writeWalkError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"status": "success", "walk": item})
			return
		}
		if len(parts) != 2 {
			writeWalkError(w, errors.New("walk is not available"))
			return
		}
		switch parts[1] {
		case "respond":
			if r.Method != http.MethodPost {
				writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"status": "error", "code": "method_not_allowed"})
				return
			}
			var body struct {
				Action string `json:"action"`
			}
			if json.NewDecoder(r.Body).Decode(&body) != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"status": "error", "code": "invalid_request"})
				return
			}
			if err := service.Respond(user.ID, id, body.Action); err != nil {
				writeWalkError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"status": "success"})
		case "cancel":
			if r.Method != http.MethodPost {
				writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"status": "error", "code": "method_not_allowed"})
				return
			}
			if err := service.Cancel(user.ID, id); err != nil {
				writeWalkError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"status": "success"})
		case "reschedule":
			if r.Method != http.MethodPost {
				writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"status": "error", "code": "method_not_allowed"})
				return
			}
			var body struct {
				Start     time.Time `json:"scheduled_start"`
				End       time.Time `json:"scheduled_end"`
				Location  string    `json:"location_name"`
				Latitude  *float64  `json:"latitude"`
				Longitude *float64  `json:"longitude"`
			}
			if json.NewDecoder(r.Body).Decode(&body) != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"status": "error", "code": "invalid_request"})
				return
			}
			if err := service.Reschedule(user.ID, id, body.Start, body.End, body.Location, body.Latitude, body.Longitude); err != nil {
				writeWalkError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"status": "success"})
		case "complete":
			if r.Method != http.MethodPost {
				writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"status": "error", "code": "method_not_allowed"})
				return
			}
			if err := service.Complete(user.ID, id); err != nil {
				writeWalkError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"status": "success"})
		case "calendar":
			if r.Method != http.MethodGet {
				writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"status": "error", "code": "method_not_allowed"})
				return
			}
			item, err := service.Export(user.ID, id)
			if err != nil {
				writeWalkError(w, err)
				return
			}
			format := r.URL.Query().Get("format")
			if format == "ics" {
				w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
				w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=walk-%s.ics", id))
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(walks.ICS(item))
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"status": "success", "links": walks.CalendarLinks(item)})
		default:
			writeWalkError(w, errors.New("walk is not available"))
		}
	})
}

func writeWalkError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	code := "server_error"
	message := "walk request failed"
	switch {
	case strings.Contains(err.Error(), "private"):
		status = http.StatusConflict
		code = "availability_private"
		message = "точное расписание части друзей скрыто; укажите время вручную"
	case strings.Contains(err.Error(), "not available"):
		status = http.StatusNotFound
		code = "not_found"
		message = "walk is not available"
	case strings.Contains(err.Error(), "only the organizer") || strings.Contains(err.Error(), "account is unavailable"):
		status = http.StatusForbidden
		code = "forbidden"
		message = err.Error()
	case strings.Contains(err.Error(), "invalid") || strings.Contains(err.Error(), "must ") || strings.Contains(err.Error(), "select between") || strings.Contains(err.Error(), "duplicate") || strings.Contains(err.Error(), "cannot") || strings.Contains(err.Error(), "awaiting") || strings.Contains(err.Error(), "organizer cannot"):
		status = http.StatusBadRequest
		code = "invalid_walk_request"
		message = err.Error()
	}
	writeJSON(w, status, map[string]any{"status": "error", "code": code, "message": message})
}
