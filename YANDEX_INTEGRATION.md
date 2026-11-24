# Интеграция Яндекс Расписания в IGoGo

## API Ключ

Получите собственный ключ на https://yandex.ru/dev/rasp/ и храните его только
в локальном `.env` или переменных окружения. Полный список настроек — в `.env.example`.

## Использование

### 1. Установка API ключа в окружение

```bash
export TRANSPORT_PROVIDER=yandex
export YANDEX_RASP_API_KEY=your_yandex_rasp_api_key
```

Или в `.env` файле для локальной разработки:
```
TRANSPORT_PROVIDER=yandex
YANDEX_RASP_API_KEY=your_yandex_rasp_api_key
```

Запустите `go run .` или `go run ./cmd/api`. Провайдер подключён к существующим
`GET /api/transport/stations` и `GET /api/transport/routes`; отдельный сервер не нужен.
По умолчанию `TRANSPORT_PROVIDER=mock`, ключ не требуется. Прежнее имя
`YANDEX_API_KEY` поддерживается, но `YANDEX_RASP_API_KEY` имеет приоритет.
`HTTP_PORT` задаёт порт HTTP-сервера; прежнее имя `PORT` также поддерживается.

В Docker Compose эти настройки передаются приложению из `.env` или окружения.
Для БД, авторизации и истории установите `DB_ENABLED=true`; с `false` API работает без БД.
Учётные данные `DB_USER`, `DB_PASSWORD`, `DB_NAME` используются обоими контейнерами.
При уже существующем томе PostgreSQL изменение пароля в `.env` само по себе
не меняет пароль пользователя БД. Не удаляйте том с данными ради смены настроек.

Примеры HTTP-запросов (при включённой БД нужен токен авторизации):

```text
GET /api/transport/stations?lat=55.7558&lon=37.6173&radius_m=10000
GET /api/transport/routes?from_id=s2006004&to_id=s9602494&date=2026-11-25&timezone=Europe/Moscow
```

Используйте `external_id` станций из первого ответа. Метаданные Яндекса содержат
`source: "yandex"` и `is_real: true`; ошибки провайдера не подменяются mock-данными.

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
- ✅ Поезда и электрички (train; Яндекс `suburban` приводится к `train`)
- ✅ Междугородние и пригородные автобусы (bus / interurban)
- ❌ Городские метро (subway) и трамваи (tramway)

API Расписаний не является источником расписаний городского транспорта.

## API Endpoints

Провайдер использует https://api.rasp.yandex-net.ru и следующие Яндекс API endpoints.
Ключ передаётся в заголовке `Authorization`, а не в URL.

1. **Ближайшие станции**: `/v3.0/nearest_stations/`
   - Параметры: `lat`, `lng`, `distance` (километры), `transport_types`, `limit`, `format`
   - Возвращает: список станций с расстояниями

2. **Информация о станции**: `/v3.0/schedule/`
   - Параметры: `station`, `limit`, `format`
   - Возвращает: объект `station` внутри расписания. Координаты в этом ответе
     могут отсутствовать; координаты станций доступны в поиске ближайших станций.

3. **Поиск маршрутов**: `/v3.0/search/`
   - Параметры: `from`, `to`, `date`, `result_timezone`, `transport_types`, `transfers`, `format`
   - Возвращает: список доступных маршрутов

## Тестирование

Запуск всех тестов Yandex провайдера:
```bash
go test ./internal/providers/transport -v -run TestYandex
```

Запуск интеграционного теста с реальным API:
```bash
export YANDEX_RASP_API_KEY=your_yandex_rasp_api_key
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

- Поиск ближайших станций ограничен радиусом 50 км и 50 станциями.
- Поиск маршрутов возвращает первую страницу (до 100 прямых рейсов).
- Интервальные рейсы без точного времени отправления не преобразуются в `Route`.
- API не сообщает число свободных мест: `IsAvailable` означает наличие рейса
  в расписании, а не возможность забронировать билет.
- Таймаут запроса: 30 секунд; квоты зависят от условий выданного ключа.
- Времена нормализуются в UTC с учётом часового пояса ответа.

Официальная документация: https://yandex.ru/dev/rasp/doc/ru/ . При публикации данных
указывайте Яндекс Расписания как источник согласно условиям API.

Обычные тесты используют HTTP-фикстуры и не обращаются в сеть. Интеграционные
тесты запускают реальные запросы только при наличии ключа в окружении.

## Пример использования в main()

```go
func main() {
    apiKey := os.Getenv("YANDEX_RASP_API_KEY")
    if apiKey == "" {
        log.Fatal("YANDEX_RASP_API_KEY not set")
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
