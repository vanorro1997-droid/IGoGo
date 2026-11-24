# Transport Providers Documentation

## Overview

This package contains interfaces and implementations for transport data providers in the IGoGo project. Transport providers supply:
- **Stations**: locations of train stations, bus stops, and other transport hubs
- **Timetables**: schedules of direct routes between stations

## Interfaces

### StationProvider

Finds and retrieves station information.

```go
type StationProvider interface {
    SearchNearby(ctx context.Context, lat, lon float64, radiusM int) (*StationSearchResult, *ResponseMetadata, error)
    GetStationByID(ctx context.Context, externalID string) (*Station, *ResponseMetadata, error)
    Name() string
}
```

**Methods:**
- `SearchNearby`: Returns stations within `radiusM` meters of the given coordinates
- `GetStationByID`: Retrieves a specific station by its provider-specific ID
- `Name`: Returns the provider name (e.g., "yandex", "openstreetmap", "mock")

### TimetableProvider

Searches for direct routes between stations.

```go
type TimetableProvider interface {
    SearchRoutes(
        ctx context.Context,
        from *Station,
        to *Station,
        date time.Time,
        earliestDeparture time.Time,
        tz string,
    ) (*RoutesSearchResult, *ResponseMetadata, error)
    SupportsTransport(transportType TransportType, region string) bool
    Name() string
}
```

**Methods:**
- `SearchRoutes`: Returns available routes between two stations on a specific date, departing at or after `earliestDeparture`
- `SupportsTransport`: Checks if this provider covers the transport type and region
- `Name`: Returns the provider name

## Data Models

### Station

Represents a transport hub with:
- `ExternalID`: stable identifier from the provider
- `Name`: human-readable station name
- `Latitude`, `Longitude`: geographic coordinates (WGS84)
- `Type`: transport type (train, bus, tramway, subway, interurban)
- `Provider`: name of the data source

### Route

Represents a direct journey with:
- `ExternalID`: unique route identifier from provider
- `StartStation`, `EndStation`: stations
- `DepartureTime`, `ArrivalTime`: times in UTC
- `TransportType`: type of transport
- `RouteNumber`: human-readable route ID (e.g., "120", "45")
- `IsAvailable`: whether the route can be booked
- `AvailabilityReason`: explanation if unavailable
- `Provider`: name of the data source

### ResponseMetadata

Accompanies every response:
- `Source`: provider name
- `Timestamp`: when data was fetched
- `IsReal`: true for real data, false for mock/fixture data

## Mock Provider

The mock provider (`NewMockStationProvider`, `NewMockTimetableProvider`) supplies test data for development and testing without external API dependencies.

**Features:**
- Returns predictable stations and routes
- Supports testing of complete workflows
- Clearly marked as non-real data

**Usage:**
```go
stationProvider := NewMockStationProvider()
timetableProvider := NewMockTimetableProvider()

// Use in trip planning without external dependencies
```

**Test data includes:**
- 4 stations (trains and buses)
- Sample direct routes with various transport types
- Routes at different times
- Timezone support

## Implementing Real Providers

When integrating a real transport data source:

1. **Create a new file** (e.g., `yandex.go`, `gtfs_provider.go`)
2. **Implement both interfaces** or one if the provider only supplies stations or timetables
3. **Handle errors gracefully**:
   - Return `RoutesSearchResult{Error: "reason"}` instead of failing when specific routes are unavailable
   - Distinguish between "no routes found" and "provider unavailable"
4. **Respect rate limits and caching rules** of the provider
5. **Store provider-specific configuration** (API keys, base URLs) in a dedicated struct
6. **Add tests** with fixtures for known control points

### Example: Minimal Real Provider

```go
type RealProvider struct {
    apiKey  string
    baseURL string
    client  *http.Client
}

func (p *RealProvider) SearchNearby(ctx context.Context, lat, lon float64, radiusM int) (*StationSearchResult, *ResponseMetadata, error) {
    // 1. Validate inputs
    // 2. Make API call with context and timeout
    // 3. Parse response
    // 4. Normalize station data to Station structs
    // 5. Return result with ResponseMetadata{IsReal: true}
}

func (p *RealProvider) Name() string {
    return "provider_name"
}
```

## Design Principles

### Abstraction

Providers are pluggable through interfaces. The trip planner doesn't care whether data comes from Yandex, GTFS, or a mock—it uses the same methods.

### Data Normalization

Each provider returns data normalized to common structures (`Station`, `Route`). The trip planner doesn't know provider-specific formats.

### Error Handling

- HTTP errors, timeouts, and invalid responses return errors
- Missing data (no routes for a query) returns empty results, not errors
- Provider failures are clearly communicated through `ResponseMetadata`

### Real vs. Mock

Every response includes `ResponseMetadata.IsReal`:
- `true` = data from a real provider
- `false` = mock or fixture data

The trip planner may warn users if the plan was calculated entirely on mock data.

### Timezone Support

- All times are stored and returned in UTC
- The IANA timezone (e.g., `"Europe/Moscow"`) is passed separately
- Providers and consumers must handle timezone conversions correctly

## Integration Checklist for New Providers

- [ ] Interfaces fully implemented
- [ ] Error cases handled (no data, provider down, invalid response)
- [ ] API keys stored in environment, not in code
- [ ] Requests use context and timeouts
- [ ] Data normalized to standard structs
- [ ] `ResponseMetadata` set correctly, especially `IsReal`
- [ ] Timezone handling tested
- [ ] Rate limits respected
- [ ] Fixtures created for testing (e.g., test routes)
- [ ] Unit tests pass without live API access
- [ ] Documentation notes coverage area, supported transport types, and costs
