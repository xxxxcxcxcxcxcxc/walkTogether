package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"friendwalk/internal/auth"
	mapprovider "friendwalk/internal/maps"
	"friendwalk/internal/places"
)

func registerPlaceRoutes(mux *http.ServeMux, service *places.Service, provider mapprovider.Provider, authService *auth.Service, cookieName string) {
	mux.HandleFunc("/api/places/search", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"status": "error", "code": "method_not_allowed"})
			return
		}
		if _, ok := requireAPIUser(w, r, authService, cookieName); !ok {
			return
		}
		items, err := provider.Search(r.Context(), r.URL.Query().Get("q"))
		if err != nil {
			status := http.StatusBadGateway
			if strings.Contains(err.Error(), "query must") {
				status = http.StatusBadRequest
			}
			writeJSON(w, status, map[string]any{"status": "error", "code": "map_search_failed", "message": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "success", "places": items})
	})
	mux.HandleFunc("/api/places", func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAPIUser(w, r, authService, cookieName)
		if !ok {
			return
		}
		switch r.Method {
		case http.MethodGet:
			items, err := service.List(user.ID)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]any{"status": "error", "code": "server_error"})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"status": "success", "places": items})
		case http.MethodPost:
			var item places.Place
			if json.NewDecoder(r.Body).Decode(&item) != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"status": "error", "code": "invalid_request"})
				return
			}
			created, err := service.Create(user.ID, item)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"status": "error", "code": "invalid_place", "message": err.Error()})
				return
			}
			writeJSON(w, http.StatusCreated, map[string]any{"status": "success", "place": created})
		default:
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"status": "error", "code": "method_not_allowed"})
		}
	})
	mux.HandleFunc("/api/places/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"status": "error", "code": "method_not_allowed"})
			return
		}
		user, ok := requireAPIUser(w, r, authService, cookieName)
		if !ok {
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/api/places/")
		if id == "" || strings.Contains(id, "/") {
			writeJSON(w, http.StatusNotFound, map[string]any{"status": "error", "code": "not_found"})
			return
		}
		if err := service.Delete(user.ID, id); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeJSON(w, http.StatusNotFound, map[string]any{"status": "error", "code": "not_found"})
			} else {
				writeJSON(w, http.StatusInternalServerError, map[string]any{"status": "error", "code": "server_error"})
			}
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "success"})
	})
}
