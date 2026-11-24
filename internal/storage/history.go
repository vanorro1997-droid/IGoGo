package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

func parseExternalID(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return ""
	}
	switch v := value.(type) {
	case json.Number:
		return v.String()
	case string:
		return v
	default:
		return fmt.Sprint(v)
	}
}

// SaveSearchRun сохраняет успешный ответ 2ГИС в search_run / place / search_result.
func (s *Store) SaveSearchRun(ctx context.Context, userID int64, query url.Values, body []byte) error {
	var payload struct {
		Meta struct {
			Code int `json:"code"`
		} `json:"meta"`
		Result struct {
			Total int               `json:"total"`
			Items []json.RawMessage `json:"items"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return fmt.Errorf("parse 2gis response: %w", err)
	}
	if payload.Meta.Code >= 400 {
		return nil
	}

	var cityID any // NULL, если город не известен
	if externalID := strings.TrimSpace(query.Get("city_id")); externalID != "" {
		var id int64
		if err := s.db.QueryRowContext(ctx, `SELECT id FROM city WHERE external_id = $1`, externalID).Scan(&id); err == nil {
			cityID = id
		}
	}

	total := payload.Result.Total
	if total <= 0 {
		total = len(payload.Result.Items)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin search tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var runID int64
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO search_run (user_id, saved_search_id, status, started_at, finished_at, result_count)
		VALUES ($1, NULL, 'success', now(), now(), $2)
		RETURNING id
	`, userID, total).Scan(&runID); err != nil {
		return fmt.Errorf("insert search_run: %w", err)
	}

	for i, raw := range payload.Result.Items {
		var item struct {
			ID          json.RawMessage `json:"id"`
			Name        string          `json:"name"`
			AddressName string          `json:"address_name"`
			Type        string          `json:"type"`
			Point       struct {
				Lat float64 `json:"lat"`
				Lon float64 `json:"lon"`
			} `json:"point"`
		}
		if err := json.Unmarshal(raw, &item); err != nil {
			continue
		}
		externalID := parseExternalID(item.ID)
		if externalID == "" || (item.Point.Lat == 0 && item.Point.Lon == 0) {
			continue
		}

		var placeID int64
		if err := tx.QueryRowContext(ctx, `
			INSERT INTO place (external_id, city_id, name, address, type, latitude, longitude, raw)
			VALUES ($1, $2, $3, NULLIF($4, ''), NULLIF($5, ''), $6, $7, $8)
			ON CONFLICT (external_id) DO UPDATE
			    SET name = EXCLUDED.name,
			        address = EXCLUDED.address,
			        type = EXCLUDED.type,
			        latitude = EXCLUDED.latitude,
			        longitude = EXCLUDED.longitude,
			        raw = EXCLUDED.raw,
			        updated_at = now()
			RETURNING id
		`, externalID, cityID, item.Name, item.AddressName, item.Type,
			item.Point.Lat, item.Point.Lon, []byte(raw)).Scan(&placeID); err != nil {
			return fmt.Errorf("upsert place: %w", err)
		}

		if _, err := tx.ExecContext(ctx, `
			INSERT INTO search_result (search_run_id, place_id, rank)
			VALUES ($1, $2, $3)
			ON CONFLICT (search_run_id, place_id) DO NOTHING
		`, runID, placeID, i+1); err != nil {
			return fmt.Errorf("insert search_result: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit search tx: %w", err)
	}
	return nil
}

// SaveWeather сохраняет текущую погоду и город ответа WeatherAPI в weather_current/city.
func (s *Store) SaveWeather(ctx context.Context, body []byte) error {
	var payload struct {
		Location struct {
			Name      string  `json:"name"`
			Region    string  `json:"region"`
			Country   string  `json:"country"`
			TzID      string  `json:"tz_id"`
			Localtime string  `json:"localtime"`
			Lat       float64 `json:"lat"`
			Lon       float64 `json:"lon"`
		} `json:"location"`
		Current struct {
			TempC      float64 `json:"temp_c"`
			FeelslikeC float64 `json:"feelslike_c"`
			WindKph    float64 `json:"wind_kph"`
			WindDir    string  `json:"wind_dir"`
			Humidity   int     `json:"humidity"`
			PrecipMm   float64 `json:"precip_mm"`
			Condition  struct {
				Code int    `json:"code"`
				Text string `json:"text"`
				Icon string `json:"icon"`
			} `json:"condition"`
		} `json:"current"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return fmt.Errorf("parse weather response: %w", err)
	}
	if payload.Current.TempC == 0 && payload.Current.Condition.Text == "" {
		return nil // похоже на error-ответ WeatherAPI
	}

	externalID := fmt.Sprintf("weatherapi:%.6f,%.6f", payload.Location.Lat, payload.Location.Lon)
	var cityID int64
	if err := s.db.QueryRowContext(ctx, `
		INSERT INTO city (external_id, name, region, country, latitude, longitude, timezone)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (external_id) DO UPDATE
		    SET name = EXCLUDED.name,
		        region = EXCLUDED.region,
		        country = EXCLUDED.country,
		        latitude = EXCLUDED.latitude,
		        longitude = EXCLUDED.longitude,
		        timezone = EXCLUDED.timezone,
		        updated_at = now()
		RETURNING id
	`, externalID, payload.Location.Name, payload.Location.Region, payload.Location.Country,
		payload.Location.Lat, payload.Location.Lon, payload.Location.TzID).Scan(&cityID); err != nil {
		return fmt.Errorf("upsert city: %w", err)
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO weather_current
		    (city_id, observed_at, local_time_label, temperature_c, feelslike_c,
		     condition_code, condition_text, icon_url, wind_kph, wind_dir,
		     humidity, precip_mm)
		VALUES ($1, now(), $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (city_id) DO UPDATE
		    SET observed_at = now(),
		        local_time_label = EXCLUDED.local_time_label,
		        temperature_c = EXCLUDED.temperature_c,
		        feelslike_c = EXCLUDED.feelslike_c,
		        condition_code = EXCLUDED.condition_code,
		        condition_text = EXCLUDED.condition_text,
		        icon_url = EXCLUDED.icon_url,
		        wind_kph = EXCLUDED.wind_kph,
		        wind_dir = EXCLUDED.wind_dir,
		        humidity = EXCLUDED.humidity,
		        precip_mm = EXCLUDED.precip_mm,
		        updated_at = now()
	`, cityID, payload.Location.Localtime, payload.Current.TempC, payload.Current.FeelslikeC,
		payload.Current.Condition.Code, payload.Current.Condition.Text, payload.Current.Condition.Icon,
		payload.Current.WindKph, payload.Current.WindDir, payload.Current.Humidity, payload.Current.PrecipMm)
	if err != nil {
		return fmt.Errorf("upsert weather_current: %w", err)
	}
	return nil
}
