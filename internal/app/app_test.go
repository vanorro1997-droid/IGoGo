package app

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vanorro1997-droid/IGoGo/internal/config"
	"github.com/vanorro1997-droid/IGoGo/internal/providers/llm"
	"github.com/vanorro1997-droid/IGoGo/internal/providers/transport"
	"github.com/vanorro1997-droid/IGoGo/internal/storage"
)

func TestNewHandlerWiresModules(t *testing.T) {
	handler, err := NewHandler(config.Config{WeatherProvider: "mock"})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		method, path string
		status       int
	}{
		{http.MethodGet, "/api/health", http.StatusOK},
		{http.MethodGet, "/api/data", http.StatusOK},
		{http.MethodGet, "/api/weather", http.StatusOK},
		{http.MethodGet, "/api/transport/stations?lat=55.7558&lon=37.6173", http.StatusOK},
		{http.MethodGet, "/api/transport/routes?from_id=mock_station_1&to_id=mock_station_2&date=2026-11-25", http.StatusOK},
		{http.MethodPost, "/api/llm", http.StatusServiceUnavailable},
	} {
		t.Run(test.path, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(test.method, test.path, nil))
			if response.Code != test.status {
				t.Fatalf("status = %d, want %d: %s", response.Code, test.status, response.Body.String())
			}
		})
	}
}

func TestNewHandlerOpenMeteoInitialization(t *testing.T) {
	// Инициализация не должна обращаться к внешнему API.
	if _, err := NewHandler(config.Config{WeatherProvider: "open-meteo"}); err != nil {
		t.Fatal(err)
	}
}

func TestNewHandlerWiresYandexTransport(t *testing.T) {
	previousTransport := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = previousTransport })
	calls := map[string]int{}
	http.DefaultTransport = appRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "api.rasp.yandex-net.ru" || r.Header.Get("Authorization") != "test-rasp-key" || r.URL.Query().Get("apikey") != "" {
			t.Error("Expected Yandex API requests with credentials only in Authorization")
		}
		calls[r.URL.Path]++
		var body string
		switch r.URL.Path {
		case "/v3.0/nearest_stations/":
			body = `{"stations":[{"code":"s1","title":"Station 1","lat":55.7558,"lng":37.6173,"transport_type":"train"}]}`
		case "/v3.0/schedule/":
			id := r.URL.Query().Get("station")
			if id != "s1" && id != "s2" {
				t.Fatal("Unexpected station ID")
			}
			body = `{"station":{"code":"` + id + `","title":"Station","transport_type":"train"},"schedule":[]}`
		case "/v3.0/search/":
			if r.URL.Query().Get("date") != "2026-11-25" || r.URL.Query().Get("result_timezone") != "Europe/Moscow" {
				t.Error("Expected the local date and timezone to reach the Yandex provider")
			}
			body = `{"segments":[{"departure":"2026-11-25T10:00:00+03:00","arrival":"2026-11-25T12:00:00+03:00","thread":{"uid":"yandex-route","number":"120","transport_type":"train"}}]}`
		default:
			t.Fatalf("Unexpected Yandex API path: %s", r.URL.Path)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})
	handler, err := NewHandler(config.Config{WeatherProvider: "mock", TransportProvider: "yandex", YandexRaspAPIKey: "test-rasp-key"})
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 0 {
		t.Fatal("Provider initialization must not call external APIs")
	}
	stations := httptest.NewRecorder()
	handler.ServeHTTP(stations, httptest.NewRequest(http.MethodGet, "/api/transport/stations?lat=55.7558&lon=37.6173", nil))
	var stationsResponse struct {
		Stations []*transport.Station       `json:"stations"`
		Metadata transport.ResponseMetadata `json:"metadata"`
	}
	if stations.Code != http.StatusOK {
		t.Fatalf("Stations status = %d: %s", stations.Code, stations.Body.String())
	}
	if err := json.Unmarshal(stations.Body.Bytes(), &stationsResponse); err != nil {
		t.Fatal(err)
	}
	if len(stationsResponse.Stations) != 1 || stationsResponse.Stations[0].ExternalID != "s1" ||
		stationsResponse.Metadata.Source != "yandex" || !stationsResponse.Metadata.IsReal {
		t.Fatal("Station endpoint must use the configured Yandex provider")
	}
	routes := httptest.NewRecorder()
	handler.ServeHTTP(routes, httptest.NewRequest(http.MethodGet, "/api/transport/routes?from_id=s1&to_id=s2&date=2026-11-25&timezone=Europe/Moscow&earliest_departure=2026-11-25T07:00:00Z", nil))
	var routesResponse struct {
		Routes   []*transport.Route         `json:"routes"`
		Metadata transport.ResponseMetadata `json:"metadata"`
	}
	if routes.Code != http.StatusOK {
		t.Fatalf("Routes status = %d: %s", routes.Code, routes.Body.String())
	}
	if err := json.Unmarshal(routes.Body.Bytes(), &routesResponse); err != nil {
		t.Fatal(err)
	}
	if len(routesResponse.Routes) != 1 || routesResponse.Routes[0].ExternalID != "yandex-route" ||
		routesResponse.Routes[0].DepartureTime.Format("15:04") != "07:00" ||
		routesResponse.Metadata.Source != "yandex" || !routesResponse.Metadata.IsReal {
		t.Fatal("Route endpoint must use the configured Yandex provider and UTC times")
	}
	if calls["/v3.0/nearest_stations/"] != 1 || calls["/v3.0/schedule/"] != 2 || calls["/v3.0/search/"] != 1 {
		t.Fatalf("Unexpected provider calls: %v", calls)
	}
}

type appRoundTripFunc func(*http.Request) (*http.Response, error)

func (f appRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestNewHandlerRejectsInvalidTransportConfiguration(t *testing.T) {
	for _, cfg := range []config.Config{
		{WeatherProvider: "mock", TransportProvider: "unknown"},
		{WeatherProvider: "mock", TransportProvider: "yandex"},
		{WeatherProvider: "mock", TransportProvider: "yandex", YandexRaspAPIKey: "   "},
	} {
		if _, err := NewHandler(cfg); err == nil {
			t.Fatal("Expected an invalid transport configuration error")
		}
	}
}

func TestNewHandlerRejectsUnknownWeatherProvider(t *testing.T) {
	if _, err := NewHandler(config.Config{WeatherProvider: "unknown"}); err == nil {
		t.Fatal("expected an unsupported provider error")
	}
}

func TestNewHandlerRequiresCredentialsOnlyWhenLLMEnabled(t *testing.T) {
	cfg := config.Config{WeatherProvider: "mock", LLMEnabled: true}
	if _, err := NewHandler(cfg); err == nil {
		t.Fatal("enabled LLM must require credentials")
	}
	cfg.LLM = llm.Config{APIKey: "test-token", AccountID: "test-account"}
	if _, err := NewHandler(cfg); err != nil {
		t.Fatalf("LLM initialization must not make external requests: %v", err)
	}
}

func TestDatabaseHandlerProtectsBusinessEndpoints(t *testing.T) {
	// Запрос без токена должен отклоняться до обращения к БД.
	handler, err := newHandler(config.Config{WeatherProvider: "mock"}, &storage.Store{})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/data", "/api/weather", "/api/transport/stations", "/api/history", "/api/2gis/search"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("%s status = %d, want 401", path, response.Code)
		}
	}
}
