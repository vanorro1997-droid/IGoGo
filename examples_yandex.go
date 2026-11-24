package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/vanorro1997-droid/IGoGo/internal/providers/transport"
)

// Example of how to use Yandex provider with the API key
func demonstrateYandexIntegration() {
	apiKey := os.Getenv("YANDEX_API_KEY")
	if value := os.Getenv("YANDEX_RASP_API_KEY"); value != "" {
		apiKey = value
	}
	if apiKey == "" {
		log.Println("YANDEX_RASP_API_KEY environment variable not set")
		return
	}

	// Create Yandex providers
	stationProvider := transport.NewYandexStationProvider(apiKey)
	timetableProvider := transport.NewYandexTimetableProvider(apiKey)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Example 1: Search for nearby stations
	log.Println("\n=== Example 1: Search for nearby stations ===")
	result, metadata, err := stationProvider.SearchNearby(ctx, 55.7558, 37.6173, 500)
	if err != nil {
		log.Printf("Error searching nearby stations: %v\n", err)
	} else {
		log.Printf("Provider: %s, Real data: %v\n", metadata.Source, metadata.IsReal)
		log.Printf("Found %d stations:\n", len(result.Stations))
		for _, station := range result.Stations {
			log.Printf("  - %s (ID: %s, Type: %s)\n", station.Name, station.ExternalID, station.Type)
		}
	}

	// Example 2: Get station by ID
	log.Println("\n=== Example 2: Get station by ID ===")
	if result != nil && len(result.Stations) > 0 {
		stationID := result.Stations[0].ExternalID
		station, metadata, err := stationProvider.GetStationByID(ctx, stationID)
		if err != nil {
			log.Printf("Error getting station: %v\n", err)
		} else {
			log.Printf("Station: %s\n", station.Name)
			log.Printf("Provider: %s\n", metadata.Source)
		}
	}

	// Example 3: Search routes
	log.Println("\n=== Example 3: Search routes ===")
	if result != nil && len(result.Stations) >= 2 {
		from := result.Stations[0]
		to := result.Stations[1]

		searchDate := time.Now().Add(24 * time.Hour)
		earliestDeparture := searchDate.Add(-time.Hour)

		routes, metadata, err := timetableProvider.SearchRoutes(
			ctx,
			from,
			to,
			searchDate,
			earliestDeparture,
			"Europe/Moscow",
		)
		if err != nil {
			log.Printf("Error searching routes: %v\n", err)
		} else {
			log.Printf("Provider: %s, Real data: %v\n", metadata.Source, metadata.IsReal)
			log.Printf("Found %d routes from %s to %s:\n", len(routes.Routes), from.Name, to.Name)
			for _, route := range routes.Routes {
				status := "available"
				if !route.IsAvailable {
					status = "unavailable"
				}
				log.Printf("  - Route %s (%s): %s -> %s [%s]\n",
					route.RouteNumber,
					route.TransportType,
					route.DepartureTime.Format("15:04"),
					route.ArrivalTime.Format("15:04"),
					status,
				)
			}
		}
	}

	// Example 4: Check transport support
	log.Println("\n=== Example 4: Check transport support ===")
	transportTypes := []transport.TransportType{
		transport.TransportTypeTrain,
		transport.TransportTypeBus,
		transport.TransportTypeSubway,
		transport.TransportTypeTramway,
		transport.TransportTypeInterurban,
	}

	for _, t := range transportTypes {
		supported := timetableProvider.SupportsTransport(t, "Russia")
		log.Printf("Transport type %s supported: %v\n", t, supported)
	}
}
