package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/vanorro1997-droid/IGoGo/internal/storage"
)

const (
	twoGISItemsURL = "https://catalog.api.2gis.com/3.0/items"
	twoGISKeyEnv   = "TWOGIS_API_KEY"
	// twoGISAPIKey is a fallback for local runs. Prefer the TWOGIS_API_KEY
	// environment variable and do not commit real keys to source control.
	twoGISAPIKey = "f3706770-5041-4b26-b945-a63007d216bf"

	weatherAPIURL    = "https://api.weatherapi.com/v1/current.json"
	weatherAPIKeyEnv = "WEATHERAPI_KEY"
	weatherAPILang   = "ru"
	// weatherAPIKey is a fallback for local runs. Prefer the WEATHERAPI_KEY
	// environment variable and do not commit real keys to source control.
	weatherAPIKey = "d077351def0541169d6171342261011"

	maxResponseSize = 5 << 20 // 5 MB
)

var twoGISClient = &http.Client{Timeout: 10 * time.Second}
var weatherClient = &http.Client{Timeout: 10 * time.Second}

// apiServer связывает HTTP-хендлеры с хранилищем (пользователи и история запросов).
type apiServer struct {
	store *storage.Store
}

func (s *apiServer) twoGISSearchHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	key := strings.TrimSpace(os.Getenv(twoGISKeyEnv))
	if key == "" {
		key = twoGISAPIKey
	}
	if key == "" {
		log.Printf("2GIS: API key is not configured (set %s)", twoGISKeyEnv)
		writeJSONError(w, http.StatusInternalServerError, "2GIS API key is not configured")
		return
	}

	query := r.URL.Query()
	q := strings.TrimSpace(query.Get("q"))
	if q == "" {
		writeJSONError(w, http.StatusBadRequest, `query parameter "q" is required`)
		return
	}

	// 2GIS requires a georestriction for text search.
	if query.Get("location") == "" && query.Get("region_id") == "" && query.Get("city_id") == "" {
		writeJSONError(w, http.StatusBadRequest, `one of "location", "region_id" or "city_id" is required`)
		return
	}

	// Only forward known parameters, so the client cannot override the key.
	params := url.Values{}
	params.Set("q", q)
	for _, name := range []string{
		"location", "region_id", "city_id", "type", "fields",
		"sort", "page", "page_size", "radius", "locale",
	} {
		if v := strings.TrimSpace(query.Get(name)); v != "" {
			params.Set(name, v)
		}
	}
	params.Set("key", key)

	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, twoGISItemsURL+"?"+params.Encode(), nil)
	if err != nil {
		log.Printf("2GIS: failed to build request: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "failed to build 2GIS request")
		return
	}

	resp, err := twoGISClient.Do(req)
	if err != nil {
		log.Printf("2GIS: request failed: %v", err)
		writeJSONError(w, http.StatusBadGateway, "2GIS API is unavailable")
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if err != nil {
		log.Printf("2GIS: failed to read response: %v", err)
		writeJSONError(w, http.StatusBadGateway, "failed to read 2GIS response")
		return
	}

	// 2GIS may return HTTP 200 with an error code inside meta.code.
	status := resp.StatusCode
	var meta struct {
		Meta struct {
			Code int `json:"code"`
		} `json:"meta"`
	}
	if json.Unmarshal(body, &meta) == nil && meta.Meta.Code >= 400 && meta.Meta.Code < 600 {
		status = meta.Meta.Code
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if _, err := w.Write(body); err != nil {
		log.Printf("2GIS: failed to write response: %v", err)
	}

	// Сохраняем результат поиска в search_run / place / search_result.
	if status >= 200 && status < 300 {
		user, ok := userFromContext(r.Context())
		if ok {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := s.store.SaveSearchRun(ctx, user.ID, query, body); err != nil {
				log.Printf("2GIS: failed to save search history: %v", err)
			}
		}
	}
}

// weatherLocation converts the client's "longitude,latitude" (the same format
// used by /api/2gis/search) to WeatherAPI's "latitude,longitude". Non-numeric
// values such as city names are passed through unchanged.
func weatherLocation(location string) string {
	parts := strings.Split(location, ",")
	if len(parts) != 2 {
		return location
	}
	lon, lonErr := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	lat, latErr := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if lonErr != nil || latErr != nil {
		return location
	}
	return fmt.Sprintf("%g,%g", lat, lon)
}

// weatherHandler returns current weather at the given location via WeatherAPI.
// The API key is read from the environment and never exposed to the client.
//
// Example (client sends longitude,latitude):
//
//	GET /api/weather?location=37.630866,55.752256
func (s *apiServer) weatherHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	key := strings.TrimSpace(os.Getenv(weatherAPIKeyEnv))
	if key == "" {
		key = weatherAPIKey
	}
	if key == "" {
		log.Printf("weather: API key is not configured (set %s)", weatherAPIKeyEnv)
		writeJSONError(w, http.StatusInternalServerError, "weather API key is not configured")
		return
	}

	query := r.URL.Query()
	location := strings.TrimSpace(query.Get("location"))
	if location == "" {
		writeJSONError(w, http.StatusBadRequest, `query parameter "location" is required`)
		return
	}

	// Only forward known parameters, so the client cannot override the key.
	params := url.Values{}
	params.Set("q", weatherLocation(location))
	params.Set("lang", weatherAPILang)
	params.Set("aqi", "no")
	params.Set("key", key)

	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, weatherAPIURL+"?"+params.Encode(), nil)
	if err != nil {
		log.Printf("weather: failed to build request: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "failed to build weather request")
		return
	}

	resp, err := weatherClient.Do(req)
	if err != nil {
		log.Printf("weather: request failed: %v", err)
		writeJSONError(w, http.StatusBadGateway, "weather API is unavailable")
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if err != nil {
		log.Printf("weather: failed to read response: %v", err)
		writeJSONError(w, http.StatusBadGateway, "failed to read weather response")
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(resp.StatusCode)
	if _, err := w.Write(body); err != nil {
		log.Printf("weather: failed to write response: %v", err)
	}

	// Сохраняем город и текущую погоду в city / weather_current.
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		_, ok := userFromContext(r.Context())
		if ok {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := s.store.SaveWeather(ctx, body); err != nil {
				log.Printf("weather: failed to save history: %v", err)
			}
		}
	}
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
