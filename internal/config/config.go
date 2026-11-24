package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
	"github.com/vanorro1997-droid/IGoGo/internal/providers/llm"
)

type Config struct {
	Port              string
	FrontendDir       string
	WeatherProvider   string
	TransportProvider string
	YandexRaspAPIKey  string
	DatabaseEnabled   bool
	LLMEnabled        bool
	LLM               llm.Config
}

// Load загружает необязательный .env, не переопределяя переменные окружения.
func Load() (Config, error) {
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return Config{}, fmt.Errorf("load .env: %w", err)
	}

	cfg := Config{
		Port:              strings.TrimSpace(envOr("HTTP_PORT", envOr("PORT", "8080"))),
		FrontendDir:       envOr("FRONTEND_DIR", "frontend"),
		WeatherProvider:   strings.ToLower(strings.TrimSpace(envOr("WEATHER_PROVIDER", "mock"))),
		TransportProvider: strings.ToLower(strings.TrimSpace(envOr("TRANSPORT_PROVIDER", "mock"))),
		YandexRaspAPIKey:  strings.TrimSpace(envOr("YANDEX_RASP_API_KEY", os.Getenv("YANDEX_API_KEY"))),
	}

	port, err := strconv.Atoi(cfg.Port)
	if err != nil || port < 1 || port > 65535 {
		return Config{}, fmt.Errorf("HTTP_PORT (or PORT) must be an integer between 1 and 65535")
	}
	if cfg.WeatherProvider != "mock" && cfg.WeatherProvider != "open-meteo" {
		return Config{}, fmt.Errorf("WEATHER_PROVIDER must be mock or open-meteo")
	}
	if cfg.TransportProvider != "mock" && cfg.TransportProvider != "yandex" {
		return Config{}, fmt.Errorf("TRANSPORT_PROVIDER must be mock or yandex")
	}
	if cfg.TransportProvider == "yandex" && cfg.YandexRaspAPIKey == "" {
		return Config{}, fmt.Errorf("YANDEX_RASP_API_KEY is required when TRANSPORT_PROVIDER=yandex")
	}
	cfg.DatabaseEnabled, err = strconv.ParseBool(strings.TrimSpace(envOr("DB_ENABLED", strconv.FormatBool(databaseConfigured()))))
	if err != nil {
		return Config{}, fmt.Errorf("DB_ENABLED must be a boolean")
	}
	cfg.LLMEnabled, err = strconv.ParseBool(strings.TrimSpace(envOr("LLM_ENABLED", "false")))
	if err != nil {
		return Config{}, fmt.Errorf("LLM_ENABLED must be a boolean")
	}

	cfg.LLM = llm.Config{
		APIKey:       os.Getenv("CLOUDFLARE_API_TOKEN"),
		AccountID:    os.Getenv("CLOUDFLARE_ACCOUNT_ID"),
		Model:        os.Getenv("CLOUDFLARE_MODEL"),
		Proxy:        envOr("CLOUDFLARE_PROXY", os.Getenv("HTTPS_PROXY")),
		SystemPrompt: os.Getenv("CLOUDFLARE_SYSTEM_PROMPT"),
	}
	return cfg, nil
}

func databaseConfigured() bool {
	for _, key := range []string{"DB_HOST", "DB_PORT", "DB_USER", "DB_PASSWORD", "DB_NAME"} {
		if strings.TrimSpace(os.Getenv(key)) != "" {
			return true
		}
	}
	return false
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
