package transport

import (
	"time"
)

// Station представляет остановку или станцию общественного транспорта
type Station struct {
	// ExternalID — стабильный идентификатор от провайдера
	ExternalID string `json:"external_id"`
	// Name — название станции
	Name string `json:"name"`
	// Latitude и Longitude — координаты
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	// Type — тип транспорта (train, bus, tramway и т.д.)
	Type TransportType `json:"type"`
	// Provider — источник данных
	Provider string `json:"provider"`
}

// TransportType — тип общественного транспорта
type TransportType string

const (
	TransportTypeTrain      TransportType = "train"
	TransportTypeBus        TransportType = "bus"
	TransportTypeTramway    TransportType = "tramway"
	TransportTypeSubway     TransportType = "subway"
	TransportTypeInterurban TransportType = "interurban" // пригородный автобус
)

// Route представляет маршрут между двумя станциями
type Route struct {
	// ExternalID — уникальный идентификатор рейса от провайдера
	ExternalID string `json:"external_id"`
	// StartStation — станция отправления
	StartStation *Station `json:"start_station"`
	// EndStation — станция прибытия
	EndStation *Station `json:"end_station"`
	// DepartureTime — время отправления в UTC
	DepartureTime time.Time `json:"departure_time"`
	// ArrivalTime — время прибытия в UTC
	ArrivalTime time.Time `json:"arrival_time"`
	// TransportType — тип транспорта
	TransportType TransportType `json:"transport_type"`
	// RouteNumber — номер маршрута
	RouteNumber string `json:"route_number"`
	// IsAvailable — доступен ли рейс
	IsAvailable bool `json:"is_available"`
	// AvailabilityReason — причина недоступности (если есть)
	AvailabilityReason string `json:"availability_reason,omitempty"`
	// Provider — источник данных
	Provider string `json:"provider"`
}

// StationSearchResult содержит результаты поиска станций
type StationSearchResult struct {
	Stations []*Station `json:"stations"`
	Error    string     `json:"error,omitempty"` // если есть ошибка
}

// RoutesSearchResult содержит результаты поиска маршрутов
type RoutesSearchResult struct {
	Routes []*Route `json:"routes"`
	Error  string   `json:"error,omitempty"` // если есть ошибка
}

// ResponseMetadata информация об ответе источника
type ResponseMetadata struct {
	// Source — название провайдера
	Source string `json:"source"`
	// Timestamp — когда были получены данные
	Timestamp time.Time `json:"timestamp"`
	// IsReal — реальные данные или mock
	IsReal bool `json:"is_real"`
}
