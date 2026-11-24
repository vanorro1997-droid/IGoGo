package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vanorro1997-droid/IGoGo/internal/providers/transport"
	"github.com/vanorro1997-droid/IGoGo/internal/providers/weather"
)

func testDependencies() Dependencies {
	return Dependencies{
		Weather:   weather.NewMockProvider(),
		Stations:  transport.NewMockStationProvider(),
		Timetable: transport.NewMockTimetableProvider(),
	}
}

func requestJSON(t *testing.T, handler http.Handler, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	response := httptest.NewRecorder()
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(response, request)
	if !strings.HasPrefix(response.Header().Get("Content-Type"), "application/json") || !json.Valid(response.Body.Bytes()) {
		t.Fatalf("expected a JSON response: %s", response.Body.String())
	}
	return response
}

func TestExistingHealthAndDataResponses(t *testing.T) {
	handler := NewRouter("", testDependencies())
	for _, test := range []struct {
		path, body string
	}{
		{"/api/health", `{"status":"ok"}`},
		{"/api/data", `{"message":"Hello from Go API"}`},
	} {
		t.Run(test.path, func(t *testing.T) {
			response := requestJSON(t, handler, http.MethodGet, test.path, "")
			if response.Code != http.StatusOK || strings.TrimSpace(response.Body.String()) != test.body {
				t.Fatalf("unexpected existing response: %d %s", response.Code, response.Body.String())
			}
		})
	}
}

func TestMethodsAndUnknownAPIPaths(t *testing.T) {
	handler := NewRouter("", testDependencies())
	for _, test := range []struct {
		method, path, allow string
		status              int
	}{
		{http.MethodPost, "/api/health", http.MethodGet, http.StatusMethodNotAllowed},
		{http.MethodPost, "/api/data", http.MethodGet, http.StatusMethodNotAllowed},
		{http.MethodPost, "/api/weather", http.MethodGet, http.StatusMethodNotAllowed},
		{http.MethodPost, "/api/transport/stations", http.MethodGet, http.StatusMethodNotAllowed},
		{http.MethodPost, "/api/transport/routes", http.MethodGet, http.StatusMethodNotAllowed},
		{http.MethodGet, "/api/llm", http.MethodPost, http.StatusMethodNotAllowed},
		{http.MethodGet, "/api/unknown", "", http.StatusNotFound},
		{http.MethodGet, "/api", "", http.StatusNotFound},
	} {
		t.Run(test.method+test.path, func(t *testing.T) {
			response := requestJSON(t, handler, test.method, test.path, "")
			if response.Code != test.status || response.Header().Get("Allow") != test.allow {
				t.Fatalf("status = %d, Allow = %q", response.Code, response.Header().Get("Allow"))
			}
		})
	}
}

func TestWeatherValidation(t *testing.T) {
	handler := NewRouter("", testDependencies())
	for _, query := range []string{
		"lat=91", "lat=NaN", "lat=Inf", "lat=invalid",
		"lon=-181", "lon=NaN", "lon=Inf", "arrival_at=invalid",
	} {
		t.Run(query, func(t *testing.T) {
			response := requestJSON(t, handler, http.MethodGet, "/api/weather?"+query, "")
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", response.Code)
			}
		})
	}
}

type weatherStub struct {
	forecast *weather.Forecast
	err      error
	arrival  time.Time
	lat, lon float64
}

func (s *weatherStub) GetForecast(_ context.Context, lat, lon float64, arrival time.Time) (*weather.Forecast, error) {
	s.lat, s.lon, s.arrival = lat, lon, arrival
	return s.forecast, s.err
}

func TestWeatherPreservesForecastContract(t *testing.T) {
	arrival := time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)
	provider := &weatherStub{forecast: &weather.Forecast{
		Latitude: 55.7558, Longitude: 37.6173, Time: arrival,
		TemperatureC: 15.5, Source: "fixture", ReceivedAt: arrival,
	}}
	deps := testDependencies()
	deps.Weather = provider
	response := requestJSON(t, NewRouter("", deps), http.MethodGet,
		"/api/weather?lat=55.7558&lon=37.6173&arrival_at=2026-11-25T15:00:00%2B03:00", "")
	var forecast weather.Forecast
	if err := json.Unmarshal(response.Body.Bytes(), &forecast); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || forecast.Source != "fixture" || forecast.TemperatureC != 15.5 ||
		!forecast.Time.Equal(arrival) || provider.lat != 55.7558 || provider.lon != 37.6173 ||
		!provider.arrival.Equal(arrival) || provider.arrival.Location() != time.UTC {
		t.Fatalf("weather contract or query forwarding changed: %s", response.Body.String())
	}
}

func TestWeatherProviderErrors(t *testing.T) {
	for _, test := range []struct {
		err    error
		status int
	}{
		{fmt.Errorf("wrapped: %w", weather.ErrForecastUnavailable), http.StatusNotFound},
		{errors.New("provider unavailable"), http.StatusBadGateway},
	} {
		t.Run(test.err.Error(), func(t *testing.T) {
			deps := testDependencies()
			deps.Weather = &weatherStub{err: test.err}
			response := requestJSON(t, NewRouter("", deps), http.MethodGet, "/api/weather", "")
			if response.Code != test.status {
				t.Fatalf("status = %d, want %d", response.Code, test.status)
			}
		})
	}
}

func TestTransportStationResponses(t *testing.T) {
	handler := NewRouter("", testDependencies())
	for _, test := range []struct {
		query string
		count int
	}{
		{"lat=55.7558&lon=37.6173&radius_m=10000", 4},
		{"lat=0&lon=0", 0},
	} {
		t.Run(test.query, func(t *testing.T) {
			response := requestJSON(t, handler, http.MethodGet, "/api/transport/stations?"+test.query, "")
			var result struct {
				Stations []*transport.Station        `json:"stations"`
				Metadata *transport.ResponseMetadata `json:"metadata"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if response.Code != http.StatusOK || result.Stations == nil || len(result.Stations) != test.count ||
				result.Metadata == nil || result.Metadata.IsReal || result.Metadata.Source != "mock" {
				t.Fatalf("unexpected stations or missing mock marker: %s", response.Body.String())
			}
		})
	}
}

func TestTransportStationValidation(t *testing.T) {
	handler := NewRouter("", testDependencies())
	for _, query := range []string{
		"", "lat=55.7558", "lat=NaN&lon=0", "lat=0&lon=181",
		"lat=0&lon=0&radius_m=0", "lat=0&lon=0&radius_m=100001", "lat=0&lon=0&radius_m=abc",
	} {
		t.Run(query, func(t *testing.T) {
			response := requestJSON(t, handler, http.MethodGet, "/api/transport/stations?"+query, "")
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", response.Code)
			}
		})
	}
}

const routeQuery = "from_id=mock_station_1&to_id=mock_station_2&date=2026-11-25"

func TestTransportRoutesAndDepartureFilter(t *testing.T) {
	handler := NewRouter("", testDependencies())
	for _, test := range []struct {
		query string
		count int
	}{
		{routeQuery, 3},
		{routeQuery + "&earliest_departure=2026-11-25T02:00:00Z", 1},
		{routeQuery + "&earliest_departure=2026-11-26T00:00:00Z", 0},
	} {
		t.Run(test.query, func(t *testing.T) {
			response := requestJSON(t, handler, http.MethodGet, "/api/transport/routes?"+test.query, "")
			var result struct {
				Routes   []*transport.Route          `json:"routes"`
				Metadata *transport.ResponseMetadata `json:"metadata"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if response.Code != http.StatusOK || result.Routes == nil || len(result.Routes) != test.count ||
				result.Metadata == nil || result.Metadata.IsReal || result.Metadata.Source != "mock" {
				t.Fatalf("unexpected routes or missing mock marker: %s", response.Body.String())
			}
			for _, route := range result.Routes {
				if route.StartStation.ExternalID != "mock_station_1" || route.EndStation.ExternalID != "mock_station_2" ||
					route.DepartureTime.Location() != time.UTC {
					t.Fatal("station IDs and UTC timestamps must be preserved")
				}
			}
		})
	}
}

func TestTransportRoutesTimezone(t *testing.T) {
	response := requestJSON(t, NewRouter("", testDependencies()), http.MethodGet,
		"/api/transport/routes?"+routeQuery+"&timezone=Europe%2FMoscow", "")
	var result struct {
		Routes []*transport.Route `json:"routes"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 11, 24, 22, 0, 0, 0, time.UTC)
	if response.Code != http.StatusOK || len(result.Routes) != 3 || !result.Routes[0].DepartureTime.Equal(want) {
		t.Fatalf("date must be interpreted in the requested timezone: %s", response.Body.String())
	}
}

func TestTransportRoutesValidation(t *testing.T) {
	handler := NewRouter("", testDependencies())
	for _, query := range []string{
		"", "from_id=a&to_id=a&date=2026-11-25", "from_id=a&to_id=b&date=invalid",
		routeQuery + "&timezone=Invalid%2FZone", routeQuery + "&earliest_departure=invalid",
	} {
		t.Run(query, func(t *testing.T) {
			response := requestJSON(t, handler, http.MethodGet, "/api/transport/routes?"+query, "")
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", response.Code)
			}
		})
	}

	response := requestJSON(t, handler, http.MethodGet,
		"/api/transport/routes?from_id=missing&to_id=mock_station_2&date=2026-11-25", "")
	if response.Code != http.StatusNotFound {
		t.Fatalf("unknown station status = %d, want 404", response.Code)
	}
}

type failingStations struct {
	transport.StationProvider
}

func (s failingStations) GetStationByID(context.Context, string) (*transport.Station, *transport.ResponseMetadata, error) {
	return nil, nil, errors.New("upstream unavailable")
}

func TestTransportDoesNotTreatUpstreamFailureAsMissingStation(t *testing.T) {
	deps := testDependencies()
	deps.Stations = failingStations{deps.Stations}
	response := requestJSON(t, NewRouter("", deps), http.MethodGet, "/api/transport/routes?"+routeQuery, "")
	if response.Code != http.StatusBadGateway {
		t.Fatalf("upstream failure status = %d, want 502", response.Code)
	}
}

type llmStub struct {
	prompt string
	ctx    context.Context
	err    error
	calls  int
}

func (s *llmStub) AskCtx(ctx context.Context, prompt string) (string, error) {
	s.prompt, s.ctx = prompt, ctx
	s.calls++
	return "test answer", s.err
}

func (s *llmStub) Model() string { return "test-model" }

func TestLLMDisabled(t *testing.T) {
	response := requestJSON(t, NewRouter("", testDependencies()), http.MethodPost, "/api/llm", `{"prompt":"test"}`)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("disabled LLM status = %d, want 503", response.Code)
	}
}

func TestLLMRequestAndContext(t *testing.T) {
	client := &llmStub{}
	deps := testDependencies()
	deps.LLM = client
	request := httptest.NewRequest(http.MethodPost, "/api/llm", strings.NewReader(`{"prompt":"  test prompt  "}`))
	ctx, cancel := context.WithCancel(request.Context())
	defer cancel()
	response := httptest.NewRecorder()
	NewRouter("", deps).ServeHTTP(response, request.WithContext(ctx))
	if response.Code != http.StatusOK || client.prompt != "test prompt" || client.ctx != ctx || client.calls != 1 ||
		!strings.Contains(response.Body.String(), `"text":"test answer"`) || !strings.Contains(response.Body.String(), `"model":"test-model"`) {
		t.Fatalf("LLM request was not forwarded correctly: %s", response.Body.String())
	}
}

func TestLLMRejectsInvalidBodiesBeforeCallingProvider(t *testing.T) {
	for _, test := range []struct {
		name, body string
		status     int
	}{
		{"empty", "", http.StatusBadRequest},
		{"missing prompt", `{}`, http.StatusBadRequest},
		{"blank prompt", `{"prompt":"  "}`, http.StatusBadRequest},
		{"unknown field", `{"prompt":"test","extra":true}`, http.StatusBadRequest},
		{"multiple objects", `{"prompt":"test"}{}`, http.StatusBadRequest},
		{"invalid JSON", `{"prompt":`, http.StatusBadRequest},
		{"too large", `{"prompt":"` + strings.Repeat("x", 20<<10) + `"}`, http.StatusRequestEntityTooLarge},
		{"too much padding", `{"prompt":"test"}` + strings.Repeat(" ", 20<<10), http.StatusRequestEntityTooLarge},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := &llmStub{}
			deps := testDependencies()
			deps.LLM = client
			response := requestJSON(t, NewRouter("", deps), http.MethodPost, "/api/llm", test.body)
			if response.Code != test.status || client.calls != 0 {
				t.Fatalf("status = %d, want %d; provider calls = %d", response.Code, test.status, client.calls)
			}
		})
	}
}

func TestLLMProviderErrorsAreSanitized(t *testing.T) {
	deps := testDependencies()
	deps.LLM = &llmStub{err: errors.New("test-token should not be returned")}
	response := requestJSON(t, NewRouter("", deps), http.MethodPost, "/api/llm", `{"prompt":"test"}`)
	if response.Code != http.StatusBadGateway || strings.Contains(response.Body.String(), "test-token") {
		t.Fatal("LLM provider errors must not expose credentials")
	}
}

func TestStaticFrontendUsesConfiguredDirectory(t *testing.T) {
	root := t.TempDir()
	frontend := filepath.Join(root, "frontend")
	if err := os.Mkdir(frontend, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(frontend, "index.html"), []byte("<h1>local frontend</h1>"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "outside.env"), []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	handler := NewRouter(frontend, testDependencies())
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "local frontend") {
		t.Fatalf("configured frontend was not served: %s", response.Body.String())
	}
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/%2e%2e/outside.env", nil))
	if response.Code != http.StatusNotFound || strings.Contains(response.Body.String(), "private") {
		t.Fatal("files outside the frontend directory must not be served")
	}
	response = requestJSON(t, handler, http.MethodGet, "/api/unknown", "")
	if response.Code != http.StatusNotFound {
		t.Fatal("unknown API paths must not fall through to static files")
	}
}
