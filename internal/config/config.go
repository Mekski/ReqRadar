// Package config loads service configuration from the environment. Defaults
// match deploy/docker-compose.yml so a bare `go run` works against the dev stack.
package config

import (
	"log/slog"
	"os"
)

type Config struct {
	NATSURL       string
	PostgresDSN   string
	LogLevel      slog.Level
	TelegramToken string
	APIAddr       string
}

func Load() Config {
	return Config{
		NATSURL:       env("REQRADAR_NATS_URL", "nats://localhost:4222"),
		PostgresDSN:   env("REQRADAR_POSTGRES_DSN", "postgres://reqradar:reqradar@localhost:5432/reqradar?sslmode=disable"),
		LogLevel:      parseLevel(env("REQRADAR_LOG_LEVEL", "info")),
		TelegramToken: env("TELEGRAM_BOT_TOKEN", ""),
		APIAddr:       env("REQRADAR_API_ADDR", ":8080"),
	}
}

func parseLevel(s string) slog.Level {
	switch s {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
