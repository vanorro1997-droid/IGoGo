package config

import (
	"os"
	"path/filepath"
	"testing"
)

func isolateEnvironment(t *testing.T) {
	t.Helper()
	t.Chdir(t.TempDir())
	for _, key := range []string{
		"HTTP_PORT", "PORT", "FRONTEND_DIR", "WEATHER_PROVIDER", "LLM_ENABLED",
		"TRANSPORT_PROVIDER", "YANDEX_RASP_API_KEY", "YANDEX_API_KEY",
		"CLOUDFLARE_API_TOKEN", "CLOUDFLARE_ACCOUNT_ID", "CLOUDFLARE_MODEL",
		"CLOUDFLARE_PROXY", "CLOUDFLARE_SYSTEM_PROMPT", "HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY",
		"WEATHERAPI_KEY", "TWOGIS_API_KEY", "SESSION_TTL_HOURS",
		"DB_ENABLED", "DB_HOST", "DB_PORT", "DB_USER", "DB_PASSWORD", "DB_NAME", "DB_SSLMODE",
	} {
		t.Setenv(key, "")
		if err := os.Unsetenv(key); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLoadDefaults(t *testing.T) {
	isolateEnvironment(t)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != "8080" || cfg.FrontendDir != "frontend" || cfg.WeatherProvider != "mock" ||
		cfg.TransportProvider != "mock" || cfg.YandexRaspAPIKey != "" || cfg.LLMEnabled || cfg.DatabaseEnabled {
		t.Fatalf("unexpected defaults: port=%s, frontend=%s, weather=%s, transport=%s, llm=%t",
			cfg.Port, cfg.FrontendDir, cfg.WeatherProvider, cfg.TransportProvider, cfg.LLMEnabled)
	}
}

func TestLoadEnvironment(t *testing.T) {
	isolateEnvironment(t)
	t.Setenv("PORT", "9000")
	t.Setenv("FRONTEND_DIR", "static")
	t.Setenv("WEATHER_PROVIDER", "open-meteo")
	t.Setenv("LLM_ENABLED", "true")
	t.Setenv("CLOUDFLARE_API_TOKEN", "test-token")
	t.Setenv("CLOUDFLARE_ACCOUNT_ID", "test-account")
	t.Setenv("CLOUDFLARE_MODEL", "test-model")
	t.Setenv("CLOUDFLARE_SYSTEM_PROMPT", "test system prompt")
	t.Setenv("HTTPS_PROXY", "http://localhost:9001")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != "9000" || cfg.FrontendDir != "static" || cfg.WeatherProvider != "open-meteo" || !cfg.LLMEnabled {
		t.Fatal("environment settings were not loaded")
	}
	if cfg.LLM.APIKey != "test-token" || cfg.LLM.AccountID != "test-account" || cfg.LLM.Model != "test-model" ||
		cfg.LLM.SystemPrompt != "test system prompt" || cfg.LLM.Proxy != "http://localhost:9001" {
		t.Fatal("LLM settings were not loaded")
	}
	t.Setenv("CLOUDFLARE_PROXY", "http://localhost:9002")
	cfg, err = Load()
	if err != nil || cfg.LLM.Proxy != "http://localhost:9002" {
		t.Fatal("explicit Cloudflare proxy must take precedence")
	}
}

func TestLoadRejectsInvalidSettings(t *testing.T) {
	for _, test := range []struct {
		key, value string
	}{
		{"PORT", "not-a-port"},
		{"PORT", "0"},
		{"PORT", "65536"},
		{"HTTP_PORT", "not-a-port"},
		{"HTTP_PORT", "0"},
		{"HTTP_PORT", "65536"},
		{"WEATHER_PROVIDER", "unknown"},
		{"TRANSPORT_PROVIDER", "unknown"},
		{"TRANSPORT_PROVIDER", "yandex"},
		{"LLM_ENABLED", "sometimes"},
		{"DB_ENABLED", "sometimes"},
	} {
		t.Run(test.key+"="+test.value, func(t *testing.T) {
			isolateEnvironment(t)
			t.Setenv(test.key, test.value)
			if _, err := Load(); err == nil {
				t.Fatal("expected invalid configuration to be rejected")
			}
		})
	}
}

func TestLoadEnvironmentExample(t *testing.T) {
	examplePath, err := filepath.Abs("../../.env.example")
	if err != nil {
		t.Fatal(err)
	}
	example, err := os.ReadFile(examplePath)
	if err != nil {
		t.Fatal(err)
	}
	isolateEnvironment(t)
	if err := os.WriteFile(".env", example, 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatalf(".env.example must be a valid configuration: %v", err)
	}
	if cfg.Port != "8080" || cfg.TransportProvider != "mock" || cfg.WeatherProvider != "mock" ||
		cfg.YandexRaspAPIKey != "" || cfg.LLMEnabled || cfg.DatabaseEnabled {
		t.Fatal(".env.example must allow startup without external credentials or a database")
	}
}

func TestLoadDescriptiveEnvironmentNamesAndLegacyAliases(t *testing.T) {
	for _, test := range []struct {
		name, httpPort, port, raspKey, legacyKey string
		wantPort, wantKey                        string
	}{
		{"descriptive names", "9100", "", "rasp-test-key", "", "9100", "rasp-test-key"},
		{"legacy aliases", "", "9200", "", "legacy-test-key", "9200", "legacy-test-key"},
		{"descriptive names take precedence", "9300", "9400", "rasp-test-key", "legacy-test-key", "9300", "rasp-test-key"},
	} {
		t.Run(test.name, func(t *testing.T) {
			isolateEnvironment(t)
			t.Setenv("HTTP_PORT", test.httpPort)
			t.Setenv("PORT", test.port)
			t.Setenv("TRANSPORT_PROVIDER", " Yandex ")
			t.Setenv("YANDEX_RASP_API_KEY", test.raspKey)
			t.Setenv("YANDEX_API_KEY", test.legacyKey)
			cfg, err := Load()
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Port != test.wantPort || cfg.TransportProvider != "yandex" || cfg.YandexRaspAPIKey != test.wantKey {
				t.Fatal("transport settings or environment aliases were not loaded correctly")
			}
		})
	}
}

func TestLoadYandexCredentialsAreOptionalForMock(t *testing.T) {
	isolateEnvironment(t)
	t.Setenv("YANDEX_RASP_API_KEY", "unused-test-key")
	cfg, err := Load()
	if err != nil || cfg.TransportProvider != "mock" {
		t.Fatal("an API key alone must not replace the default mock provider")
	}
	t.Setenv("TRANSPORT_PROVIDER", "yandex")
	t.Setenv("YANDEX_RASP_API_KEY", "   ")
	if _, err := Load(); err == nil {
		t.Fatal("Yandex must reject an empty API key")
	}
}

func TestLoadDatabaseConfiguration(t *testing.T) {
	for _, test := range []struct {
		name, host, enabled string
		want                bool
	}{
		{"configured database", "localhost", "", true},
		{"explicitly enabled", "", "true", true},
		{"explicitly disabled", "localhost", "false", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			isolateEnvironment(t)
			t.Setenv("DB_HOST", test.host)
			t.Setenv("DB_ENABLED", test.enabled)
			cfg, err := Load()
			if err != nil || cfg.DatabaseEnabled != test.want {
				t.Fatalf("DatabaseEnabled = %t, want %t; error = %v", cfg.DatabaseEnabled, test.want, err)
			}
		})
	}
}

func TestLoadDotEnvWithoutOverridingEnvironment(t *testing.T) {
	isolateEnvironment(t)
	if err := os.WriteFile(".env", []byte("PORT=9000\nWEATHER_PROVIDER=open-meteo\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PORT", "9100")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != "9100" || cfg.WeatherProvider != "open-meteo" {
		t.Fatal(".env must be loaded without overriding existing environment variables")
	}
}

func TestLoadYandexSettingsFromDotEnv(t *testing.T) {
	isolateEnvironment(t)
	if err := os.WriteFile(".env", []byte("HTTP_PORT=9200\nTRANSPORT_PROVIDER=yandex\nYANDEX_RASP_API_KEY=dotenv-test-key\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("YANDEX_RASP_API_KEY", "environment-test-key")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != "9200" || cfg.TransportProvider != "yandex" || cfg.YandexRaspAPIKey != "environment-test-key" {
		t.Fatal("Yandex settings must be loaded without overriding the environment")
	}
}

func TestLoadRejectsMalformedDotEnv(t *testing.T) {
	isolateEnvironment(t)
	if err := os.WriteFile(".env", []byte("PORT=\"unterminated\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(); err == nil {
		t.Fatal("expected malformed .env to be rejected")
	}
}
