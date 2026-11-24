package app

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vanorro1997-droid/IGoGo/internal/config"
	"github.com/vanorro1997-droid/IGoGo/internal/providers/llm"
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
