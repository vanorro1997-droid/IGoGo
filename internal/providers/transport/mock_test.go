package transport

import (
	"context"
	"testing"
	"time"
)

func TestMockStationProviderSearchNearby(t *testing.T) {
	provider := NewMockStationProvider()
	ctx := context.Background()

	// Ищем станции вблизи центра
	result, meta, err := provider.SearchNearby(ctx, 55.7558, 37.6173, 10000)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result == nil {
		t.Fatal("expected non-nil result")
	}

	if meta.IsReal {
		t.Error("expected mock data (IsReal=false)")
	}

	if len(result.Stations) == 0 {
		t.Error("expected to find nearby stations")
	}

	// Проверяем, что они действительно в радиусе
	for _, station := range result.Stations {
		dist := haversineDistance(55.7558, 37.6173, station.Latitude, station.Longitude)
		if dist > 10000 {
			t.Errorf("station is outside search radius: distance=%f", dist)
		}
	}
}

func TestMockStationProviderGetStationByID(t *testing.T) {
	provider := NewMockStationProvider()
	ctx := context.Background()

	station, meta, err := provider.GetStationByID(ctx, "mock_station_1")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if station == nil {
		t.Fatal("expected non-nil station")
	}

	if station.Name != "Station Central" {
		t.Errorf("expected 'Station Central', got %s", station.Name)
	}

	if meta.IsReal {
		t.Error("expected IsReal=false for mock data")
	}
}

func TestMockStationProviderNotFound(t *testing.T) {
	provider := NewMockStationProvider()
	ctx := context.Background()

	station, meta, err := provider.GetStationByID(ctx, "nonexistent")

	if err == nil {
		t.Fatal("expected error for nonexistent station")
	}

	if station != nil {
		t.Error("expected nil station on error")
	}

	if meta != nil {
		t.Error("expected nil metadata on error")
	}
}

func TestMockTimetableProviderSearchRoutes(t *testing.T) {
	provider := NewMockTimetableProvider()
	ctx := context.Background()

	stationProvider := NewMockStationProvider()
	from, _, _ := stationProvider.GetStationByID(ctx, "mock_station_1")
	to, _, _ := stationProvider.GetStationByID(ctx, "mock_station_2")

	date := time.Now().UTC()
	earliestDeparture := date.Add(-1 * time.Hour)

	result, _, err := provider.SearchRoutes(ctx, from, to, date, earliestDeparture, "UTC")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result == nil {
		t.Fatal("expected non-nil result")
	}

	if len(result.Routes) == 0 {
		t.Fatal("expected to find routes")
	}

	// Проверяем, что все маршруты доступны
	for _, route := range result.Routes {
		if !route.IsAvailable {
			t.Error("expected all mock routes to be available")
		}
		if route.DepartureTime.Before(date) {
			t.Error("expected departure time to be on the date or after")
		}
	}
}

func TestMockTimetableProviderSupportsTransport(t *testing.T) {
	provider := NewMockTimetableProvider()

	if !provider.SupportsTransport(TransportTypeTrain, "moscow") {
		t.Error("expected train support")
	}

	if !provider.SupportsTransport(TransportTypeBus, "moscow") {
		t.Error("expected bus support")
	}

	// Mock не поддерживает метро в тесте
	if provider.SupportsTransport(TransportTypeSubway, "moscow") {
		t.Error("expected no subway support in mock")
	}
}

func TestMockStationProviderName(t *testing.T) {
	provider := NewMockStationProvider()
	if provider.Name() != "mock_stations" {
		t.Errorf("expected 'mock_stations', got %s", provider.Name())
	}
}

func TestMockTimetableProviderName(t *testing.T) {
	provider := NewMockTimetableProvider()
	if provider.Name() != "mock_timetable" {
		t.Errorf("expected 'mock_timetable', got %s", provider.Name())
	}
}
