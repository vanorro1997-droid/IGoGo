package weather

import (
	"context"
	"time"
)

type MockProvider struct{}

func NewMockProvider() Provider { return &MockProvider{} }

func (m *MockProvider) GetForecast(
	ctx context.Context,
	lat float64,
	lon float64,
	arrivalTime time.Time,
) (*Forecast, error) {
	// Имитация задержки сети, чтобы видеть разницу с реальным API
	time.Sleep(50 * time.Millisecond)

	return &Forecast{
		Latitude:        lat,
		Longitude:       lon,
		Time:            arrivalTime.UTC(),
		TemperatureC:    15.5,
		PrecipitationMm: 0.0,
		WeatherCode:     0,
		WindSpeedKmh:    3.2,
		Source:          "mock",
		ReceivedAt:      time.Now().UTC(),
	}, nil
}
