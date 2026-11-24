package httpapi

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vanorro1997-droid/IGoGo/internal/storage"
)

func integrationStore(t *testing.T) (*storage.Store, *sql.DB) {
	t.Helper()
	dsn := os.Getenv("IGOGO_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set IGOGO_TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	parsed, err := url.Parse(dsn)
	if err != nil || parsed.User == nil || parsed.Hostname() == "" {
		t.Fatal("IGOGO_TEST_DATABASE_URL must be a PostgreSQL URL")
	}
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	var suffix [8]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		t.Fatal(err)
	}
	name := "igogo_integration_" + hex.EncodeToString(suffix[:])
	// Only a freshly created database is modified or removed by this test.
	if _, err := admin.Exec("CREATE DATABASE " + name); err != nil {
		_ = admin.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec("DROP DATABASE " + name); err != nil {
			t.Errorf("clean up integration database: %v", err)
		}
		_ = admin.Close()
	})
	parsed.Path = "/" + name
	db, err := sql.Open("pgx", parsed.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	schema, err := os.ReadFile(filepath.Join("..", "..", "db", "schema.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(schema)); err != nil {
		t.Fatal(err)
	}
	password, _ := parsed.User.Password()
	port := parsed.Port()
	if port == "" {
		port = "5432"
	}
	sslmode := parsed.Query().Get("sslmode")
	if sslmode == "" {
		sslmode = "disable"
	}
	for key, value := range map[string]string{
		"DB_HOST": parsed.Hostname(), "DB_PORT": port, "DB_USER": parsed.User.Username(),
		"DB_PASSWORD": password, "DB_NAME": name, "DB_SSLMODE": sslmode,
	} {
		t.Setenv(key, value)
	}
	store, err := storage.OpenStore()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store, db
}

func TestPostgresIntegratedAPI(t *testing.T) {
	store, db := integrationStore(t)
	t.Setenv(twoGISKeyEnv, "test-2gis-key")
	t.Setenv(weatherAPIKeyEnv, "test-weather-key")
	oldGIS, oldWeather := twoGISClient, weatherClient
	t.Cleanup(func() { twoGISClient, weatherClient = oldGIS, oldWeather })
	searchBody := `{"meta":{"code":200},"result":{"total":2,"items":[{"id":"9007199254740993","name":"Cafe","address_name":"Test street","type":"branch","point":{"lat":55.75,"lon":37.62}},{"id":9007199254740995,"name":"Park","point":{"lat":55.76,"lon":37.63}}]}}`
	weatherBody := `{"location":{"name":"Moscow","region":"Moscow","country":"Russia","tz_id":"Europe/Moscow","localtime":"2026-11-25 15:00","lat":55.75,"lon":37.62},"current":{"temp_c":5,"feelslike_c":3,"wind_kph":12,"wind_dir":"N","humidity":70,"precip_mm":0.2,"condition":{"code":1003,"text":"Cloudy","icon":"//example.test/cloud.png"}}}`
	twoGISClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		query := r.URL.Query()
		if query.Get("key") != "test-2gis-key" || query.Get("q") != "cafe" ||
			query.Get("location") != "37.62,55.75" || query.Has("unexpected") {
			t.Errorf("2GIS query forwarding changed: %s", r.URL.RawQuery)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header),
			Body: io.NopCloser(strings.NewReader(searchBody))}, nil
	})}
	weatherClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		query := r.URL.Query()
		if query.Get("key") != "test-weather-key" || query.Get("q") != "55.75,37.62" ||
			query.Get("lang") != "ru" || query.Get("aqi") != "no" || query.Has("unexpected") {
			t.Errorf("WeatherAPI query forwarding changed: %s", r.URL.RawQuery)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header),
			Body: io.NopCloser(strings.NewReader(weatherBody))}, nil
	})}
	deps := testDependencies()
	deps.Store = store
	deps.LLM = &llmStub{}
	handler := NewRouter("", deps)
	request := func(method, path, body, token string, want int) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		if response.Code != want || !json.Valid(response.Body.Bytes()) {
			t.Fatalf("%s %s = %d, want %d: %s", method, path, response.Code, want, response.Body.String())
		}
		return response
	}
	type session struct {
		User      storage.User `json:"user"`
		Token     string       `json:"token"`
		TokenType string       `json:"token_type"`
	}
	decodeSession := func(response *httptest.ResponseRecorder) session {
		t.Helper()
		var value session
		if err := json.Unmarshal(response.Body.Bytes(), &value); err != nil {
			t.Fatal(err)
		}
		if value.User.ID == 0 || len(value.Token) != 64 || value.TokenType != "Bearer" {
			t.Fatal("registration/login response contract changed")
		}
		return value
	}
	request(http.MethodGet, "/api/health", "", "", http.StatusOK)
	for _, path := range []string{"/api/data", "/api/me", "/api/history", "/api/weather",
		"/api/weather?location=37.62,55.75", "/api/2gis/search", "/api/transport/stations", "/api/transport/routes"} {
		request(http.MethodGet, path, "", "", http.StatusUnauthorized)
	}
	request(http.MethodPost, "/api/llm", `{"prompt":"test"}`, "", http.StatusUnauthorized)
	request(http.MethodGet, "/api/me", "", "invalid-token", http.StatusUnauthorized)
	request(http.MethodPost, "/api/auth/register", `{"email":"invalid","password":"test-password"}`, "", http.StatusBadRequest)
	request(http.MethodPost, "/api/auth/register", `{"email":"test@example.test","password":"short"}`, "", http.StatusBadRequest)
	registered := decodeSession(request(http.MethodPost, "/api/auth/register",
		`{"email":" User@Example.test ","password":"test-password","display_name":" Test User "}`, "", http.StatusCreated))
	if registered.User.Email != "user@example.test" || registered.User.DisplayName != "Test User" || registered.User.Role != "user" {
		t.Fatalf("unexpected registered user: %+v", registered.User)
	}
	request(http.MethodPost, "/api/auth/register", `{"email":"user@example.test","password":"test-password"}`, "", http.StatusConflict)
	request(http.MethodPost, "/api/auth/login", `{"email":"user@example.test","password":"wrong-password"}`, "", http.StatusUnauthorized)
	request(http.MethodPost, "/api/auth/login", `{"email":"unknown@example.test","password":"test-password"}`, "", http.StatusUnauthorized)
	loggedIn := decodeSession(request(http.MethodPost, "/api/auth/login",
		`{"email":"USER@example.test","password":"test-password"}`, "", http.StatusOK))
	token := loggedIn.Token
	if loggedIn.User.ID != registered.User.ID || token == registered.Token {
		t.Fatal("login must create a new session for the existing user")
	}
	request(http.MethodGet, "/api/me", "", token, http.StatusOK)
	data := request(http.MethodGet, "/api/data?q=visible&token=hidden&password=hidden&key=hidden&api_key=hidden", "", token, http.StatusOK)
	if !strings.Contains(data.Body.String(), "Hello from Go API") {
		t.Fatal("existing data response changed")
	}
	for i := 0; i < 2; i++ {
		search := request(http.MethodGet, "/api/2gis/search?q=cafe&location=37.62,55.75&key=client-key&unexpected=ignored", "", token, http.StatusOK)
		if strings.TrimSpace(search.Body.String()) != searchBody {
			t.Fatal("2GIS response contract changed")
		}
		current := request(http.MethodGet, "/api/weather?location=37.62,55.75&key=client-key&unexpected=ignored", "", token, http.StatusOK)
		if strings.TrimSpace(current.Body.String()) != weatherBody {
			t.Fatal("WeatherAPI response contract changed")
		}
	}
	forecast := request(http.MethodGet, "/api/weather?lat=55.75&lon=37.62&arrival_at=2026-11-25T12:00:00Z", "", token, http.StatusOK)
	if !strings.Contains(forecast.Body.String(), `"source":"mock"`) {
		t.Fatal("forecast provider must remain available alongside WeatherAPI")
	}
	request(http.MethodGet, "/api/weather?location=", "", token, http.StatusBadRequest)
	request(http.MethodGet, "/api/2gis/search?q=cafe", "", token, http.StatusBadRequest)
	request(http.MethodGet, "/api/transport/stations?lat=55.7558&lon=37.6173", "", token, http.StatusOK)
	request(http.MethodGet, "/api/transport/routes?"+routeQuery, "", token, http.StatusOK)
	llm := request(http.MethodPost, "/api/llm", `{"prompt":"test"}`, token, http.StatusOK)
	if !strings.Contains(llm.Body.String(), "test answer") {
		t.Fatal("LLM endpoint must remain available with authentication")
	}
	searchBody = `{"meta":{"code":404},"result":{}}`
	request(http.MethodGet, "/api/2gis/search?q=cafe&location=37.62,55.75", "", token, http.StatusNotFound)
	for _, test := range []struct {
		query string
		want  int
	}{
		{"SELECT count(*) FROM app_user", 1},
		{"SELECT count(*) FROM search_run", 2},
		{"SELECT count(*) FROM place", 2},
		{"SELECT count(*) FROM search_result", 4},
		{"SELECT count(*) FROM weather_current", 1},
		{"SELECT count(*) FROM city", 1},
		{"SELECT count(*) FROM place WHERE external_id IN ('9007199254740993', '9007199254740995')", 2},
	} {
		var count int
		if err := db.QueryRow(test.query).Scan(&count); err != nil || count != test.want {
			t.Fatalf("%s = %d, want %d; error = %v", test.query, count, test.want, err)
		}
	}
	var passwordHash, tokenHash string
	if err := db.QueryRow("SELECT password_hash FROM app_user WHERE id = $1", registered.User.ID).Scan(&passwordHash); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow("SELECT token_hash FROM user_session WHERE token_hash = $1", hashToken(token)).Scan(&tokenHash); err != nil {
		t.Fatal(err)
	}
	if passwordHash == "test-password" || tokenHash == token {
		t.Fatal("passwords and tokens must not be stored in plaintext")
	}
	history := request(http.MethodGet, "/api/history?limit=200", "", token, http.StatusOK)
	var result struct {
		Items []storage.RequestHistoryItem `json:"items"`
	}
	if err := json.Unmarshal(history.Body.Bytes(), &result); err != nil || len(result.Items) < 10 {
		t.Fatalf("request history is incomplete: %v", err)
	}
	foundData, foundForecast, foundCurrent, foundError := false, false, false, false
	for _, item := range result.Items {
		var query url.Values
		if err := json.Unmarshal(item.QueryParams, &query); err != nil {
			t.Fatal(err)
		}
		for _, key := range []string{"token", "password", "key", "api_key"} {
			if query.Has(key) {
				t.Fatal("credentials must not be recorded in request history")
			}
		}
		if item.Path == "/api/data" && query.Get("q") == "visible" {
			foundData = true
		}
		if item.Path == "/api/weather" && query.Has("arrival_at") && item.Provider == nil {
			foundForecast = true
		}
		if item.Path == "/api/weather" && query.Has("location") && item.Provider != nil && *item.Provider == "weatherapi" {
			foundCurrent = true
		}
		if item.Path == "/api/2gis/search" && item.StatusCode == http.StatusNotFound {
			foundError = true
		}
	}
	if !foundData || !foundForecast || !foundCurrent || !foundError {
		t.Fatal("history must include old/new endpoints, providers and failed requests")
	}
	limited := request(http.MethodGet, "/api/history?limit=1", "", token, http.StatusOK)
	if err := json.Unmarshal(limited.Body.Bytes(), &result); err != nil || len(result.Items) != 1 {
		t.Fatal("history limit must be respected")
	}
	other := decodeSession(request(http.MethodPost, "/api/auth/register",
		`{"email":"other@example.test","password":"test-password"}`, "", http.StatusCreated))
	otherHistory := request(http.MethodGet, "/api/history", "", other.Token, http.StatusOK)
	if err := json.Unmarshal(otherHistory.Body.Bytes(), &result); err != nil || len(result.Items) != 0 {
		t.Fatal("users must not see another user's history")
	}
	request(http.MethodPost, "/api/auth/logout", "", token, http.StatusOK)
	request(http.MethodGet, "/api/me", "", token, http.StatusUnauthorized)
	request(http.MethodGet, "/api/me", "", registered.Token, http.StatusOK)
	if _, err := db.Exec("UPDATE user_session SET created_at = now() - interval '2 hours', expires_at = now() - interval '1 hour' WHERE token_hash = $1", hashToken(registered.Token)); err != nil {
		t.Fatal(err)
	}
	request(http.MethodGet, "/api/me", "", registered.Token, http.StatusUnauthorized)
	if _, err := db.Exec("UPDATE app_user SET status = 'blocked' WHERE id = $1", other.User.ID); err != nil {
		t.Fatal(err)
	}
	request(http.MethodGet, "/api/me", "", other.Token, http.StatusUnauthorized)
}
