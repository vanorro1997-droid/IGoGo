package transport

import (
	"context"
	"fmt"
	"math"
	"time"
)

// MockStationProvider — mock-провайдер для тестирования
type MockStationProvider struct {
	stations []*Station
}

// NewMockStationProvider создаёт mock-провайдер со станциями тестовой области
func NewMockStationProvider() *MockStationProvider {
	return &MockStationProvider{
		stations: testStations(),
	}
}

// SearchNearby ищет станции вблизи координат
func (m *MockStationProvider) SearchNearby(ctx context.Context, lat, lon float64, radiusM int) (*StationSearchResult, *ResponseMetadata, error) {
	var nearby []*Station

	for _, station := range m.stations {
		dist := haversineDistance(lat, lon, station.Latitude, station.Longitude)
		if dist <= float64(radiusM) {
			nearby = append(nearby, station)
		}
	}

	return &StationSearchResult{
			Stations: nearby,
		}, &ResponseMetadata{
			Source:    "mock",
			Timestamp: time.Now().UTC(),
			IsReal:    false,
		}, nil
}

// GetStationByID возвращает станцию по ID
func (m *MockStationProvider) GetStationByID(ctx context.Context, externalID string) (*Station, *ResponseMetadata, error) {
	for _, station := range m.stations {
		if station.ExternalID == externalID {
			return station, &ResponseMetadata{
				Source:    "mock",
				Timestamp: time.Now().UTC(),
				IsReal:    false,
			}, nil
		}
	}
	return nil, nil, fmt.Errorf("%w: %s", ErrStationNotFound, externalID)
}

// Name возвращает название провайдера
func (m *MockStationProvider) Name() string {
	return "mock_stations"
}

// MockTimetableProvider — mock-провайдер расписаний
type MockTimetableProvider struct{}

// NewMockTimetableProvider создаёт mock-провайдер расписаний
func NewMockTimetableProvider() *MockTimetableProvider {
	return &MockTimetableProvider{}
}

// SearchRoutes ищет маршруты между станциями
func (m *MockTimetableProvider) SearchRoutes(
	ctx context.Context,
	from *Station,
	to *Station,
	date time.Time,
	earliestDeparture time.Time,
	tz string,
) (*RoutesSearchResult, *ResponseMetadata, error) {
	// Генерируем тестовые маршруты
	routes := generateTestRoutes(from, to, date, earliestDeparture, tz)

	return &RoutesSearchResult{
			Routes: routes,
		}, &ResponseMetadata{
			Source:    "mock",
			Timestamp: time.Now().UTC(),
			IsReal:    false,
		}, nil
}

// SupportsTransport проверяет поддержку транспорта в регионе
func (m *MockTimetableProvider) SupportsTransport(transportType TransportType, region string) bool {
	// Mock всегда поддерживает электрички и автобусы
	return transportType == TransportTypeTrain || transportType == TransportTypeBus
}

// Name возвращает название провайдера
func (m *MockTimetableProvider) Name() string {
	return "mock_timetable"
}

// Вспомогательные функции

func testStations() []*Station {
	return []*Station{
		{
			ExternalID: "mock_station_1",
			Name:       "Station Central",
			Latitude:   55.7558,
			Longitude:  37.6173,
			Type:       TransportTypeTrain,
			Provider:   "mock",
		},
		{
			ExternalID: "mock_station_2",
			Name:       "Station North",
			Latitude:   55.8016,
			Longitude:  37.5230,
			Type:       TransportTypeTrain,
			Provider:   "mock",
		},
		{
			ExternalID: "mock_station_3",
			Name:       "Bus Terminal South",
			Latitude:   55.7000,
			Longitude:  37.6000,
			Type:       TransportTypeBus,
			Provider:   "mock",
		},
		{
			ExternalID: "mock_station_4",
			Name:       "Bus Stop East",
			Latitude:   55.7500,
			Longitude:  37.7000,
			Type:       TransportTypeBus,
			Provider:   "mock",
		},
	}
}

func generateTestRoutes(from, to *Station, date time.Time, earliestDeparture time.Time, tz string) []*Route {
	routes := []*Route{}

	// Маршрут 1: прямой поезд
	routes = append(routes, &Route{
		ExternalID:    "mock_route_1",
		StartStation:  from,
		EndStation:    to,
		DepartureTime: date.Add(1 * time.Hour),
		ArrivalTime:   date.Add(3*time.Hour + 30*time.Minute),
		TransportType: TransportTypeTrain,
		RouteNumber:   "120",
		IsAvailable:   true,
		Provider:      "mock",
	})

	// Маршрут 2: ещё один поезд
	routes = append(routes, &Route{
		ExternalID:    "mock_route_2",
		StartStation:  from,
		EndStation:    to,
		DepartureTime: date.Add(2 * time.Hour),
		ArrivalTime:   date.Add(4*time.Hour + 15*time.Minute),
		TransportType: TransportTypeTrain,
		RouteNumber:   "121",
		IsAvailable:   true,
		Provider:      "mock",
	})

	// Маршрут 3: автобус
	routes = append(routes, &Route{
		ExternalID:    "mock_route_3",
		StartStation:  from,
		EndStation:    to,
		DepartureTime: date.Add(30 * time.Minute),
		ArrivalTime:   date.Add(2*time.Hour + 45*time.Minute),
		TransportType: TransportTypeBus,
		RouteNumber:   "45",
		IsAvailable:   true,
		Provider:      "mock",
	})

	// Фильтруем по времени выхода
	var filtered []*Route
	for _, route := range routes {
		if route.DepartureTime.After(earliestDeparture) || route.DepartureTime.Equal(earliestDeparture) {
			filtered = append(filtered, route)
		}
	}

	return filtered
}

// haversineDistance вычисляет расстояние между двумя координатами в метрах
func haversineDistance(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 6371000 // метры

	dLat := (lat2 - lat1) * math.Pi / 180
	dLon := (lon2 - lon1) * math.Pi / 180

	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*math.Pi/180)*math.Cos(lat2*math.Pi/180)*
			math.Sin(dLon/2)*math.Sin(dLon/2)

	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return R * c
}
