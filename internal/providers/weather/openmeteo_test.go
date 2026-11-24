package weather

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

const forecastResponse = `{"hourly":{"time":["2026-11-25T12:00","2026-11-25T13:00"],"temperature_2m":[5,6],"precipitation":[0,0.2],"weathercode":[1,3],"windspeed_10m":[10,12]}}`

func TestOpenMeteoForecast(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if query.Get("latitude") != "55.750000" || query.Get("longitude") != "37.620000" ||
			query.Get("timezone") != "UTC" || query.Get("forecast_days") != "3" {
			t.Errorf("unexpected forecast query: %s", r.URL.RawQuery)
		}
		fmt.Fprint(w, forecastResponse)
	}))
	defer server.Close()
	provider := NewOpenMeteo()
	provider.baseURL = server.URL
	arrival := time.Date(2026, 11, 25, 13, 10, 0, 0, time.UTC)
	forecast, err := provider.GetForecast(context.Background(), 55.75, 37.62, arrival)
	if err != nil {
		t.Fatal(err)
	}
	if forecast.Source != "open-meteo" || forecast.TemperatureC != 6 || forecast.PrecipitationMm != 0.2 ||
		forecast.WeatherCode != 3 || forecast.WindSpeedKmh != 12 || forecast.Time.Hour() != 13 {
		t.Fatalf("unexpected forecast: %+v", forecast)
	}
	_, err = provider.GetForecast(context.Background(), 55.75, 37.62, arrival.Add(48*time.Hour))
	if !errors.Is(err, ErrForecastUnavailable) {
		t.Fatalf("out-of-range forecast error = %v", err)
	}
}

func TestOpenMeteoRejectsInvalidResponses(t *testing.T) {
	for _, test := range []struct {
		name, body string
		status     int
	}{
		{"upstream failure", `{"error":true}`, http.StatusBadGateway},
		{"invalid JSON", `{`, http.StatusOK},
		{"empty forecast", `{"hourly":{"time":[]}}`, http.StatusOK},
		{"inconsistent arrays", `{"hourly":{"time":["2026-11-25T12:00"]}}`, http.StatusOK},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(test.status)
				fmt.Fprint(w, test.body)
			}))
			defer server.Close()
			provider := NewOpenMeteo()
			provider.baseURL = server.URL
			if _, err := provider.GetForecast(context.Background(), 55.75, 37.62, time.Now()); err == nil {
				t.Fatal("expected an upstream error")
			}
		})
	}
}

func TestOpenMeteoUsesEnvironmentProxy(t *testing.T) {
	// ProxyFromEnvironment caches its settings; a subprocess isolates the test.
	if os.Getenv("IGOGO_WEATHER_PROXY_HELPER") == "1" {
		provider := NewOpenMeteo()
		provider.baseURL = "http://open-meteo.proxy.test/v1/forecast"
		arrival := time.Date(2026, 11, 25, 13, 0, 0, 0, time.UTC)
		forecast, err := provider.GetForecast(context.Background(), 55.75, 37.62, arrival)
		if err != nil || forecast.Source != "open-meteo" {
			t.Fatalf("forecast through environment proxy failed: %v", err)
		}
		return
	}
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Host != "open-meteo.proxy.test" || r.URL.Path != "/v1/forecast" {
			t.Errorf("unexpected proxied URL: %s", r.URL)
		}
		fmt.Fprint(w, forecastResponse)
	}))
	defer proxy.Close()
	command := exec.Command(os.Args[0], "-test.run=^TestOpenMeteoUsesEnvironmentProxy$")
	for _, value := range os.Environ() {
		key, _, _ := strings.Cut(value, "=")
		switch strings.ToUpper(key) {
		case "HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY", "IGOGO_WEATHER_PROXY_HELPER":
			continue
		}
		command.Env = append(command.Env, value)
	}
	command.Env = append(command.Env, "HTTP_PROXY="+proxy.URL, "HTTPS_PROXY="+proxy.URL,
		"NO_PROXY=", "IGOGO_WEATHER_PROXY_HELPER=1")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("proxy test failed: %v\n%s", err, output)
	}
}
