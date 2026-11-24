package transport

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestYandexStationProvider_SearchNearby(t *testing.T) {
	// Mock Yandex API server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3.0/nearest_stations/" {
			http.NotFound(w, r)
			return
		}

		if r.Header.Get("Authorization") != "test-api-key" || r.URL.Query().Get("apikey") != "" {
			t.Error("API key must be sent in Authorization, not the URL")
		}
		for key, want := range map[string]string{
			"lat": "55.7558123", "lng": "37.6173456", "distance": "0.5",
			"format": "json", "transport_types": "train,suburban,bus", "limit": "50",
		} {
			if got := r.URL.Query().Get(key); got != want {
				t.Errorf("query %s = %q, want %q", key, got, want)
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		response := `{
			"stations": [
				{
					"code": "s1",
					"title": "Station 1",
					"popular_title": "Central Station",
					"transport_type": "train",
					"lat": 55.7558,
					"lng": 37.6173,
					"distance": 0.1
				}
			]
		}`
		w.Write([]byte(response))
	}))
	defer server.Close()

	// Create provider with test API key
	provider := NewYandexStationProvider("test-api-key")
	provider.client.Transport = &testTransport{
		baseURL: server.URL,
	}

	// Test search nearby
	ctx := context.Background()
	result, metadata, err := provider.SearchNearby(ctx, 55.7558123, 37.6173456, 500)

	if err != nil {
		t.Fatalf("SearchNearby failed: %v", err)
	}

	if metadata == nil || metadata.Source != "yandex" {
		t.Errorf("Expected yandex source, got %v", metadata)
	}

	if result == nil || len(result.Stations) != 1 {
		t.Fatal("Expected one station in result")
	}

	if len(result.Stations) > 0 && result.Stations[0].Name != "Station 1" {
		t.Errorf("Expected station name 'Station 1', got %s", result.Stations[0].Name)
	}
	if result.Stations[0].Latitude != 55.7558 || result.Stations[0].Longitude != 37.6173 {
		t.Error("Expected coordinates from Yandex lat/lng fields")
	}
}

func TestYandexStationProvider_GetStationByID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3.0/schedule/" || r.URL.Query().Get("station") != "s1" {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		response := `{
			"station": {
				"code": "s1",
				"title": "Station 1",
				"popular_title": "Central Station",
				"transport_type": "train"
			},
			"schedule": []
		}`
		w.Write([]byte(response))
	}))
	defer server.Close()

	provider := NewYandexStationProvider("test-api-key")
	provider.client.Transport = &testTransport{
		baseURL: server.URL,
	}

	ctx := context.Background()
	station, metadata, err := provider.GetStationByID(ctx, "s1")

	if err != nil {
		t.Fatalf("GetStationByID failed: %v", err)
	}

	if station == nil || station.ExternalID != "s1" {
		t.Fatalf("Expected station with ID s1, got %v", station)
	}

	if station.Name != "Station 1" {
		t.Errorf("Expected name 'Station 1', got %s", station.Name)
	}

	if metadata == nil || metadata.Source != "yandex" {
		t.Errorf("Expected yandex source, got %v", metadata)
	}
}

func TestYandexTimetableProvider_SearchRoutes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3.0/search/" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("date") != "2024-11-24" || r.URL.Query().Get("result_timezone") != "Europe/Moscow" {
			t.Error("Expected the requested local date and timezone")
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		response := `{
			"segments": [
				{
					"from": {
						"code": "s1",
						"title": "Station 1"
					},
					"to": {
						"code": "s2",
						"title": "Station 2"
					},
					"departure": "2024-11-24T10:00:00+03:00",
					"arrival": "2024-11-24T12:00:00+03:00",
					"thread": {
						"uid": "route1",
						"number": "120",
						"transport_type": "suburban"
					}
				},
				{
					"from": {
						"code": "s1",
						"title": "Station 1"
					},
					"to": {
						"code": "s2",
						"title": "Station 2"
					},
					"departure": "2024-11-24T14:00:00+03:00",
					"arrival": "2024-11-24T16:00:00+03:00",
					"thread": {
						"uid": "route2",
						"number": "121",
						"transport_type": "train"
					}
				}
			]
		}`
		w.Write([]byte(response))
	}))
	defer server.Close()

	provider := NewYandexTimetableProvider("test-api-key")
	provider.client.Transport = &testTransport{
		baseURL: server.URL,
	}

	from := &Station{
		ExternalID: "s1",
		Name:       "Station 1",
		Provider:   "yandex",
	}
	to := &Station{
		ExternalID: "s2",
		Name:       "Station 2",
		Provider:   "yandex",
	}

	searchDate := time.Date(2024, 11, 24, 0, 0, 0, 0, time.UTC)
	earliestDeparture := time.Date(2024, 11, 24, 7, 0, 0, 0, time.UTC)

	ctx := context.Background()
	result, metadata, err := provider.SearchRoutes(ctx, from, to, searchDate, earliestDeparture, "Europe/Moscow")

	if err != nil {
		t.Fatalf("SearchRoutes failed: %v", err)
	}

	if result == nil || len(result.Routes) != 2 {
		t.Fatal("Expected 2 routes")
	}

	if result.Routes[0].IsAvailable != true {
		t.Errorf("Expected first route to be available")
	}

	if !result.Routes[1].IsAvailable {
		t.Error("A scheduled route must not be marked unavailable just because seat counts are absent")
	}
	if result.Routes[0].ExternalID != "route1" || result.Routes[0].RouteNumber != "120" || result.Routes[0].TransportType != TransportTypeTrain {
		t.Error("Expected route data from the nested thread object")
	}
	if !result.Routes[0].DepartureTime.Equal(earliestDeparture) || result.Routes[0].DepartureTime.Location() != time.UTC {
		t.Error("Expected timezone-aware departure converted to UTC")
	}

	if metadata == nil || metadata.Source != "yandex" {
		t.Fatalf("Expected yandex source, got %v", metadata)
	}

	if metadata.IsReal != true {
		t.Errorf("Expected IsReal to be true")
	}
}

func TestYandexTimetableProvider_SupportsTransport(t *testing.T) {
	provider := NewYandexTimetableProvider("test-api-key")

	tests := []struct {
		transportType TransportType
		region        string
		expected      bool
	}{
		{TransportTypeTrain, "Russia", true},
		{TransportTypeBus, "Russia", true},
		{TransportTypeSubway, "Russia", false},
		{TransportTypeTramway, "Russia", false},
		{TransportTypeInterurban, "Russia", true},
	}

	for _, test := range tests {
		result := provider.SupportsTransport(test.transportType, test.region)
		if result != test.expected {
			t.Errorf("SupportsTransport(%s, %s) = %v, expected %v",
				test.transportType, test.region, result, test.expected)
		}
	}
}

func TestYandexProvider_Names(t *testing.T) {
	stationProvider := NewYandexStationProvider("test-api-key")
	timetableProvider := NewYandexTimetableProvider("test-api-key")

	if stationProvider.Name() != "yandex_stations" {
		t.Errorf("Expected 'yandex_stations', got %s", stationProvider.Name())
	}

	if timetableProvider.Name() != "yandex_timetable" {
		t.Errorf("Expected 'yandex_timetable', got %s", timetableProvider.Name())
	}
}

// testTransport is a custom transport that redirects requests to our test server
type testTransport struct {
	baseURL string
}

func (t *testTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	base, err := url.Parse(t.baseURL)
	if err != nil {
		return nil, err
	}
	newReq := req.Clone(req.Context())
	newReq.URL.Scheme = base.Scheme
	newReq.URL.Host = base.Host
	return http.DefaultTransport.RoundTrip(newReq)
}

func TestYandexStationProvider_ErrorsDoNotExposeAPIKey(t *testing.T) {
	for _, test := range []struct {
		name   string
		status int
		body   string
	}{
		{"unauthorized", http.StatusUnauthorized, `{"error":{"request":"?apikey=test-api-key"}}`},
		{"not found", http.StatusNotFound, `{"error":{"request":"?apikey=test-api-key"}}`},
		{"upstream unavailable", http.StatusServiceUnavailable, "test-api-key"},
		{"invalid JSON", http.StatusOK, "{"},
		{"missing station", http.StatusOK, `{"station":{}}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(test.status)
				_, _ = w.Write([]byte(test.body))
			}))
			defer server.Close()
			provider := NewYandexStationProvider("test-api-key")
			provider.client.Transport = &testTransport{baseURL: server.URL}
			_, _, err := provider.GetStationByID(context.Background(), "s1")
			if err == nil {
				t.Fatal("Expected a provider error")
			}
			if strings.Contains(err.Error(), "test-api-key") {
				t.Fatal("Provider errors must not expose API keys")
			}
			if errors.Is(err, ErrStationNotFound) != (test.status == http.StatusNotFound) {
				t.Errorf("Unexpected station-not-found mapping: %v", err)
			}
		})
	}
}

func TestYandexStationProvider_PreservesCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("A cancelled request must not reach the server")
	}))
	defer server.Close()
	provider := NewYandexStationProvider("test-api-key")
	provider.client.Transport = &testTransport{baseURL: server.URL}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := provider.SearchNearby(ctx, 55.7558, 37.6173, 500)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Expected context cancellation, got %v", err)
	}
}

func TestYandexTimetableProvider_LocalDateAndEarliestDeparture(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("date") != "2026-11-25" {
			t.Error("Converting midnight to UTC must not shift the requested local date")
		}
		_, _ = w.Write([]byte(`{"segments":[
			{"departure":"2026-11-25T09:59:59+03:00","arrival":"2026-11-25T12:00:00+03:00","thread":{"uid":"early","number":"1","transport_type":"train"}},
			{"departure":"2026-11-25T10:00:00+03:00","arrival":"2026-11-25T12:00:00+03:00","thread":{"uid":"on-time","number":"2","transport_type":"bus"}},
			{"departure":"invalid","arrival":"2026-11-25T12:00:00+03:00","thread":{"uid":"invalid","number":"3","transport_type":"train"}}
		]}`))
	}))
	defer server.Close()
	provider := NewYandexTimetableProvider("test-api-key")
	provider.client.Transport = &testTransport{baseURL: server.URL}
	date := time.Date(2026, 11, 24, 21, 0, 0, 0, time.UTC)
	earliest := time.Date(2026, 11, 25, 7, 0, 0, 0, time.UTC)
	result, _, err := provider.SearchRoutes(context.Background(), &Station{ExternalID: "s1"}, &Station{ExternalID: "s2"}, date, earliest, "Europe/Moscow")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Routes) != 1 || result.Routes[0].ExternalID != "on-time" {
		t.Fatal("Expected the route exactly at earliest departure, excluding earlier/invalid times")
	}
}

func TestYandexTimetableProvider_LegacySegments(t *testing.T) {
	segment := YandexSegment{StartTime: "2026-11-25T10:00:00", EndTime: "2026-11-25T12:00:00", ThreadNumber: "legacy", Vehicle: "120", Available: 5}
	segment.Transport.Type = "train"
	routes := convertYandexSegmentsToRoutes([]YandexSegment{segment}, &Station{ExternalID: "s1"}, &Station{ExternalID: "s2"}, time.Time{}, time.UTC)
	if len(routes) != 1 || routes[0].ExternalID != "legacy" || !routes[0].IsAvailable {
		t.Fatal("Legacy segment fields must remain supported")
	}
}
