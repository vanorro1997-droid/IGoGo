package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const (
	yandexAPIBaseURL = "https://api.rasp.yandex.net"
)

// YandexStationProvider реализует поиск станций через Яндекс Расписание
type YandexStationProvider struct {
	apiKey string
	client *http.Client
}

// YandexTimetableProvider реализует поиск расписаний через Яндекс Расписание
type YandexTimetableProvider struct {
	apiKey string
	client *http.Client
}

// NewYandexStationProvider создаёт провайдера для поиска станций Яндекс
func NewYandexStationProvider(apiKey string) *YandexStationProvider {
	return &YandexStationProvider{
		apiKey: apiKey,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// NewYandexTimetableProvider создаёт провайдера для расписаний Яндекс
func NewYandexTimetableProvider(apiKey string) *YandexTimetableProvider {
	return &YandexTimetableProvider{
		apiKey: apiKey,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// SearchNearby ищет станции возле координат
func (y *YandexStationProvider) SearchNearby(ctx context.Context, lat, lon float64, radiusM int) (*StationSearchResult, *ResponseMetadata, error) {
	// Яндекс расписание работает с кодами станций, а не с координатами напрямую
	// Используем простой поиск, который можно расширить

	// Яндекс API ограничивает расстояние до 50км максимум
	distance := radiusM / 1000 // Convert to km
	if distance > 50 {
		distance = 50
	}
	if distance < 1 {
		distance = 1
	}

	params := url.Values{}
	params.Set("apikey", y.apiKey)
	params.Set("lat", fmt.Sprintf("%.4f", lat))
	params.Set("lng", fmt.Sprintf("%.4f", lon)) // Note: Yandex uses 'lng' not 'lon'
	params.Set("distance", fmt.Sprintf("%d", distance))
	params.Set("format", "json")

	fullURL := fmt.Sprintf("%s/v3.0/nearest_stations/?%s", yandexAPIBaseURL, params.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := y.client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("API error: status %d, body: %s", resp.StatusCode, string(body))
	}

	var yandexResp YandexNearestStationsResponse
	if err := json.Unmarshal(body, &yandexResp); err != nil {
		return nil, nil, fmt.Errorf("failed to parse response: %w", err)
	}

	stations := make([]*Station, 0, len(yandexResp.StationsWithDist))
	for _, dist := range yandexResp.StationsWithDist {
		s := dist.Station
		stations = append(stations, &Station{
			ExternalID: s.Code,
			Name:       s.Title,
			Latitude:   s.Latitude,
			Longitude:  s.Longitude,
			Type:       parseTransportType(s.TransportType),
			Provider:   "yandex",
		})
	}

	return &StationSearchResult{
		Stations: stations,
	}, &ResponseMetadata{
		Source:    "yandex",
		Timestamp: time.Now().UTC(),
		IsReal:    true,
	}, nil
}

// GetStationByID возвращает станцию по ID
func (y *YandexStationProvider) GetStationByID(ctx context.Context, externalID string) (*Station, *ResponseMetadata, error) {
	params := url.Values{}
	params.Set("apikey", y.apiKey)
	params.Set("code", externalID)
	params.Set("format", "json")

	fullURL := fmt.Sprintf("%s/v3.0/station/%s/?%s", yandexAPIBaseURL, externalID, params.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := y.client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("API error: status %d, body: %s", resp.StatusCode, string(body))
	}

	var yandexStation YandexStation
	if err := json.Unmarshal(body, &yandexStation); err != nil {
		return nil, nil, fmt.Errorf("failed to parse response: %w", err)
	}

	station := &Station{
		ExternalID: yandexStation.Code,
		Name:       yandexStation.Title,
		Latitude:   yandexStation.Latitude,
		Longitude:  yandexStation.Longitude,
		Type:       parseTransportType(yandexStation.TransportType),
		Provider:   "yandex",
	}

	return station, &ResponseMetadata{
		Source:    "yandex",
		Timestamp: time.Now().UTC(),
		IsReal:    true,
	}, nil
}

// Name возвращает название провайдера
func (y *YandexStationProvider) Name() string {
	return "yandex_stations"
}

// SearchRoutes ищет рейсы между станциями
func (y *YandexTimetableProvider) SearchRoutes(
	ctx context.Context,
	from *Station,
	to *Station,
	date time.Time,
	earliestDeparture time.Time,
	tz string,
) (*RoutesSearchResult, *ResponseMetadata, error) {
	params := url.Values{}
	params.Set("apikey", y.apiKey)
	params.Set("from", from.ExternalID)
	params.Set("to", to.ExternalID)
	params.Set("date", date.Format("2006-01-02"))
	params.Set("format", "json")

	fullURL := fmt.Sprintf("%s/v3.0/search/?%s", yandexAPIBaseURL, params.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := y.client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("API error: status %d, body: %s", resp.StatusCode, string(body))
	}

	var yandexResp YandexSearchResponse
	if err := json.Unmarshal(body, &yandexResp); err != nil {
		return nil, nil, fmt.Errorf("failed to parse response: %w", err)
	}

	routes := convertYandexSegmentsToRoutes(yandexResp.Segments, from, to, earliestDeparture)

	return &RoutesSearchResult{
		Routes: routes,
	}, &ResponseMetadata{
		Source:    "yandex",
		Timestamp: time.Now().UTC(),
		IsReal:    true,
	}, nil
}

// SupportsTransport проверяет, поддерживает ли провайдер нужный тип транспорта
func (y *YandexTimetableProvider) SupportsTransport(transportType TransportType, region string) bool {
	// Яндекс поддерживает основные виды транспорта
	switch transportType {
	case TransportTypeTrain, TransportTypeBus, TransportTypeSubway, TransportTypeTramway:
		return true
	default:
		return false
	}
}

// Name возвращает название провайдера
func (y *YandexTimetableProvider) Name() string {
	return "yandex_timetable"
}

// Вспомогательные типы для парсинга Яндекс API

type YandexStation struct {
	Code            string `json:"code"`
	Title           string `json:"title"`
	Popular_title   string `json:"popular_title"`
	TransportType   string `json:"transport_type"`
	Country         string `json:"country"`
	City            string `json:"city"`
	TZ              string `json:"timezone"`
	YandexID        string `json:"yandex_id"`
	YandexGeoID     string `json:"yandex_geo_id"`
	Latitude        float64 `json:"lat"`
	Longitude       float64 `json:"lon"`
	StructuredTitle map[string]interface{} `json:"structured_title"`
}

type YandexStationWithDist struct {
	Station  YandexStation `json:"station"`
	Distance float64       `json:"distance"`
}

type YandexNearestStationsResponse struct {
	StationsWithDist []YandexStationWithDist `json:"stations"`
}

type YandexSegment struct {
	From struct {
		Code  string `json:"code"`
		Title string `json:"title"`
	} `json:"from"`
	To struct {
		Code  string `json:"code"`
		Title string `json:"title"`
	} `json:"to"`
	StartTime         string `json:"departure"`
	EndTime           string `json:"arrival"`
	ThreadNumber      string `json:"thread_uid"`
	Transport         struct {
		Code string `json:"code"`
		Type string `json:"type"`
	} `json:"transport"`
	Vehicle      string `json:"vehicle_number"`
	Available    int    `json:"available_seats"`
	IsRecommended bool `json:"is_recommended"`
	Interval     interface{} `json:"interval"`
}

type YandexSearchResponse struct {
	Segments []YandexSegment `json:"segments"`
}

// parseTransportType преобразует строку из Яндекс в TransportType
func parseTransportType(t string) TransportType {
	switch t {
	case "train", "suburban":
		return TransportTypeTrain
	case "bus":
		return TransportTypeBus
	case "subway":
		return TransportTypeSubway
	case "tramway":
		return TransportTypeTramway
	default:
		return TransportTypeBus
	}
}

// convertYandexSegmentsToRoutes конвертирует сегменты Яндекс в Route
func convertYandexSegmentsToRoutes(segments []YandexSegment, from *Station, to *Station, earliest time.Time) []*Route {
	routes := make([]*Route, 0, len(segments))

	for _, seg := range segments {
		// Парсим время отправления и прибытия
		departTime, err := time.Parse("2006-01-02T15:04:05", seg.StartTime)
		if err != nil {
			continue
		}

		arriveTime, err := time.Parse("2006-01-02T15:04:05", seg.EndTime)
		if err != nil {
			continue
		}

		// Фильтруем по времени выхода
		if !departTime.After(earliest) && !departTime.Equal(earliest) {
			continue
		}

		route := &Route{
			ExternalID:    seg.ThreadNumber,
			StartStation:  from,
			EndStation:    to,
			DepartureTime: departTime.UTC(),
			ArrivalTime:   arriveTime.UTC(),
			TransportType: parseTransportType(seg.Transport.Type),
			RouteNumber:   seg.Vehicle,
			IsAvailable:   seg.Available > 0,
			Provider:      "yandex",
		}

		routes = append(routes, route)
	}

	return routes
}
