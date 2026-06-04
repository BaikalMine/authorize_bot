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
	t.Setenv("TELEGRAM_IP_FAMILY", "tcp")
	t.Setenv("TELEGRAM_CONNECT_TIMEOUT", "4s")
	t.Setenv("TELEGRAM_REQUEST_TIMEOUT", "70s")
	t.Setenv("TELEGRAM_API_ENDPOINT", "http://localhost:8081/bot%s/%s")
	t.Setenv("CAPTCHA_MAX_ATTEMPTS", "2")
	t.Setenv("MAX_ACTIVE_CHALLENGES", "10")
	t.Setenv("MAX_ACTIVE_CHALLENGES_PER_CHAT", "5")
	t.Setenv("CLEANUP_BATCH_SIZE", "3")
	t.Setenv("PROBATION_ENABLED", "true")
	t.Setenv("PROBATION_DURATION", "12h")
	t.Setenv("PROBATION_BLOCK_LINKS", "true")
	t.Setenv("PROBATION_BLOCK_FORWARDS", "false")
	t.Setenv("PROBATION_BLOCK_MEDIA", "false")
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
	if cfg.TelegramIPFamily != "tcp" {
		t.Fatalf("unexpected TelegramIPFamily: %s", cfg.TelegramIPFamily)
	}
	if cfg.CaptchaMaxAttempts != 2 {
		t.Fatalf("unexpected CaptchaMaxAttempts: %d", cfg.CaptchaMaxAttempts)
	}
	if cfg.MaxActiveChallenges != 10 {
		t.Fatalf("unexpected MaxActiveChallenges: %d", cfg.MaxActiveChallenges)
	}
	if cfg.MaxActiveChallengesPerChat != 5 {
		t.Fatalf("unexpected MaxActiveChallengesPerChat: %d", cfg.MaxActiveChallengesPerChat)
	}
	if cfg.CleanupBatchSize != 3 {
		t.Fatalf("unexpected CleanupBatchSize: %d", cfg.CleanupBatchSize)
	}
	if !cfg.ProbationEnabled {
		t.Fatal("expected ProbationEnabled to be true")
	}
	if cfg.ProbationDuration != 12*time.Hour {
		t.Fatalf("unexpected ProbationDuration: %s", cfg.ProbationDuration)
	}
	if !cfg.ProbationBlockLinks {
		t.Fatal("expected ProbationBlockLinks to be true")
	}
	if cfg.ProbationBlockForwards {
		t.Fatal("expected ProbationBlockForwards to be false")
	}
	if cfg.ProbationBlockMedia {
		t.Fatal("expected ProbationBlockMedia to be false")
	}
	if cfg.NetworkDiagnostics {
		t.Fatal("expected NetworkDiagnostics to be false")
	}
}

func TestLoadRejectsInvalidTelegramIPFamily(t *testing.T) {
	t.Setenv("BOT_TOKEN", "token")
	t.Setenv("TELEGRAM_IP_FAMILY", "udp")

	if _, err := Load(); err == nil {
		t.Fatal("expected invalid TELEGRAM_IP_FAMILY error")
	}
}

func TestLoadRejectsNonPositiveDurationsAndLimits(t *testing.T) {
	tests := map[string]string{
		"CAPTCHA_TIMEOUT":                "0s",
		"POLLING_TIMEOUT":                "0",
		"TELEGRAM_CONNECT_TIMEOUT":       "0s",
		"TELEGRAM_REQUEST_TIMEOUT":       "0s",
		"STARTUP_RETRIES":                "-1",
		"STARTUP_RETRY_DELAY":            "0s",
		"CAPTCHA_MAX_ATTEMPTS":           "0",
		"MAX_ACTIVE_CHALLENGES":          "0",
		"MAX_ACTIVE_CHALLENGES_PER_CHAT": "0",
		"CLEANUP_BATCH_SIZE":             "0",
		"PROBATION_DURATION":             "0s",
	}

	for key, value := range tests {
		t.Run(key, func(t *testing.T) {
			t.Setenv("BOT_TOKEN", "token")
			t.Setenv(key, value)

			if _, err := Load(); err == nil {
				t.Fatalf("expected %s=%s to be rejected", key, value)
			}
		})
	}
}

func TestLoadRejectsMalformedTelegramAPIEndpoint(t *testing.T) {
	t.Setenv("BOT_TOKEN", "token")
	t.Setenv("TELEGRAM_API_ENDPOINT", "not-a-url")

	if _, err := Load(); err == nil {
		t.Fatal("expected malformed TELEGRAM_API_ENDPOINT to be rejected")
	}
}
