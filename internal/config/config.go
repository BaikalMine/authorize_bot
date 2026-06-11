package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	BotToken                   string
	TelegramAPIEndpoint        string
	TelegramIPFamily           string
	TelegramConnectTimeout     time.Duration
	TelegramRequestTimeout     time.Duration
	CaptchaTimeout             time.Duration
	CaptchaMaxAttempts         int
	MaxActiveChallenges        int
	MaxActiveChallengesPerChat int
	CleanupBatchSize           int
	ProbationEnabled           bool
	ProbationDuration          time.Duration
	ProbationBlockLinks        bool
	ProbationBlockForwards     bool
	ProbationBlockMedia        bool
	SpamGuardEnabled           bool
	SpamGuardKick              bool
	PollingTimeout             int
	StartupRetries             int
	StartupRetryDelay          time.Duration
	NetworkDiagnostics         bool
	KickOnTimeout              bool
	LogLevel                   string
}

func Load() (Config, error) {
	cfg := Config{
		BotToken:                   os.Getenv("BOT_TOKEN"),
		TelegramAPIEndpoint:        os.Getenv("TELEGRAM_API_ENDPOINT"),
		TelegramIPFamily:           envOrDefault("TELEGRAM_IP_FAMILY", "tcp6"),
		TelegramConnectTimeout:     10 * time.Second,
		CaptchaTimeout:             120 * time.Second,
		CaptchaMaxAttempts:         3,
		MaxActiveChallenges:        1000,
		MaxActiveChallengesPerChat: 200,
		CleanupBatchSize:           100,
		ProbationEnabled:           true,
		ProbationDuration:          24 * time.Hour,
		ProbationBlockLinks:        true,
		ProbationBlockForwards:     true,
		ProbationBlockMedia:        true,
		SpamGuardEnabled:           true,
		SpamGuardKick:              true,
		PollingTimeout:             60,
		StartupRetries:             10,
		StartupRetryDelay:          10 * time.Second,
		NetworkDiagnostics:         true,
		KickOnTimeout:              true,
		LogLevel:                   envOrDefault("LOG_LEVEL", "info"),
	}

	if cfg.BotToken == "" {
		return Config{}, errors.New("BOT_TOKEN is required")
	}
	if cfg.TelegramIPFamily != "tcp" && cfg.TelegramIPFamily != "tcp4" && cfg.TelegramIPFamily != "tcp6" {
		return Config{}, errors.New("TELEGRAM_IP_FAMILY must be tcp, tcp4, or tcp6")
	}
	if cfg.TelegramAPIEndpoint != "" {
		if err := validateTelegramAPIEndpoint(cfg.TelegramAPIEndpoint); err != nil {
			return Config{}, err
		}
	}

	if raw := os.Getenv("CAPTCHA_TIMEOUT"); raw != "" {
		timeout, err := time.ParseDuration(raw)
		if err != nil {
			return Config{}, err
		}
		if timeout <= 0 {
			return Config{}, errors.New("CAPTCHA_TIMEOUT must be positive")
		}
		cfg.CaptchaTimeout = timeout
	}

	if raw := os.Getenv("POLLING_TIMEOUT"); raw != "" {
		timeout, err := strconv.Atoi(raw)
		if err != nil {
			return Config{}, err
		}
		if timeout <= 0 {
			return Config{}, errors.New("POLLING_TIMEOUT must be positive")
		}
		cfg.PollingTimeout = timeout
	}

	cfg.TelegramRequestTimeout = time.Duration(cfg.PollingTimeout+30) * time.Second

	if raw := os.Getenv("TELEGRAM_CONNECT_TIMEOUT"); raw != "" {
		timeout, err := time.ParseDuration(raw)
		if err != nil {
			return Config{}, err
		}
		if timeout <= 0 {
			return Config{}, errors.New("TELEGRAM_CONNECT_TIMEOUT must be positive")
		}
		cfg.TelegramConnectTimeout = timeout
	}

	if raw := os.Getenv("TELEGRAM_REQUEST_TIMEOUT"); raw != "" {
		timeout, err := time.ParseDuration(raw)
		if err != nil {
			return Config{}, err
		}
		if timeout <= 0 {
			return Config{}, errors.New("TELEGRAM_REQUEST_TIMEOUT must be positive")
		}
		cfg.TelegramRequestTimeout = timeout
	}

	if raw := os.Getenv("STARTUP_RETRIES"); raw != "" {
		retries, err := strconv.Atoi(raw)
		if err != nil {
			return Config{}, err
		}
		if retries < 0 {
			return Config{}, errors.New("STARTUP_RETRIES must be non-negative")
		}
		cfg.StartupRetries = retries
	}

	if raw := os.Getenv("STARTUP_RETRY_DELAY"); raw != "" {
		delay, err := time.ParseDuration(raw)
		if err != nil {
			return Config{}, err
		}
		if delay <= 0 {
			return Config{}, errors.New("STARTUP_RETRY_DELAY must be positive")
		}
		cfg.StartupRetryDelay = delay
	}

	if raw := os.Getenv("CAPTCHA_MAX_ATTEMPTS"); raw != "" {
		attempts, err := strconv.Atoi(raw)
		if err != nil {
			return Config{}, err
		}
		if attempts <= 0 {
			return Config{}, errors.New("CAPTCHA_MAX_ATTEMPTS must be positive")
		}
		cfg.CaptchaMaxAttempts = attempts
	}

	if raw := os.Getenv("MAX_ACTIVE_CHALLENGES"); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil {
			return Config{}, err
		}
		if limit <= 0 {
			return Config{}, errors.New("MAX_ACTIVE_CHALLENGES must be positive")
		}
		cfg.MaxActiveChallenges = limit
	}

	if raw := os.Getenv("MAX_ACTIVE_CHALLENGES_PER_CHAT"); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil {
			return Config{}, err
		}
		if limit <= 0 {
			return Config{}, errors.New("MAX_ACTIVE_CHALLENGES_PER_CHAT must be positive")
		}
		cfg.MaxActiveChallengesPerChat = limit
	}

	if raw := os.Getenv("CLEANUP_BATCH_SIZE"); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil {
			return Config{}, err
		}
		if limit <= 0 {
			return Config{}, errors.New("CLEANUP_BATCH_SIZE must be positive")
		}
		cfg.CleanupBatchSize = limit
	}

	if raw := os.Getenv("PROBATION_ENABLED"); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return Config{}, err
		}
		cfg.ProbationEnabled = value
	}

	if raw := os.Getenv("PROBATION_DURATION"); raw != "" {
		duration, err := time.ParseDuration(raw)
		if err != nil {
			return Config{}, err
		}
		if duration <= 0 {
			return Config{}, errors.New("PROBATION_DURATION must be positive")
		}
		cfg.ProbationDuration = duration
	}

	if raw := os.Getenv("PROBATION_BLOCK_LINKS"); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return Config{}, err
		}
		cfg.ProbationBlockLinks = value
	}

	if raw := os.Getenv("PROBATION_BLOCK_FORWARDS"); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return Config{}, err
		}
		cfg.ProbationBlockForwards = value
	}

	if raw := os.Getenv("PROBATION_BLOCK_MEDIA"); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return Config{}, err
		}
		cfg.ProbationBlockMedia = value
	}

	if raw := os.Getenv("SPAM_GUARD_ENABLED"); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return Config{}, err
		}
		cfg.SpamGuardEnabled = value
	}

	if raw := os.Getenv("SPAM_GUARD_KICK"); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return Config{}, err
		}
		cfg.SpamGuardKick = value
	}

	if raw := os.Getenv("NETWORK_DIAGNOSTICS"); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return Config{}, err
		}
		cfg.NetworkDiagnostics = value
	}

	if raw := os.Getenv("KICK_ON_TIMEOUT"); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return Config{}, err
		}
		cfg.KickOnTimeout = value
	}

	return cfg, nil
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func validateTelegramAPIEndpoint(endpoint string) error {
	if strings.Count(endpoint, "%s") != 2 {
		return errors.New("TELEGRAM_API_ENDPOINT must contain exactly two %s placeholders")
	}

	rawURL := fmt.Sprintf(endpoint, "token", "getMe")
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("TELEGRAM_API_ENDPOINT must use http or https")
	}
	if parsed.Hostname() == "" {
		return errors.New("TELEGRAM_API_ENDPOINT must include a host")
	}
	return nil
}
