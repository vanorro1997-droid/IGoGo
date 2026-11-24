package httpapi

import (
	"net/http"

	"github.com/vanorro1997-droid/IGoGo/internal/providers/transport"
	"github.com/vanorro1997-droid/IGoGo/internal/providers/weather"
)

type Dependencies struct {
	Weather   weather.Provider
	Stations  transport.StationProvider
	Timetable transport.TimetableProvider
	LLM       LLMClient
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

	register("/api/health", http.MethodGet, healthHandler)
	register("/api/data", http.MethodGet, dataHandler)
	register("/api/weather", http.MethodGet, makeWeatherHandler(deps.Weather))
	register("/api/transport/stations", http.MethodGet, makeStationsHandler(deps.Stations))
	register("/api/transport/routes", http.MethodGet, makeRoutesHandler(deps.Stations, deps.Timetable))
	register("/api/llm", http.MethodPost, makeLLMHandler(deps.LLM))

	notFound := func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	}
	mux.HandleFunc("/api", notFound)
	mux.HandleFunc("/api/", notFound)
	if frontendDir == "" {
		mux.HandleFunc("/", notFound)
	} else {
		mux.Handle("/", http.FileServer(http.Dir(frontendDir)))
	}
	return mux
}

func dataHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"message": "Hello from Go API"})
}
