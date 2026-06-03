package config

import (
	"errors"
	"os"
	"strconv"
	"time"
)

type Config struct {
	BotToken            string
	TelegramAPIEndpoint string
	CaptchaTimeout      time.Duration
	PollingTimeout      int
	StartupRetries      int
	StartupRetryDelay   time.Duration
	KickOnTimeout       bool
	LogLevel            string
}

func Load() (Config, error) {
	cfg := Config{
		BotToken:            os.Getenv("BOT_TOKEN"),
		TelegramAPIEndpoint: os.Getenv("TELEGRAM_API_ENDPOINT"),
		CaptchaTimeout:      120 * time.Second,
		PollingTimeout:      60,
		StartupRetries:      10,
		StartupRetryDelay:   10 * time.Second,
		KickOnTimeout:       true,
		LogLevel:            envOrDefault("LOG_LEVEL", "info"),
	}

	if cfg.BotToken == "" {
		return Config{}, errors.New("BOT_TOKEN is required")
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
