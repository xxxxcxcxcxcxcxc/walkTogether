package maps

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNominatimSearchMapsResultsThroughProvider(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("q") != "Gorky Park" || r.URL.Query().Get("format") != "jsonv2" || r.URL.Query().Get("limit") != "5" {
			t.Errorf("unexpected search parameters: %s", r.URL.RawQuery)
		}
		if r.Header.Get("User-Agent") != "WalkTogetherTest/1.0" {
			t.Errorf("missing provider-identifying user agent: %q", r.Header.Get("User-Agent"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"name":"Gorky Park","display_name":"Moscow, Gorky Park","lat":"55.731","lon":"37.603"}]`))
	}))
	defer server.Close()

	places, err := NewNominatimProvider(server.URL, "WalkTogetherTest/1.0").Search(context.Background(), "Gorky Park")
	if err != nil || len(places) != 1 {
		t.Fatalf("search returned %d places, err=%v", len(places), err)
	}
	if places[0].Name != "Gorky Park" || places[0].Address != "Moscow, Gorky Park" || places[0].Latitude != 55.731 || places[0].Longitude != 37.603 {
		t.Fatalf("provider response was mapped incorrectly: %+v", places[0])
	}
}

func TestNominatimSearchRejectsInvalidQuery(t *testing.T) {
	if _, err := NewNominatimProvider("", "").Search(context.Background(), " "); err == nil {
		t.Fatal("empty query should fail validation")
	}
}
