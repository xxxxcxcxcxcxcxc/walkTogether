package maps

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Place struct {
	Name      string  `json:"name"`
	Address   string  `json:"address"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type Provider interface {
	Search(context.Context, string) ([]Place, error)
}

// NominatimProvider isolates the geocoding backend from the walk domain.
// The public instance is rate limited to one request per second.
type NominatimProvider struct {
	endpoint    string
	client      *http.Client
	userAgent   string
	mu          sync.Mutex
	lastRequest time.Time
}

func NewNominatimProvider(endpoint, userAgent string) *NominatimProvider {
	if strings.TrimSpace(endpoint) == "" {
		endpoint = "https://nominatim.openstreetmap.org/search"
	}
	if strings.TrimSpace(userAgent) == "" {
		userAgent = "WalkTogether/0.1"
	}
	return &NominatimProvider{endpoint: endpoint, userAgent: userAgent, client: &http.Client{Timeout: 8 * time.Second}}
}

func (p *NominatimProvider) Search(ctx context.Context, query string) ([]Place, error) {
	query = strings.TrimSpace(query)
	if len([]rune(query)) < 2 || len([]rune(query)) > 120 {
		return nil, errors.New("search query must contain between 2 and 120 characters")
	}
	p.mu.Lock()
	delay := time.Until(p.lastRequest.Add(time.Second))
	if delay > 0 {
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			p.mu.Unlock()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	p.lastRequest = time.Now()
	p.mu.Unlock()
	parsed, err := url.Parse(p.endpoint)
	if err != nil {
		return nil, fmt.Errorf("invalid geocoding endpoint: %w", err)
	}
	params := parsed.Query()
	params.Set("format", "jsonv2")
	params.Set("limit", "5")
	params.Set("q", query)
	parsed.RawQuery = params.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", p.userAgent)
	request.Header.Set("Accept-Language", "ru")
	response, err := p.client.Do(request)
	if err != nil {
		return nil, errors.New("address search is temporarily unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("geocoding provider returned HTTP %d", response.StatusCode)
	}
	var raw []struct {
		Name        string `json:"name"`
		DisplayName string `json:"display_name"`
		Lat         string `json:"lat"`
		Lon         string `json:"lon"`
	}
	if err = json.NewDecoder(response.Body).Decode(&raw); err != nil {
		return nil, errors.New("geocoding provider returned invalid data")
	}
	places := make([]Place, 0, len(raw))
	for _, item := range raw {
		lat, e1 := strconv.ParseFloat(item.Lat, 64)
		lon, e2 := strconv.ParseFloat(item.Lon, 64)
		if e1 != nil || e2 != nil || lat < -90 || lat > 90 || lon < -180 || lon > 180 {
			continue
		}
		name := item.Name
		if name == "" {
			name = item.DisplayName
		}
		places = append(places, Place{Name: name, Address: item.DisplayName, Latitude: lat, Longitude: lon})
	}
	return places, nil
}
