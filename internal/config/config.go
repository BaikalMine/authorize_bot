package config

import (
	"errors"
	"os"
	"strconv"
	"time"
)

type Config struct {
	BotToken               string
	TelegramAPIEndpoint    string
	TelegramIPFamily       string
	TelegramConnectTimeout time.Duration
	TelegramRequestTimeout time.Duration
	CaptchaTimeout         time.Duration
	PollingTimeout         int
	StartupRetries         int
	StartupRetryDelay      time.Duration
	NetworkDiagnostics     bool
	KickOnTimeout          bool
	LogLevel               string
}

func Load() (Config, error) {
	cfg := Config{
		BotToken:               os.Getenv("BOT_TOKEN"),
		TelegramAPIEndpoint:    os.Getenv("TELEGRAM_API_ENDPOINT"),
		TelegramIPFamily:       envOrDefault("TELEGRAM_IP_FAMILY", "tcp4"),
		TelegramConnectTimeout: 10 * time.Second,
		CaptchaTimeout:         120 * time.Second,
		PollingTimeout:         60,
		StartupRetries:         10,
		StartupRetryDelay:      10 * time.Second,
		NetworkDiagnostics:     true,
		KickOnTimeout:          true,
		LogLevel:               envOrDefault("LOG_LEVEL", "info"),
	}

	if cfg.BotToken == "" {
		return Config{}, errors.New("BOT_TOKEN is required")
	}
	if cfg.TelegramIPFamily != "tcp" && cfg.TelegramIPFamily != "tcp4" && cfg.TelegramIPFamily != "tcp6" {
		return Config{}, errors.New("TELEGRAM_IP_FAMILY must be tcp, tcp4, or tcp6")
	}

	if raw := os.Getenv("CAPTCHA_TIMEOUT"); raw != "" {
		timeout, err := time.ParseDuration(raw)
		if err != nil {
			return Config{}, err
		}
		cfg.CaptchaTimeout = timeout
	}

	if raw := os.Getenv("POLLING_TIMEOUT"); raw != "" {
		timeout, err := strconv.Atoi(raw)
		if err != nil {
			return Config{}, err
		}
		cfg.PollingTimeout = timeout
	}

	cfg.TelegramRequestTimeout = time.Duration(cfg.PollingTimeout+30) * time.Second

	if raw := os.Getenv("TELEGRAM_CONNECT_TIMEOUT"); raw != "" {
		timeout, err := time.ParseDuration(raw)
		if err != nil {
			return Config{}, err
		}
		cfg.TelegramConnectTimeout = timeout
	}

	if raw := os.Getenv("TELEGRAM_REQUEST_TIMEOUT"); raw != "" {
		timeout, err := time.ParseDuration(raw)
		if err != nil {
			return Config{}, err
		}
		cfg.TelegramRequestTimeout = timeout
	}

	if raw := os.Getenv("STARTUP_RETRIES"); raw != "" {
		retries, err := strconv.Atoi(raw)
		if err != nil {
			return Config{}, err
		}
		cfg.StartupRetries = retries
	}

	if raw := os.Getenv("STARTUP_RETRY_DELAY"); raw != "" {
		delay, err := time.ParseDuration(raw)
		if err != nil {
			return Config{}, err
		}
		cfg.StartupRetryDelay = delay
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
