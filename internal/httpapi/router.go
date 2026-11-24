package httpapi

import (
	"net/http"

	"github.com/vanorro1997-droid/IGoGo/internal/providers/transport"
	"github.com/vanorro1997-droid/IGoGo/internal/providers/weather"
	"github.com/vanorro1997-droid/IGoGo/internal/storage"
)

type Dependencies struct {
	Weather   weather.Provider
	Stations  transport.StationProvider
	Timetable transport.TimetableProvider
	LLM       LLMClient
	Store     *storage.Store
}

func NewRouter(frontendDir string, deps Dependencies) http.Handler {
	mux := http.NewServeMux()
	register := func(path, method string, handler http.HandlerFunc) {
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			if r.Method != method {
				w.Header().Set("Allow", method)
				writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
				return
			}
			handler(w, r)
		})
	}

	auth := newAuthService(deps.Store)
	srv := &apiServer{store: deps.Store}
	protected := func(handler http.HandlerFunc) http.HandlerFunc {
		if deps.Store == nil {
			return handler
		}
		return auth.requireAuth(logRequests(deps.Store, handler)).ServeHTTP
	}
	databaseOnly := func(handler http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if deps.Store == nil {
				writeJSONError(w, http.StatusServiceUnavailable, "database is not configured")
				return
			}
			handler(w, r)
		}
	}
	forecastHandler := makeWeatherHandler(deps.Weather)
	currentWeatherHandler := databaseOnly(srv.weatherHandler)
	weatherHandler := func(w http.ResponseWriter, r *http.Request) {
		// Сохраняем оба контракта: location для UI, lat/lon/arrival_at для прогноза.
		if r.URL.Query().Has("location") {
			currentWeatherHandler(w, r)
			return
		}
		forecastHandler(w, r)
	}

	register("/api/health", http.MethodGet, healthHandler)
	register("/api/data", http.MethodGet, protected(dataHandler))
	register("/api/weather", http.MethodGet, protected(weatherHandler))
	register("/api/transport/stations", http.MethodGet, protected(makeStationsHandler(deps.Stations)))
	register("/api/transport/routes", http.MethodGet, protected(makeRoutesHandler(deps.Stations, deps.Timetable)))
	register("/api/llm", http.MethodPost, protected(makeLLMHandler(deps.LLM)))
	register("/api/auth/register", http.MethodPost, databaseOnly(auth.registerHandler))
	register("/api/auth/login", http.MethodPost, databaseOnly(auth.loginHandler))
	register("/api/auth/logout", http.MethodPost, databaseOnly(auth.requireAuth(http.HandlerFunc(auth.logoutHandler)).ServeHTTP))
	register("/api/me", http.MethodGet, databaseOnly(auth.requireAuth(http.HandlerFunc(auth.meHandler)).ServeHTTP))
	register("/api/history", http.MethodGet, databaseOnly(protected(srv.historyHandler)))
	register("/api/2gis/search", http.MethodGet, databaseOnly(protected(srv.twoGISSearchHandler)))

	notFound := func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	}
	mux.HandleFunc("/api", notFound)
	mux.HandleFunc("/api/", notFound)
	if frontendDir == "" {
		mux.HandleFunc("/", notFound)
	} else {
		mux.Handle("/", makeFrontendHandler(frontendDir))
	}
	return mux
}

func dataHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"message": "Hello from Go API"})
}
