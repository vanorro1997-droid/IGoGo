package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const (
	yandexAPIBaseURL      = "https://api.rasp.yandex-net.ru"
	yandexMaxResponseSize = 5 << 20
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
	if radiusM < 1 {
		return nil, nil, fmt.Errorf("radius_m must be positive")
	}
	// Передаём километры без округления радиуса до целого километра.
	distance := float64(radiusM) / 1000
	if distance > 50 {
		distance = 50
	}

	params := url.Values{}
	params.Set("lat", strconv.FormatFloat(lat, 'f', -1, 64))
	params.Set("lng", strconv.FormatFloat(lon, 'f', -1, 64))
	params.Set("distance", strconv.FormatFloat(distance, 'f', -1, 64))
	params.Set("transport_types", "train,suburban,bus")
	params.Set("limit", "50")

	body, err := yandexGet(ctx, y.client, y.apiKey, "/v3.0/nearest_stations/", params)
	if err != nil {
		return nil, nil, err
	}

	var yandexResp YandexNearestStationsResponse
	if err := json.Unmarshal(body, &yandexResp); err != nil {
		return nil, nil, fmt.Errorf("failed to parse response: %w", err)
	}

	stations := make([]*Station, 0, len(yandexResp.StationsWithDist))
	for _, dist := range yandexResp.StationsWithDist {
		s := dist.YandexStation
		if s.Code == "" {
			// Совместимость с ранее использовавшимся вложенным форматом.
			s = dist.Station
		}
		if s.Code == "" {
			return nil, nil, fmt.Errorf("Yandex response contains a station without a code")
		}
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
	params.Set("station", externalID)
	params.Set("limit", "1")

	// У API нет /station/{code}/: сведения о станции входят в /schedule/.
	body, err := yandexGet(ctx, y.client, y.apiKey, "/v3.0/schedule/", params)
	if err != nil {
		return nil, nil, err
	}

	var yandexResp YandexScheduleResponse
	if err := json.Unmarshal(body, &yandexResp); err != nil {
		return nil, nil, fmt.Errorf("failed to parse response: %w", err)
	}
	yandexStation := yandexResp.Station
	if yandexStation.Code == "" || yandexStation.Code != externalID {
		return nil, nil, fmt.Errorf("Yandex response does not contain the requested station")
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
	if from == nil || to == nil || from.ExternalID == "" || to.ExternalID == "" {
		return nil, nil, fmt.Errorf("from and to stations are required")
	}
	if tz == "" {
		tz = "UTC"
	}
	location, err := time.LoadLocation(tz)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid timezone: %w", err)
	}

	params := url.Values{}
	params.Set("from", from.ExternalID)
	params.Set("to", to.ExternalID)
	params.Set("date", date.In(location).Format(time.DateOnly))
	params.Set("result_timezone", tz)
	params.Set("transport_types", "train,suburban,bus")
	params.Set("transfers", "false")

	body, err := yandexGet(ctx, y.client, y.apiKey, "/v3.0/search/", params)
	if err != nil {
		return nil, nil, err
	}

	var yandexResp YandexSearchResponse
	if err := json.Unmarshal(body, &yandexResp); err != nil {
		return nil, nil, fmt.Errorf("failed to parse response: %w", err)
	}

	routes := convertYandexSegmentsToRoutes(yandexResp.Segments, from, to, earliestDeparture, location)

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
	// Расписания покрывают междугородние автобусы, поезда и электрички,
	// но не городские метро и трамваи.
	switch transportType {
	case TransportTypeTrain, TransportTypeBus, TransportTypeInterurban:
		return true
	default:
		return false
	}
}

// Name возвращает название провайдера
func (y *YandexTimetableProvider) Name() string {
	return "yandex_timetable"
}

func yandexGet(ctx context.Context, client *http.Client, apiKey, path string, params url.Values) ([]byte, error) {
	params.Set("format", "json")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, yandexAPIBaseURL+path+"?"+params.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	// Официальный API принимает ключ в Authorization: он не попадёт в URL
	// сетевых ошибок или в возвращаемое API тело ошибки.
	req.Header.Set("Authorization", apiKey)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("Yandex API: %w", ErrStationNotFound)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Yandex API returned status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, yandexMaxResponseSize+1))
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	if len(body) > yandexMaxResponseSize {
		return nil, fmt.Errorf("Yandex response exceeds size limit")
	}
	return body, nil
}

// Вспомогательные типы для парсинга Яндекс API

type YandexStation struct {
	Code            string                 `json:"code"`
	Title           string                 `json:"title"`
	Popular_title   string                 `json:"popular_title"`
	TransportType   string                 `json:"transport_type"`
	Country         string                 `json:"country"`
	City            string                 `json:"city"`
	TZ              string                 `json:"timezone"`
	YandexID        string                 `json:"yandex_id"`
	YandexGeoID     string                 `json:"yandex_geo_id"`
	Latitude        float64                `json:"lat"`
	Longitude       float64                `json:"lng"`
	StructuredTitle map[string]interface{} `json:"structured_title"`
}

type YandexStationWithDist struct {
	YandexStation
	Station  YandexStation `json:"station"`
	Distance float64       `json:"distance"`
}

type YandexNearestStationsResponse struct {
	StationsWithDist []YandexStationWithDist `json:"stations"`
}

type YandexScheduleResponse struct {
	Station YandexStation `json:"station"`
}

type YandexThread struct {
	UID           string `json:"uid"`
	Number        string `json:"number"`
	TransportType string `json:"transport_type"`
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
	StartTime    string       `json:"departure"`
	EndTime      string       `json:"arrival"`
	Thread       YandexThread `json:"thread"`
	ThreadNumber string       `json:"thread_uid"`
	Transport    struct {
		Code string `json:"code"`
		Type string `json:"type"`
	} `json:"transport"`
	Vehicle       string      `json:"vehicle_number"`
	Available     int         `json:"available_seats"`
	IsRecommended bool        `json:"is_recommended"`
	Interval      interface{} `json:"interval"`
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
func convertYandexSegmentsToRoutes(segments []YandexSegment, from *Station, to *Station, earliest time.Time, location *time.Location) []*Route {
	routes := make([]*Route, 0, len(segments))

	for _, seg := range segments {
		// Парсим время отправления и прибытия
		departTime, err := parseYandexTime(seg.StartTime, location)
		if err != nil {
			continue
		}

		arriveTime, err := parseYandexTime(seg.EndTime, location)
		if err != nil {
			continue
		}

		// Фильтруем по времени выхода
		if departTime.Before(earliest) {
			continue
		}

		id, number, transportType := seg.Thread.UID, seg.Thread.Number, seg.Thread.TransportType
		// API Расписаний не сообщает количество свободных мест.
		// Доступность означает наличие рейса в расписании, не бронь билета.
		available := true
		if id == "" {
			// Сохраняем поддержку старых ответов с полями верхнего уровня.
			id, number, transportType = seg.ThreadNumber, seg.Vehicle, seg.Transport.Type
			available = seg.Available > 0
		}
		if id == "" {
			continue
		}

		route := &Route{
			ExternalID:    id,
			StartStation:  from,
			EndStation:    to,
			DepartureTime: departTime.UTC(),
			ArrivalTime:   arriveTime.UTC(),
			TransportType: parseTransportType(transportType),
			RouteNumber:   number,
			IsAvailable:   available,
			Provider:      "yandex",
		}

		routes = append(routes, route)
	}

	return routes
}

func parseYandexTime(value string, location *time.Location) (time.Time, error) {
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed, nil
	}
	for _, layout := range []string{"2006-01-02T15:04:05", "2006-01-02 15:04:05"} {
		if parsed, err := time.ParseInLocation(layout, value, location); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid Yandex timestamp")
}
