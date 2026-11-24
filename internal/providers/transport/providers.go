package transport

import (
	"context"
	"time"
)

// StationProvider интерфейс для поиска станций
type StationProvider interface {
	// SearchNearby возвращает станции возле координат в пределах radiusM метров
	SearchNearby(ctx context.Context, lat, lon float64, radiusM int) (*StationSearchResult, *ResponseMetadata, error)

	// GetStationByID возвращает станцию по внешнему идентификатору
	GetStationByID(ctx context.Context, externalID string) (*Station, *ResponseMetadata, error)

	// Name возвращает название провайдера
	Name() string
}

// TimetableProvider интерфейс для поиска расписаний
type TimetableProvider interface {
	// SearchRoutes ищет рейсы между станциями на заданное время
	// date — дата поездки в часовом поясе tz
	// earliestDeparture — самое раннее время выхода (может быть нулевым)
	SearchRoutes(
		ctx context.Context,
		from *Station,
		to *Station,
		date time.Time,
		earliestDeparture time.Time,
		tz string,
	) (*RoutesSearchResult, *ResponseMetadata, error)

	// SupportsTransport проверяет, поддерживает ли провайдер нужный тип транспорта в регионе
	SupportsTransport(transportType TransportType, region string) bool

	// Name возвращает название провайдера
	Name() string
}
