package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/vanorro1997-droid/IGoGo/internal/config"
	"github.com/vanorro1997-droid/IGoGo/internal/httpapi"
	"github.com/vanorro1997-droid/IGoGo/internal/providers/llm"
	"github.com/vanorro1997-droid/IGoGo/internal/providers/transport"
	"github.com/vanorro1997-droid/IGoGo/internal/providers/weather"
	"github.com/vanorro1997-droid/IGoGo/internal/storage"
)

// NewHandler связывает провайдеры с HTTP API без подключения БД.
func NewHandler(cfg config.Config) (http.Handler, error) {
	return newHandler(cfg, nil)
}

func newHandler(cfg config.Config, store *storage.Store) (http.Handler, error) {
	var weatherProvider weather.Provider
	switch cfg.WeatherProvider {
	case "mock":
		weatherProvider = weather.NewMockProvider()
	case "open-meteo":
		weatherProvider = weather.NewOpenMeteo()
	default:
		return nil, fmt.Errorf("unsupported weather provider: %s", cfg.WeatherProvider)
	}

	var stationProvider transport.StationProvider
	var timetableProvider transport.TimetableProvider
	switch cfg.TransportProvider {
	case "", "mock":
		stationProvider = transport.NewMockStationProvider()
		timetableProvider = transport.NewMockTimetableProvider()
	case "yandex":
		apiKey := strings.TrimSpace(cfg.YandexRaspAPIKey)
		if apiKey == "" {
			return nil, fmt.Errorf("YANDEX_RASP_API_KEY is required when TRANSPORT_PROVIDER=yandex")
		}
		stationProvider = transport.NewYandexStationProvider(apiKey)
		timetableProvider = transport.NewYandexTimetableProvider(apiKey)
	default:
		return nil, fmt.Errorf("unsupported transport provider: %s", cfg.TransportProvider)
	}

	deps := httpapi.Dependencies{
		Weather:   weatherProvider,
		Stations:  stationProvider,
		Timetable: timetableProvider,
		Store:     store,
	}
	if cfg.LLMEnabled {
		client, err := llm.New(cfg.LLM)
		if err != nil {
			return nil, fmt.Errorf("initialize llm: %w", err)
		}
		deps.LLM = client
	}

	return httpapi.NewRouter(cfg.FrontendDir, deps), nil
}

func Run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	var store *storage.Store
	if cfg.DatabaseEnabled {
		store, err = storage.OpenStore()
		if err != nil {
			return fmt.Errorf("database: %w", err)
		}
		defer store.Close()
	}
	handler, err := newHandler(cfg, store)
	if err != nil {
		return err
	}

	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      70 * time.Second, // LLM имеет таймаут 60 секунд.
		IdleTimeout:       120 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errorsCh := make(chan error, 1)
	go func() { errorsCh <- server.ListenAndServe() }()
	log.Printf("Server starting on port %s (weather=%s, transport=%s, llm_enabled=%t, database_enabled=%t)",
		cfg.Port, cfg.WeatherProvider, cfg.TransportProvider, cfg.LLMEnabled, cfg.DatabaseEnabled)

	select {
	case err := <-errorsCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			_ = server.Close()
			return err
		}
		return nil
	}
}
