package httpapi

import (
	"errors"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/vanorro1997-droid/IGoGo/internal/providers/transport"
)

func makeStationsHandler(provider transport.StationProvider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if provider == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "transport provider is not configured"})
			return
		}
		q := r.URL.Query()
		lat, err := parseCoordinate(q.Get("lat"), -90, 90)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid lat"})
			return
		}
		lon, err := parseCoordinate(q.Get("lon"), -180, 180)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid lon"})
			return
		}
		radiusM := 10000
		if value := q.Get("radius_m"); value != "" {
			radiusM, err = strconv.Atoi(value)
			if err != nil || radiusM < 1 || radiusM > 100000 {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "radius_m must be between 1 and 100000"})
				return
			}
		}

		result, metadata, err := provider.SearchNearby(r.Context(), lat, lon, radiusM)
		if err != nil || result == nil || metadata == nil || result.Error != "" {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "station provider failed"})
			return
		}
		stations := result.Stations
		if stations == nil {
			stations = []*transport.Station{}
		}
		writeJSON(w, http.StatusOK, struct {
			Stations []*transport.Station        `json:"stations"`
			Metadata *transport.ResponseMetadata `json:"metadata"`
		}{stations, metadata})
	}
}

func makeRoutesHandler(stations transport.StationProvider, timetable transport.TimetableProvider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if stations == nil || timetable == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "transport provider is not configured"})
			return
		}
		q := r.URL.Query()
		fromID, toID := q.Get("from_id"), q.Get("to_id")
		if fromID == "" || toID == "" || fromID == toID {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "from_id and to_id must identify different stations"})
			return
		}
		timezone := q.Get("timezone")
		if timezone == "" {
			timezone = "UTC"
		}
		location, err := time.LoadLocation(timezone)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid timezone"})
			return
		}
		date, err := time.ParseInLocation(time.DateOnly, q.Get("date"), location)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "date must be YYYY-MM-DD"})
			return
		}
		earliest := date.UTC()
		if value := q.Get("earliest_departure"); value != "" {
			earliest, err = time.Parse(time.RFC3339, value)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "earliest_departure must be RFC3339"})
				return
			}
		}

		from, _, err := stations.GetStationByID(r.Context(), fromID)
		if err != nil {
			writeStationError(w, err)
			return
		}
		to, _, err := stations.GetStationByID(r.Context(), toID)
		if err != nil {
			writeStationError(w, err)
			return
		}
		if from == nil || to == nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "station provider failed"})
			return
		}

		result, metadata, err := timetable.SearchRoutes(r.Context(), from, to, date.UTC(), earliest.UTC(), timezone)
		if err != nil || result == nil || metadata == nil || result.Error != "" {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "timetable provider failed"})
			return
		}
		routes := result.Routes
		if routes == nil {
			routes = []*transport.Route{}
		}
		writeJSON(w, http.StatusOK, struct {
			Routes   []*transport.Route          `json:"routes"`
			Metadata *transport.ResponseMetadata `json:"metadata"`
		}{routes, metadata})
	}
}

func parseCoordinate(value string, min, max float64) (float64, error) {
	coordinate, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsNaN(coordinate) || coordinate < min || coordinate > max {
		return 0, errors.New("invalid coordinate")
	}
	return coordinate, nil
}

func writeStationError(w http.ResponseWriter, err error) {
	if errors.Is(err, transport.ErrStationNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "station not found"})
		return
	}
	writeJSON(w, http.StatusBadGateway, map[string]string{"error": "station provider failed"})
}
