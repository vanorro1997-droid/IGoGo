package httpapi

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestWeatherLocation(t *testing.T) {
	for _, test := range []struct{ input, want string }{
		{"37.62,55.75", "55.75,37.62"},
		{" -0.12, 51.50 ", "51.5,-0.12"},
		{"Moscow", "Moscow"},
		{"invalid,coordinates", "invalid,coordinates"},
	} {
		if got := weatherLocation(test.input); got != test.want {
			t.Errorf("weatherLocation(%q) = %q, want %q", test.input, got, test.want)
		}
	}
}

func TestUpstreamProxyErrors(t *testing.T) {
	t.Setenv(twoGISKeyEnv, "test-2gis-key")
	t.Setenv(weatherAPIKeyEnv, "test-weather-key")
	oldGIS, oldWeather := twoGISClient, weatherClient
	t.Cleanup(func() { twoGISClient, weatherClient = oldGIS, oldWeather })
	srv := &apiServer{}
	for _, test := range []struct {
		name, path string
		handler    http.HandlerFunc
	}{
		{"2gis", "/api/2gis/search?q=cafe&location=37.62,55.75", srv.twoGISSearchHandler},
		{"weather", "/api/weather?location=37.62,55.75", srv.weatherHandler},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return nil, errors.New("upstream unavailable")
			})}
			twoGISClient, weatherClient = client, client
			response := httptest.NewRecorder()
			test.handler(response, httptest.NewRequest(http.MethodGet, test.path, nil))
			if response.Code != http.StatusBadGateway {
				t.Fatalf("upstream failure status = %d, want 502", response.Code)
			}
			client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusTooManyRequests,
					Body: io.NopCloser(strings.NewReader(`{"error":"quota"}`)), Header: make(http.Header)}, nil
			})
			response = httptest.NewRecorder()
			test.handler(response, httptest.NewRequest(http.MethodGet, test.path, nil))
			if response.Code != http.StatusTooManyRequests {
				t.Fatalf("upstream status = %d, want 429", response.Code)
			}
		})
	}
}
