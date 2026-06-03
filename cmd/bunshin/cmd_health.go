package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func newHealthCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "health",
		Short: "Check server health (exits 0 if healthy, 1 otherwise)",
		Long: `Sends a GET /health request to the running bunshin server and exits 0 on success.

Designed to be used as a Docker HEALTHCHECK command inside the container image
(no curl/wget needed — the binary health-checks itself).

Environment variables:
  BUNSHIN_HEALTH_ADDR   Server base URL (default: http://localhost:8080)`,
		Example: `  bunshin health
  bunshin health --addr http://localhost:9090
  BUNSHIN_HEALTH_ADDR=http://remotehost:8080 bunshin health`,
		RunE: runHealth,
	}
	cmd.Flags().String("addr", "http://localhost:8080", "Server base URL")
	mustBindFlag(cmd, "health_addr", "addr")
	return cmd
}

func runHealth(_ *cobra.Command, _ []string) error {
	addr := viper.GetString("health_addr")
	if addr == "" {
		addr = "http://localhost:8080"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, addr+"/health", nil)
	if err != nil {
		return fmt.Errorf("health: build request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "health check failed: %v\n", err)
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("health: read body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "health check: status %d %s\n", resp.StatusCode, body)
		return fmt.Errorf("unhealthy: status %d", resp.StatusCode)
	}
	fmt.Printf("%s\n", body)
	return nil
}
