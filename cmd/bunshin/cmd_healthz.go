package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/spf13/cobra"
)

func newHealthzCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "healthz <addr>",
		Short: "Check health and readiness of a remote bunshin node",
		Args:  cobra.ExactArgs(1),
		Example: `  bunshin healthz http://worker-1:8080
  bunshin healthz https://api.example.com`,
		RunE: runHealthz,
	}
}

func runHealthz(_ *cobra.Command, args []string) error {
	addr := args[0]
	client := &http.Client{Timeout: 10 * time.Second}

	for _, path := range []string{"/healthz", "/readyz"} {
		url := addr + path
		resp, err := client.Get(url)
		if err != nil {
			fmt.Printf("%-10s ERROR: %v\n", path, err)
			continue
		}
		defer resp.Body.Close()
		var body map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&body)
		status := "ok"
		if resp.StatusCode != http.StatusOK {
			status = "degraded"
		}
		fmt.Printf("%-10s HTTP %d  status=%s\n", path, resp.StatusCode, status)
		if checks, ok := body["checks"].(map[string]any); ok {
			for name, result := range checks {
				fmt.Printf("           %-20s %v\n", name, result)
			}
		}
	}
	return nil
}

func newPprofCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pprof <addr>",
		Short: "Fetch pprof profile from a remote bunshin node",
		Args:  cobra.ExactArgs(1),
		Example: `  bunshin pprof http://worker-1:8080
  bunshin pprof http://worker-1:8080 --seconds 30`,
		RunE: runPprof,
	}
	cmd.Flags().Int("seconds", 10, "CPU profile duration in seconds")
	mustBindFlag(cmd, "pprof_seconds", "seconds")
	return cmd
}

func runPprof(_ *cobra.Command, args []string) error {
	addr := args[0]
	seconds := 10

	url := fmt.Sprintf("%s/debug/pprof/profile?seconds=%d", addr, seconds)
	fmt.Printf("Fetching CPU profile from %s (%ds)...\n", addr, seconds)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(seconds+30)*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetch pprof: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("pprof endpoint returned %d", resp.StatusCode)
	}
	fmt.Printf("Profile downloaded (%s). Use 'go tool pprof' to analyse.\n", resp.Header.Get("Content-Type"))
	fmt.Printf("Tip: go tool pprof -http=:8081 <profile-file>\n")
	return nil
}
