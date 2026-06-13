// Package service holds the small amount of bootstrap wiring shared by every
// service binary: structured logging and graceful shutdown. Keeping it here
// means each cmd/<svc>/main.go stays a thin, readable entrypoint.
package service

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/Mekski/reqradar/internal/config"
)

// Bootstrap loads config, builds a JSON logger tagged with the service name,
// and returns a context cancelled on SIGINT/SIGTERM. Callers defer stop().
func Bootstrap(name string) (context.Context, config.Config, *slog.Logger, context.CancelFunc) {
	cfg := config.Load()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel})).
		With("service", name)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	return ctx, cfg, logger, stop
}
