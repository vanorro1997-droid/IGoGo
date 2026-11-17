package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"

	weather "github.com/vanorro1997-droid/IGoGo/internal/providers/weather"
)

func main() {
	http.HandleFunc("/api/health", healthHandler)
	http.HandleFunc("/api/data", dataHandler)
	http.HandleFunc("/", rootHandler)

	// http.HandleFunc("/api/weather", makeWeatherHandler(weather.NewOpenMeteo()))
	http.HandleFunc("/api/weather", makeWeatherHandler(weather.NewMockProvider())) //пока что затычка, так как open weather без впн не работает

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Server starting on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func rootHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" {
		http.ServeFile(w, r, "/app/frontend/index.html")
	} else {
		http.ServeFile(w, r, "/app/frontend/"+r.URL.Path)
	}
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func dataHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "Hello from Go API"})
}
