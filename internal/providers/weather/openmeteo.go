package weather

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

type OpenMeteo struct {
	client  *http.Client
	baseURL string
}

func NewOpenMeteo() *OpenMeteo {
	return &OpenMeteo{
		client: &http.Client{
			Timeout: 10 * time.Second,
			Transport: &http.Transport{
				// Эта строка заставляет Go читать HTTP_PROXY и HTTPS_PROXY из окружения (.env)
				Proxy: http.ProxyFromEnvironment,
			},
		},
		baseURL: "https://api.open-meteo.com/v1/forecast",
	}
}

type openMeteoResponse struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Timezone  string  `json:"timezone"`
	Hourly    struct {
		Time          []string  `json:"time"`
		Temperature2m []float64 `json:"temperature_2m"`
		Precipitation []float64 `json:"precipitation"`
		WeatherCode   []int     `json:"weathercode"`
		WindSpeed10m  []float64 `json:"windspeed_10m"`
	} `json:"hourly"`
}

func (o *OpenMeteo) GetForecast(
	ctx context.Context,
	lat float64,
	lon float64,
	arrivalTime time.Time,
) (*Forecast, error) {
	q := url.Values{}

	q.Set("latitude", strconv.FormatFloat(lat, 'f', 6, 64))
	q.Set("longitude", strconv.FormatFloat(lon, 'f', 6, 64))
	q.Set("hourly", "temperature_2m,precipitation,weathercode,windspeed_10m")
	q.Set("forecast_days", "3")
	q.Set("timezone", "UTC")

	reqURL := o.baseURL + "?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("open-meteo new request: %w", err)
	}

	req.Header.Set("Accept", "application/json")

	resp, err := o.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("open-meteo request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("open-meteo returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 5<<20))
	if err != nil {
		return nil, fmt.Errorf("open-meteo read body: %w", err)
	}

	var raw openMeteoResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("open-meteo decode json: %w", err)
	}

	if len(raw.Hourly.Time) == 0 {
		return nil, ErrForecastUnavailable
	}

	n := len(raw.Hourly.Time)

	if len(raw.Hourly.Temperature2m) != n ||
		len(raw.Hourly.Precipitation) != n ||
		len(raw.Hourly.WeatherCode) != n ||
		len(raw.Hourly.WindSpeed10m) != n {
		return nil, fmt.Errorf("open-meteo response arrays are inconsistent")
	}

	times, err := parseTimes(raw.Hourly.Time)
	if err != nil {
		return nil, err
	}

	target := arrivalTime.UTC()

	// Если запрошенное время сильно вне доступного горизонта прогноза,
	// возвращаем ErrForecastUnavailable.
	if target.Before(times[0].Add(-1*time.Hour)) ||
		target.After(times[len(times)-1].Add(1*time.Hour)) {
		return nil, ErrForecastUnavailable
	}

	idx := closestIndex(times, target)
	if idx < 0 || idx >= n {
		return nil, ErrForecastUnavailable
	}

	return &Forecast{
		Latitude:        lat,
		Longitude:       lon,
		Time:            times[idx],
		TemperatureC:    raw.Hourly.Temperature2m[idx],
		PrecipitationMm: raw.Hourly.Precipitation[idx],
		WeatherCode:     raw.Hourly.WeatherCode[idx],
		WindSpeedKmh:    raw.Hourly.WindSpeed10m[idx],
		Source:          "open-meteo",
		ReceivedAt:      time.Now().UTC(),
	}, nil
}

func parseTimes(values []string) ([]time.Time, error) {
	times := make([]time.Time, 0, len(values))

	for _, v := range values {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			t, err = time.Parse("2006-01-02T15:04", v)
			if err != nil {
				return nil, fmt.Errorf("open-meteo invalid time %q: %w", v, err)
			}
		}

		times = append(times, t.UTC())
	}

	return times, nil
}

func closestIndex(times []time.Time, target time.Time) int {
	best := -1
	var bestDiff time.Duration

	for i, t := range times {
		diff := t.Sub(target)
		if diff < 0 {
			diff = -diff
		}

		if best == -1 || diff < bestDiff {
			best = i
			bestDiff = diff
		}
	}

	return best
}
