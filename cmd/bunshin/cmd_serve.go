package main

import (
	"context"
	"errors"
	"net/http"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/sync/errgroup"

	"github.com/stlimtat/bunshin-go/internal/telemetry"
	"github.com/stlimtat/bunshin-go/pkg/core"
	"github.com/stlimtat/bunshin-go/pkg/transport"
)

func newServeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the HTTP workflow server",
		Long: `Start the bunshin-go HTTP workflow server.

Exposes:
  POST /workflows/{id}        synchronous workflow execution
  GET  /workflows/{id}/stream SSE streaming execution
  GET  /health                healthcheck (Docker/LB probe)
  GET  /live                  liveness probe (Kubernetes)
  GET  /ready                 readiness probe (Kubernetes)
  GET  /debug/pprof/*         Go pprof profiling

Environment variables (BUNSHIN_ prefix):
  BUNSHIN_ADDR        Listen address (default: :8080)
  BUNSHIN_LOG_LEVEL   Log level: debug|info|warn|error`,
		Example: `  bunshin serve
  bunshin serve --addr :9090
  BUNSHIN_ADDR=:9090 bunshin serve`,
		RunE: runServe,
	}
	cmd.Flags().String("addr", ":8080", "HTTP listen address")
	mustBindFlag(cmd, "addr", "addr")
	return cmd
}

func runServe(_ *cobra.Command, _ []string) error {
	cfg := loadConfig()
	logger := telemetry.NewLogger(cfg.LogLevel)

	handler := transport.NewMapHandler()
	handler.Register("echo", core.NewRunnableFunc("echo", func(_ context.Context, input any) (any, error) {
		return input, nil
	}))

	addr := cfg.Addr
	if addr == "" {
		addr = viper.GetString("addr")
	}

	srv := transport.NewHTTPTransport(addr).WithLogger(logger)
	logger.Info().Str("addr", addr).Msg("starting bunshin-go server")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		if err := srv.Serve(ctx, handler); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	})

	if err := g.Wait(); err != nil {
		logger.Error().Err(err).Msg("server failed")
		return err
	}
	logger.Info().Msg("server shut down cleanly")
	return nil
}
