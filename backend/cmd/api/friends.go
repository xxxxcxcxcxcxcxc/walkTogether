package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"friendwalk/internal/auth"
)

func registerFriendRoutes(mux *http.ServeMux, service *auth.Service, cookieName string) {
	mux.HandleFunc("/api/users/search", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"status": "error", "code": "method_not_allowed", "message": "method not allowed"})
			return
		}
		user, ok := requireAPIUser(w, r, service, cookieName)
		if !ok {
			return
		}
		users, err := service.SearchFriendsUsers(user.ID, r.URL.Query().Get("q"))
		if err != nil {
			writeFriendAPIError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "success", "users": users})
	})

	mux.HandleFunc("/api/users/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"status": "error", "code": "method_not_allowed", "message": "method not allowed"})
			return
		}
		user, ok := requireAPIUser(w, r, service, cookieName)
		if !ok {
			return
		}
		publicID := strings.TrimPrefix(r.URL.Path, "/api/users/")
		if publicID == "" || strings.Contains(publicID, "/") {
			writeJSON(w, http.StatusNotFound, map[string]any{"status": "error", "code": "user_not_found", "message": "user is not available"})
			return
		}
		profile, err := service.GetFriendProfile(user.ID, publicID)
		if err != nil {
			writeFriendAPIError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "success", "user": profile})
	})

	mux.HandleFunc("/api/friends", func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAPIUser(w, r, service, cookieName)
		if !ok {
			return
		}
		switch r.Method {
		case http.MethodGet:
			friends, err := service.ListFriends(user.ID)
			if err != nil {
				writeFriendAPIError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"status": "success", "friends": friends})
		case http.MethodPost:
			var payload struct {
				TargetPublicID string `json:"target_public_id"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"status": "error", "code": "invalid_request", "message": "invalid json payload"})
				return
			}
			request, err := service.CreateFriendRequest(user.ID, payload.TargetPublicID)
			if err != nil {
				writeFriendAPIError(w, err)
				return
			}
			writeJSON(w, http.StatusCreated, map[string]any{"status": "success", "request": request})
		default:
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"status": "error", "code": "method_not_allowed", "message": "method not allowed"})
		}
	})

	mux.HandleFunc("/api/friends/requests/incoming", func(w http.ResponseWriter, r *http.Request) {
		listFriendRequests(w, r, service, cookieName, "incoming")
	})
	mux.HandleFunc("/api/friends/requests/outgoing", func(w http.ResponseWriter, r *http.Request) {
		listFriendRequests(w, r, service, cookieName, "outgoing")
	})
	mux.HandleFunc("/api/friends/requests/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"status": "error", "code": "method_not_allowed", "message": "method not allowed"})
			return
		}
		user, ok := requireAPIUser(w, r, service, cookieName)
		if !ok {
			return
		}
		parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/friends/requests/"), "/"), "/")
		if len(parts) != 2 || parts[0] == "" {
			writeJSON(w, http.StatusNotFound, map[string]any{"status": "error", "code": "friend_request_not_found", "message": "friend request is not available"})
			return
		}
		if err := service.RespondFriendRequest(user.ID, parts[0], parts[1]); err != nil {
			writeFriendAPIError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "success"})
	})

	mux.HandleFunc("/api/friends/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"status": "error", "code": "method_not_allowed", "message": "method not allowed"})
			return
		}
		user, ok := requireAPIUser(w, r, service, cookieName)
		if !ok {
			return
		}
		publicID := strings.TrimPrefix(r.URL.Path, "/api/friends/")
		if publicID == "" || strings.Contains(publicID, "/") {
			writeJSON(w, http.StatusNotFound, map[string]any{"status": "error", "code": "user_not_found", "message": "user is not available"})
			return
		}
		if err := service.RemoveFriend(user.ID, publicID); err != nil {
			writeFriendAPIError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "success"})
	})

	mux.HandleFunc("/api/me/privacy", func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAPIUser(w, r, service, cookieName)
		if !ok {
			return
		}
		switch r.Method {
		case http.MethodGet:
			settings, err := service.GetFriendPrivacySettings(user.ID)
			if err != nil {
				writeFriendAPIError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"status": "success", "settings": settings})
		case http.MethodPut:
			var settings auth.FriendPrivacySettings
			if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"status": "error", "code": "invalid_request", "message": "invalid json payload"})
				return
			}
			if err := service.UpdateFriendPrivacySettings(user.ID, settings); err != nil {
				writeFriendAPIError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"status": "success", "settings": settings})
		default:
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"status": "error", "code": "method_not_allowed", "message": "method not allowed"})
		}
	})
}

func listFriendRequests(w http.ResponseWriter, r *http.Request, service *auth.Service, cookieName string, direction string) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"status": "error", "code": "method_not_allowed", "message": "method not allowed"})
		return
	}
	user, ok := requireAPIUser(w, r, service, cookieName)
	if !ok {
		return
	}
	requests, err := service.ListFriendRequests(user.ID, direction)
	if err != nil {
		writeFriendAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "success", "requests": requests})
}

func requireAPIUser(w http.ResponseWriter, r *http.Request, service *auth.Service, cookieName string) (*auth.User, bool) {
	token, err := readSessionCookie(r, cookieName)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"status": "error", "code": "unauthorized", "message": "authentication required"})
		return nil, false
	}
	user, err := service.CurrentUserFromToken(token)
	if err != nil {
		writeFriendAPIError(w, err)
		return nil, false
	}
	return user, true
}

func writeFriendAPIError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	code := "server_error"
	message := "request failed"
	var authError *auth.AuthError
	if errors.As(err, &authError) {
		code = authError.Code
		message = authError.Message
		switch code {
		case "unauthorized":
			status = http.StatusUnauthorized
		case "user_not_found", "friend_request_not_found":
			status = http.StatusNotFound
		case "already_friends", "friend_request_pending", "incoming_request_exists", "friend_request_not_pending":
			status = http.StatusConflict
		case "invalid_search_query", "invalid_request_direction", "invalid_privacy_mode", "invalid_friend_action":
			status = http.StatusBadRequest
		case "account_blocked", "forbidden":
			status = http.StatusForbidden
		case "service_unavailable":
			status = http.StatusServiceUnavailable
		}
	} else if errors.Is(err, auth.ErrUnauthorized) {
		status = http.StatusUnauthorized
		code = "unauthorized"
		message = "authentication required"
	}
	writeJSON(w, status, map[string]any{"status": "error", "code": code, "message": message})
}
