// Command bunshin is the CLI entry point for the bunshin-go workflow server.
//
// Usage:
//
//	bunshin serve [--addr :8080]   Start the HTTP workflow server
//	bunshin version                Print version information
//
// Environment variables:
//
//	BUNSHIN_ADDR        HTTP listen address (default: :8080)
//	BUNSHIN_LOG_LEVEL   Log level: debug|info|warn|error (default: info)
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/stlimtat/bunshin-go/pkg/core"
	"github.com/stlimtat/bunshin-go/pkg/transport"
)

const version = "0.1.0"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "serve":
		runServe()
	case "version":
		fmt.Printf("bunshin-go %s\n", version)
	default:
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, "Usage: bunshin <serve|version>\n")
}

func runServe() {
	addr := os.Getenv("BUNSHIN_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	// Register built-in workflows.
	handler := transport.NewMapHandler()
	handler.Register("echo", core.NewRunnableFunc("echo", func(_ context.Context, input any) (any, error) {
		return input, nil
	}))

	srv := transport.NewHTTPTransport(addr)
	logger.Info("starting bunshin-go server", slog.String("addr", addr))

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := srv.Serve(ctx, handler); err != nil {
		logger.Error("server stopped", slog.String("error", err.Error()))
	}
}
