// Package config parses environment variables into a typed Config struct.
// Called once from main; the result is passed explicitly — no globals.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	ListenAddr          string
	PublicBaseURL       string
	LogLevel            string
	TrustedProxy        bool
	DatabaseURL         string
	DefaultTTL          time.Duration
	MaxCiphertextBytes  int
	RatePerMin          int
	RateBurst           int
	NotifyWebhookURL    string
	NotifyWebhookTimeout time.Duration
	DevUser             string // empty unless GOGRAB_DEV_USER is set
}

func Load() (Config, error) {
	c := Config{
		ListenAddr:           env("GOGRAB_LISTEN_ADDR", ":8080"),
		PublicBaseURL:        env("GOGRAB_PUBLIC_BASE_URL", "http://localhost:8080"),
		LogLevel:             env("GOGRAB_LOG_LEVEL", "info"),
		DatabaseURL:          env("GOGRAB_DATABASE_URL", ""),
		NotifyWebhookURL:     env("GOGRAB_NOTIFY_WEBHOOK_URL", ""),
		DevUser:              env("GOGRAB_DEV_USER", ""),
	}
	var err error
	if c.TrustedProxy, err = envBool("GOGRAB_TRUSTED_PROXY", true); err != nil {
		return c, err
	}
	hours, err := envInt("GOGRAB_DEFAULT_TTL_HOURS", 72)
	if err != nil {
		return c, err
	}
	c.DefaultTTL = time.Duration(hours) * time.Hour
	if c.MaxCiphertextBytes, err = envInt("GOGRAB_MAX_CIPHERTEXT_BYTES", 65536); err != nil {
		return c, err
	}
	if c.RatePerMin, err = envInt("GOGRAB_RATE_PER_MIN", 20); err != nil {
		return c, err
	}
	if c.RateBurst, err = envInt("GOGRAB_RATE_BURST", 5); err != nil {
		return c, err
	}
	secs, err := envInt("GOGRAB_NOTIFY_WEBHOOK_TIMEOUT_SECONDS", 5)
	if err != nil {
		return c, err
	}
	c.NotifyWebhookTimeout = time.Duration(secs) * time.Second

	if c.DatabaseURL == "" {
		return c, fmt.Errorf("GOGRAB_DATABASE_URL is required")
	}
	return c, nil
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func envInt(k string, def int) (int, error) {
	v := os.Getenv(k)
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("env %s: %w", k, err)
	}
	return n, nil
}

func envBool(k string, def bool) (bool, error) {
	v := os.Getenv(k)
	if v == "" {
		return def, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, fmt.Errorf("env %s: %w", k, err)
	}
	return b, nil
}
