package transport

import (
	"time"
)

// Station представляет остановку или станцию общественного транспорта
type Station struct {
	// ExternalID — стабильный идентификатор от провайдера
	ExternalID string
	// Name — название станции
	Name string
	// Latitude и Longitude — координаты
	Latitude  float64
	Longitude float64
	// Type — тип транспорта (train, bus, tramway и т.д.)
	Type TransportType
	// Provider — источник данных
	Provider string
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
	ExternalID string
	// StartStation — станция отправления
	StartStation *Station
	// EndStation — станция прибытия
	EndStation *Station
	// DepartureTime — время отправления в UTC
	DepartureTime time.Time
	// ArrivalTime — время прибытия в UTC
	ArrivalTime time.Time
	// TransportType — тип транспорта
	TransportType TransportType
	// RouteNumber — номер маршрута
	RouteNumber string
	// IsAvailable — доступен ли рейс
	IsAvailable bool
	// AvailabilityReason — причина недоступности (если есть)
	AvailabilityReason string
	// Provider — источник данных
	Provider string
}

// StationSearchResult содержит результаты поиска станций
type StationSearchResult struct {
	Stations []*Station
	Error    string // если есть ошибка
}

// RoutesSearchResult содержит результаты поиска маршрутов
type RoutesSearchResult struct {
	Routes []*Route
	Error  string // если есть ошибка
}

// ResponseMetadata информация об ответе источника
type ResponseMetadata struct {
	// Source — название провайдера
	Source string
	// Timestamp — когда были получены данные
	Timestamp time.Time
	// IsReal — реальные данные или mock
	IsReal bool
}
