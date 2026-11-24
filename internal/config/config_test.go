package config

import (
	"os"
	"testing"
)

func isolateEnvironment(t *testing.T) {
	t.Helper()
	t.Chdir(t.TempDir())
	for _, key := range []string{
		"PORT", "FRONTEND_DIR", "WEATHER_PROVIDER", "LLM_ENABLED",
		"CLOUDFLARE_API_TOKEN", "CLOUDFLARE_ACCOUNT_ID", "CLOUDFLARE_MODEL",
		"CLOUDFLARE_PROXY", "CLOUDFLARE_SYSTEM_PROMPT", "HTTPS_PROXY",
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
	if cfg.Port != "8080" || cfg.FrontendDir != "frontend" || cfg.WeatherProvider != "mock" || cfg.LLMEnabled || cfg.DatabaseEnabled {
		t.Fatalf("unexpected defaults: port=%s, frontend=%s, weather=%s, llm=%t",
			cfg.Port, cfg.FrontendDir, cfg.WeatherProvider, cfg.LLMEnabled)
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
		{"WEATHER_PROVIDER", "unknown"},
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

func TestLoadRejectsMalformedDotEnv(t *testing.T) {
	isolateEnvironment(t)
	if err := os.WriteFile(".env", []byte("PORT=\"unterminated\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(); err == nil {
		t.Fatal("expected malformed .env to be rejected")
	}
}
