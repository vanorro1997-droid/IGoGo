package transport

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// TestYandexIntegration_RealAPI tests the Yandex provider with real API
// Set YANDEX_RASP_API_KEY (or the legacy YANDEX_API_KEY) to run live tests.
func TestYandexIntegration_RealAPI(t *testing.T) {
	apiKey := yandexIntegrationAPIKey(t)

	provider := NewYandexStationProvider(apiKey)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Test with Moscow coordinates
	result, metadata, err := provider.SearchNearby(ctx, 55.7558, 37.6173, 10000)
	if err != nil {
		t.Fatalf("SearchNearby failed: %v", err)
	}

	if metadata == nil {
		t.Fatal("Expected metadata, got nil")
	}

	if !metadata.IsReal {
		t.Error("Expected IsReal to be true for real API")
	}

	if result == nil || len(result.Stations) == 0 {
		t.Log("Warning: No stations found (this might be a valid API response)")
	} else {
		t.Logf("Found %d stations\n", len(result.Stations))
		for i, station := range result.Stations[:min(len(result.Stations), 3)] {
			t.Logf("Station %d: %s (ID: %s)\n", i+1, station.Name, station.ExternalID)
		}
	}
}

// TestYandexIntegration_GetStationByID tests getting a specific station
func TestYandexIntegration_GetStationByID(t *testing.T) {
	apiKey := yandexIntegrationAPIKey(t)

	provider := NewYandexStationProvider(apiKey)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// First get nearby stations
	result, _, err := provider.SearchNearby(ctx, 55.7558, 37.6173, 10000)
	if err != nil {
		t.Fatalf("SearchNearby failed: %v", err)
	}

	if len(result.Stations) == 0 {
		t.Skip("No stations found, skipping GetStationByID test")
	}

	// Try to get the first station by ID
	stationID := result.Stations[0].ExternalID
	t.Logf("Attempting to get station: %s\n", stationID)

	station, metadata, err := provider.GetStationByID(ctx, stationID)
	if err != nil {
		t.Fatalf("GetStationByID failed: %v", err)
	}

	if station == nil {
		t.Error("Expected station, got nil")
	} else {
		t.Logf("Got station: %s\n", station.Name)
	}

	if metadata == nil || !metadata.IsReal {
		t.Error("Expected real metadata")
	}
}

// TestYandexIntegration_SearchRoutes tests route searching
func TestYandexIntegration_SearchRoutes(t *testing.T) {
	apiKey := yandexIntegrationAPIKey(t)

	provider := NewYandexTimetableProvider(apiKey)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Use typical station IDs (these might vary based on API)
	from := &Station{
		ExternalID: "c213", // Moscow
		Name:       "Moscow",
		Provider:   "yandex",
	}
	to := &Station{
		ExternalID: "c54", // Saint Petersburg
		Name:       "Saint Petersburg",
		Provider:   "yandex",
	}

	searchDate := time.Now().Add(24 * time.Hour).Truncate(24 * time.Hour)
	earliestDeparture := searchDate

	t.Logf("Searching routes from %s to %s on %s\n", from.Name, to.Name, searchDate.Format("2006-01-02"))

	result, metadata, err := provider.SearchRoutes(ctx, from, to, searchDate, earliestDeparture, "Europe/Moscow")
	if err != nil {
		t.Fatalf("SearchRoutes failed: %v", err)
	}

	if metadata == nil || !metadata.IsReal {
		t.Error("Expected real metadata")
	}

	if result == nil || len(result.Routes) == 0 {
		t.Fatal("Expected scheduled routes for the Moscow — Saint Petersburg corridor")
	}

	if result != nil {
		t.Logf("Found %d routes\n", len(result.Routes))
		for i, route := range result.Routes[:min(len(result.Routes), 3)] {
			status := "available"
			if !route.IsAvailable {
				status = "unavailable"
			}
			t.Logf("Route %d: %s -> %s (%s) [%s]\n",
				i+1,
				route.DepartureTime.Format("15:04"),
				route.ArrivalTime.Format("15:04"),
				route.TransportType,
				status,
			)
		}
	}
}

// TestYandexIntegration_APIKeyValidation tests API key validation
func TestYandexIntegration_APIKeyValidation(t *testing.T) {
	yandexIntegrationAPIKey(t) // Invalid-key requests also require opting into live tests.
	invalidKey := "invalid-key-12345"
	provider := NewYandexStationProvider(invalidKey)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, _, err := provider.SearchNearby(ctx, 55.7558, 37.6173, 500)
	if err == nil {
		t.Error("Expected error with invalid API key")
	}

	t.Logf("Got expected error: %v\n", err)
}

func yandexIntegrationAPIKey(t *testing.T) string {
	t.Helper()
	apiKey := strings.TrimSpace(os.Getenv("YANDEX_RASP_API_KEY"))
	if apiKey == "" {
		apiKey = strings.TrimSpace(os.Getenv("YANDEX_API_KEY"))
	}
	if apiKey == "" {
		t.Skip("YANDEX_RASP_API_KEY not set, skipping real API test")
	}
	return apiKey
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
