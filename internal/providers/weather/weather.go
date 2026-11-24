package weather

import (
	"context"
	"errors"
	"time"
)

var ErrForecastUnavailable = errors.New("forecast unavailable")

type Forecast struct {
	Latitude        float64   `json:"latitude"`
	Longitude       float64   `json:"longitude"`
	Time            time.Time `json:"time"`
	TemperatureC    float64   `json:"temperature_c"`
	PrecipitationMm float64   `json:"precipitation_mm"`
	WeatherCode     int       `json:"weather_code"`
	WindSpeedKmh    float64   `json:"wind_speed_kmh"`
	Source          string    `json:"source"`
	ReceivedAt      time.Time `json:"received_at"`
}

type Provider interface {
	GetForecast(ctx context.Context, lat, lon float64, arrivalTime time.Time) (*Forecast, error)
}
