package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"friendwalk/internal/auth"
)

func TestFriendRoutesRequireSession(t *testing.T) {
	mux := http.NewServeMux()
	registerFriendRoutes(mux, auth.NewService(nil, "", 0, 0, "session", 0), "session")
	tests := []struct {
		method string
		path   string
	}{
		{method: http.MethodGet, path: "/api/users/search?q=al"},
		{method: http.MethodGet, path: "/api/users/123e4567-e89b-12d3-a456-426614174000"},
		{method: http.MethodGet, path: "/api/friends"},
		{method: http.MethodPost, path: "/api/friends"},
		{method: http.MethodGet, path: "/api/friends/requests/incoming"},
		{method: http.MethodPost, path: "/api/friends/requests/123e4567-e89b-12d3-a456-426614174000/accept"},
		{method: http.MethodDelete, path: "/api/friends/123e4567-e89b-12d3-a456-426614174000"},
		{method: http.MethodGet, path: "/api/me/privacy"},
		{method: http.MethodPut, path: "/api/me/privacy"},
	}
	for _, test := range tests {
		t.Run(test.method+" "+test.path, func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.path, nil)
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, request)
			if response.Code != http.StatusUnauthorized {
				t.Fatalf("unauthenticated request returned %d, want 401", response.Code)
			}
			var payload map[string]any
			if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
				t.Fatalf("decode error response: %v", err)
			}
			if payload["status"] != "error" || payload["code"] != "unauthorized" {
				t.Fatalf("unexpected error envelope: %#v", payload)
			}
		})
	}
}

func TestFriendRouteMethodErrorsUseJSONEnvelope(t *testing.T) {
	mux := http.NewServeMux()
	registerFriendRoutes(mux, nil, "session")
	request := httptest.NewRequest(http.MethodPatch, "/api/users/search", nil)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("unsupported method returned %d, want 405", response.Code)
	}
	var payload map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode method error: %v", err)
	}
	if payload["code"] != "method_not_allowed" {
		t.Fatalf("unexpected method error envelope: %#v", payload)
	}
}
