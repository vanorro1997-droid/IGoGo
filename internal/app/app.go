package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/vanorro1997-droid/IGoGo/internal/config"
	"github.com/vanorro1997-droid/IGoGo/internal/httpapi"
	"github.com/vanorro1997-droid/IGoGo/internal/providers/llm"
	"github.com/vanorro1997-droid/IGoGo/internal/providers/transport"
	"github.com/vanorro1997-droid/IGoGo/internal/providers/weather"
)

// NewHandler связывает существующие провайдеры с HTTP API.
func NewHandler(cfg config.Config) (http.Handler, error) {
	var weatherProvider weather.Provider
	switch cfg.WeatherProvider {
	case "mock":
		weatherProvider = weather.NewMockProvider()
	case "open-meteo":
		weatherProvider = weather.NewOpenMeteo()
	default:
		return nil, fmt.Errorf("unsupported weather provider: %s", cfg.WeatherProvider)
	}

	deps := httpapi.Dependencies{
		Weather:   weatherProvider,
		Stations:  transport.NewMockStationProvider(),
		Timetable: transport.NewMockTimetableProvider(),
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
	handler, err := NewHandler(cfg)
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
	log.Printf("Server starting on port %s (weather=%s, transport=mock, llm_enabled=%t)",
		cfg.Port, cfg.WeatherProvider, cfg.LLMEnabled)

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
