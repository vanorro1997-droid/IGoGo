# Интеграция Яндекс Расписания в IGoGo

## API Ключ

Ваш Яндекс API ключ для тестирования:
```
c7e69f5a-84d0-496f-b1ec-38c184f1b26e
```

## Использование

### 1. Установка API ключа в окружение

```bash
export YANDEX_API_KEY=c7e69f5a-84d0-496f-b1ec-38c184f1b26e
```

Или в `.env` файле для локальной разработки:
```
YANDEX_API_KEY=c7e69f5a-84d0-496f-b1ec-38c184f1b26e
```

### 2. Использование в коде

```go
import "github.com/vanorro1997-droid/IGoGo/internal/providers/transport"

// Создание провайдеров
stationProvider := transport.NewYandexStationProvider(apiKey)
timetableProvider := transport.NewYandexTimetableProvider(apiKey)

// Поиск станций рядом
result, metadata, err := stationProvider.SearchNearby(ctx, 55.7558, 37.6173, 500)

// Получение информации о станции
station, metadata, err := stationProvider.GetStationByID(ctx, "station_id")

// Поиск маршрутов
routes, metadata, err := timetableProvider.SearchRoutes(
    ctx,
    fromStation,
    toStation,
    searchDate,
    earliestDeparture,
    "Europe/Moscow",
)
```

## Поддерживаемые типы транспорта

Яндекс провайдер поддерживает:
- ✅ Поезда (train)
- ✅ Автобусы (bus)  
- ✅ Метро (subway)
- ✅ Трамваи (tramway)
- ❌ Пригородный автобус (interurban)

## API Endpoints

Провайдер использует следующие Яндекс API endpoints:

1. **Ближайшие станции**: `/v3.0/nearest_stations/`
   - Параметры: `lat`, `lon`, `distance`, `apikey`, `format`
   - Возвращает: список станций с расстояниями

2. **Информация о станции**: `/v3.0/station/{code}/`
   - Параметры: `apikey`, `format`
   - Возвращает: детальную информацию о станции

3. **Поиск маршрутов**: `/v3.0/search/`
   - Параметры: `from`, `to`, `date`, `apikey`, `format`
   - Возвращает: список доступных маршрутов

## Тестирование

Запуск всех тестов Yandex провайдера:
```bash
go test ./internal/providers/transport -v -run TestYandex
```

Запуск интеграционного теста с реальным API:
```bash
export YANDEX_API_KEY=c7e69f5a-84d0-496f-b1ec-38c184f1b26e
go test ./internal/providers/transport -v -run TestYandexIntegration
```

## Структура ответов

### Station (Станция)
```go
type Station struct {
    ExternalID string
    Name       string
    Latitude   float64
    Longitude  float64
    Type       TransportType
    Provider   string
}
```

### Route (Маршрут)
```go
type Route struct {
    ExternalID        string
    StartStation      *Station
    EndStation        *Station
    DepartureTime     time.Time
    ArrivalTime       time.Time
    TransportType     TransportType
    RouteNumber       string
    IsAvailable       bool
    AvailabilityReason string
    Provider          string
}
```

## Обработка ошибок

Все методы могут вернуть ошибки:
- Сетевые ошибки (timeout, connection refused)
- Ошибки парсинга ответа (invalid JSON)
- Ошибки API (неверный API ключ, квоты исчерпаны)

## Лимиты API

Яндекс Расписание имеет следующие ограничения:
- Максимум 50 запросов в секунду
- Максимум 100 результатов в одном ответе
- Таймаут запроса: 30 секунд

## Пример использования в main()

```go
func main() {
    apiKey := os.Getenv("YANDEX_API_KEY")
    if apiKey == "" {
        log.Fatal("YANDEX_API_KEY not set")
    }
    
    provider := transport.NewYandexStationProvider(apiKey)
    
    ctx := context.Background()
    result, _, err := provider.SearchNearby(ctx, 55.7558, 37.6173, 500)
    if err != nil {
        log.Fatal(err)
    }
    
    for _, station := range result.Stations {
        fmt.Printf("%s: %s\n", station.Name, station.ExternalID)
    }
}
```
