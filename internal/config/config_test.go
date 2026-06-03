package config

import (
	"testing"
	"time"
)

func TestLoadRequiresBotToken(t *testing.T) {
	t.Setenv("BOT_TOKEN", "")

	if _, err := Load(); err == nil {
		t.Fatal("expected BOT_TOKEN validation error")
	}
}

func TestLoadOptionalStartupSettings(t *testing.T) {
	t.Setenv("BOT_TOKEN", "token")
	t.Setenv("STARTUP_RETRIES", "3")
	t.Setenv("STARTUP_RETRY_DELAY", "2s")
	t.Setenv("TELEGRAM_CONNECT_TIMEOUT", "4s")
	t.Setenv("TELEGRAM_REQUEST_TIMEOUT", "70s")
	t.Setenv("TELEGRAM_API_ENDPOINT", "http://localhost:8081/bot%s/%s")
	t.Setenv("NETWORK_DIAGNOSTICS", "false")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.StartupRetries != 3 {
		t.Fatalf("unexpected StartupRetries: %d", cfg.StartupRetries)
	}
	if cfg.StartupRetryDelay.String() != "2s" {
		t.Fatalf("unexpected StartupRetryDelay: %s", cfg.StartupRetryDelay)
	}
	if cfg.TelegramConnectTimeout.String() != "4s" {
		t.Fatalf("unexpected TelegramConnectTimeout: %s", cfg.TelegramConnectTimeout)
	}
	if cfg.TelegramRequestTimeout != 70*time.Second {
		t.Fatalf("unexpected TelegramRequestTimeout: %s", cfg.TelegramRequestTimeout)
	}
	if cfg.TelegramAPIEndpoint != "http://localhost:8081/bot%s/%s" {
		t.Fatalf("unexpected TelegramAPIEndpoint: %s", cfg.TelegramAPIEndpoint)
	}
	if cfg.NetworkDiagnostics {
		t.Fatal("expected NetworkDiagnostics to be false")
	}
}
