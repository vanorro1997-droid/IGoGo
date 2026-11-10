package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

const (
	twoGISItemsURL = "https://catalog.api.2gis.com/3.0/items"
	twoGISKeyEnv   = "TWOGIS_API_KEY"
	// twoGISAPIKey is a fallback for local runs. Prefer the TWOGIS_API_KEY
	// environment variable and do not commit real keys to source control.
	twoGISAPIKey    = "f3706770-5041-4b26-b945-a63007d216bf"
	maxResponseSize = 5 << 20 // 5 MB
)

var twoGISClient = &http.Client{Timeout: 10 * time.Second}

func main() {
	http.HandleFunc("/api/health", healthHandler)
	http.HandleFunc("/api/data", dataHandler)
	http.HandleFunc("/api/2gis/search", twoGISSearchHandler)
	http.HandleFunc("/", rootHandler)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}

	log.Printf("Server starting on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func rootHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	frontendDir := os.Getenv("FRONTEND_DIR")
	if frontendDir == "" {
		frontendDir = "frontend"
	}

	// Clean the path and keep it inside frontendDir to prevent traversal.
	cleanPath := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	filePath := filepath.Join(frontendDir, filepath.FromSlash(cleanPath))

	if info, err := os.Stat(filePath); err == nil && !info.IsDir() {
		http.ServeFile(w, r, filePath)
		return
	}

	// Fallback for SPA routes.
	http.ServeFile(w, r, filepath.Join(frontendDir, "index.html"))
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func dataHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "Hello from Go API"})
}

// twoGISSearchHandler proxies search requests to the 2GIS Places API.
// The API key is read from the environment and never exposed to the client.
//
// Example:
//
//	GET /api/2gis/search?q=кафе&location=37.630866,55.752256&page_size=10
func twoGISSearchHandler(w http.ResponseWriter, r *http.Request) {
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
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}
