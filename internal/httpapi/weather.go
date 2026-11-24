package httpapi

import (
	"errors"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/vanorro1997-droid/IGoGo/internal/providers/weather"
)

func makeWeatherHandler(provider weather.Provider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if provider == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "weather provider is not configured"})
			return
		}
		q := r.URL.Query()

		// Дефолтные значения для теста
		lat := 55.7558
		lon := 37.6173
		arrival := time.Now().UTC().Add(2 * time.Hour)

		if v := q.Get("lat"); v != "" {
			parsed, err := strconv.ParseFloat(v, 64)
			if err != nil || math.IsNaN(parsed) || parsed < -90 || parsed > 90 {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid lat"})
				return
			}
			lat = parsed
		}

		if v := q.Get("lon"); v != "" {
			parsed, err := strconv.ParseFloat(v, 64)
			if err != nil || math.IsNaN(parsed) || parsed < -180 || parsed > 180 {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid lon"})
				return
			}
			lon = parsed
		}

		if v := q.Get("arrival_at"); v != "" {
			parsed, err := time.Parse(time.RFC3339, v)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{
					"error": "arrival_at must be RFC3339, example: 2026-11-17T18:00:00Z",
				})
				return
			}
			arrival = parsed.UTC()
		}

		forecast, err := provider.GetForecast(r.Context(), lat, lon, arrival)
		if err != nil {
			if errors.Is(err, weather.ErrForecastUnavailable) {
				writeJSON(w, http.StatusNotFound, map[string]string{
					"error": "forecast unavailable for requested time",
				})
				return
			}

			writeJSON(w, http.StatusBadGateway, map[string]string{
				"error":  "weather provider failed",
				"detail": err.Error(),
			})
			return
		}

		writeJSON(w, http.StatusOK, forecast)
	}
}
