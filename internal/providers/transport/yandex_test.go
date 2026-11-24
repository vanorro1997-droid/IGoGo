package transport

import (
	"context"
	"net/http"
	"net/http/httptest"
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

		// Verify API key is passed
		apiKey := r.URL.Query().Get("apikey")
		if apiKey == "" {
			http.Error(w, "Missing API key", http.StatusUnauthorized)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		response := `{
			"stations": [
				{
					"station": {
						"code": "s1",
						"title": "Station 1",
						"popular_title": "Central Station",
						"transport_type": "train",
						"country": "RU",
						"city": "Moscow",
						"timezone": "Europe/Moscow",
						"lat": 55.7558,
						"lon": 37.6173
					},
					"distance": 100
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
	result, metadata, err := provider.SearchNearby(ctx, 55.7558, 37.6173, 500)

	if err != nil {
		t.Errorf("SearchNearby failed: %v", err)
	}

	if metadata == nil || metadata.Source != "yandex" {
		t.Errorf("Expected yandex source, got %v", metadata)
	}

	if result == nil || len(result.Stations) == 0 {
		t.Errorf("Expected stations in result, got %d", len(result.Stations))
	}

	if len(result.Stations) > 0 && result.Stations[0].Name != "Station 1" {
		t.Errorf("Expected station name 'Station 1', got %s", result.Stations[0].Name)
	}
}

func TestYandexStationProvider_GetStationByID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3.0/station/s1/" {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		response := `{
			"code": "s1",
			"title": "Station 1",
			"popular_title": "Central Station",
			"transport_type": "train",
			"country": "RU",
			"city": "Moscow",
			"timezone": "Europe/Moscow",
			"lat": 55.7558,
			"lon": 37.6173
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
		t.Errorf("GetStationByID failed: %v", err)
	}

	if station == nil || station.ExternalID != "s1" {
		t.Errorf("Expected station with ID s1, got %v", station)
	}

	if station.Name != "Station 1" {
		t.Errorf("Expected name 'Station 1', got %s", station.Name)
	}

	if metadata.Source != "yandex" {
		t.Errorf("Expected yandex source, got %s", metadata.Source)
	}
}

func TestYandexTimetableProvider_SearchRoutes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3.0/search/" {
			http.NotFound(w, r)
			return
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
					"departure": "2024-11-24T10:00:00",
					"arrival": "2024-11-24T12:00:00",
					"thread_uid": "route1",
					"transport": {
						"code": "1",
						"type": "train"
					},
					"vehicle_number": "120",
					"available_seats": 5,
					"is_recommended": true
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
					"departure": "2024-11-24T14:00:00",
					"arrival": "2024-11-24T16:00:00",
					"thread_uid": "route2",
					"transport": {
						"code": "1",
						"type": "train"
					},
					"vehicle_number": "121",
					"available_seats": 0,
					"is_recommended": false
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
	earliestDeparture := time.Date(2024, 11, 24, 8, 0, 0, 0, time.UTC)

	ctx := context.Background()
	result, metadata, err := provider.SearchRoutes(ctx, from, to, searchDate, earliestDeparture, "Europe/Moscow")

	if err != nil {
		t.Errorf("SearchRoutes failed: %v", err)
	}

	if len(result.Routes) != 2 {
		t.Errorf("Expected 2 routes, got %d", len(result.Routes))
	}

	if result.Routes[0].IsAvailable != true {
		t.Errorf("Expected first route to be available")
	}

	if result.Routes[1].IsAvailable != false {
		t.Errorf("Expected second route to be unavailable")
	}

	if metadata.Source != "yandex" {
		t.Errorf("Expected yandex source, got %s", metadata.Source)
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
		{TransportTypeSubway, "Russia", true},
		{TransportTypeTramway, "Russia", true},
		{TransportTypeInterurban, "Russia", false},
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
	req.URL.Scheme = "http"
	req.URL.Host = "localhost"
	// Get the port from the test server URL and set it properly
	newReq, _ := http.NewRequest(req.Method, t.baseURL+req.URL.RequestURI(), req.Body)
	newReq.Header = req.Header
	return http.DefaultTransport.RoundTrip(newReq)
}
