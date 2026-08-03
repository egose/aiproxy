package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/dashboard"
	"github.com/egose/aiproxy/internal/dashrpc"
	"github.com/spf13/cobra"
)

const dashPollInterval = 2 * time.Second

func newDashboardCommand() *cobra.Command {
	var cfgPath string
	cmd := &cobra.Command{
		Use:   "dashboard",
		Short: "Attach an interactive dashboard to a running aiproxy server",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDashboard(cmd.Context(), cfgPath, cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
	cmd.Flags().StringVarP(&cfgPath, "config", "c", defaultConfigPath(), "path to config file")
	return cmd
}

func runDashboard(parentCtx context.Context, cfgPath string, stdout, stderr io.Writer) error {
	rt, err := config.LoadFile(cfgPath)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	if !rt.Dashboard.Enabled {
		fmt.Fprintln(stderr, "dashboard not configured: add a 'dashboard' block (with or without a token) to your config")
		return errors.New("dashboard not configured")
	}
	token := rt.Dashboard.Token
	if token == "" {
		token, err = dashrpc.LoadToken()
		if err != nil {
			fmt.Fprintln(stderr, "dashboard token not declared in config and no persisted token found — start `aiproxy serve` first, or declare token = \"...\" in the dashboard block")
			return fmt.Errorf("dashboard token: %w", err)
		}
	}

	baseURL := normalizeBaseURL(rt.Listener.Address)
	httpClient := &http.Client{Timeout: 2 * time.Second}

	ctx, stop := signal.NotifyContext(parentCtx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	initial, err := fetchSnapshot(ctx, httpClient, baseURL, token)
	if err != nil {
		if isConnectionRefused(err) {
			fmt.Fprintln(stderr, "no server running")
			return errors.New("no server running")
		}
		if errors.Is(err, errDashboardUnconfigured) {
			fmt.Fprintln(stderr, "dashboard not configured on server: add a 'dashboard' block (with or without a token) to your config and restart")
			return err
		}
		if errors.Is(err, errDashboardUnauthorized) {
			fmt.Fprintln(stderr, "unauthorized: dashboard token mismatch — check the token declared in config vs. the one the server is using")
			return err
		}
		return fmt.Errorf("fetch snapshot: %w", err)
	}

	snap := dashboard.SnapshotFromTransport(initial)
	prog := dashboard.Run(ctx, snap)
	defer prog.Close()

	ticker := time.NewTicker(dashPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			updated, err := fetchSnapshot(ctx, httpClient, baseURL, rt.Dashboard.Token)
			if err != nil {
				if errors.Is(err, errDashboardUnconfigured) || isConnectionRefused(err) {
					fmt.Fprintln(stderr, "server unreachable; dashboard detached")
					return err
				}
				continue
			}
			prog.Refresh(dashboard.SnapshotFromTransport(updated))
		}
	}
}

func normalizeBaseURL(addr string) string {
	if addr == "" {
		addr = ":8080"
	}
	if strings.HasPrefix(addr, ":") {
		return "http://127.0.0.1" + addr
	}
	if strings.HasPrefix(addr, "0.0.0.0:") {
		return "http://127.0.0.1:" + strings.TrimPrefix(addr, "0.0.0.0:")
	}
	if !strings.HasPrefix(addr, "http://") && !strings.HasPrefix(addr, "https://") {
		return "http://" + addr
	}
	return addr
}

var errDashboardUnconfigured = errors.New("dashboard not configured")
var errDashboardUnauthorized = errors.New("dashboard token mismatch")

func fetchSnapshot(ctx context.Context, c *http.Client, baseURL, token string) (dashrpc.Snapshot, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+dashrpc.SnapshotPath, nil)
	if err != nil {
		return dashrpc.Snapshot{}, err
	}
	req.Header.Set(dashrpc.AuthHeaderName, dashrpc.AuthScheme+token)
	resp, err := c.Do(req)
	if err != nil {
		return dashrpc.Snapshot{}, fmt.Errorf("%w: %v", errTransport, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return dashrpc.Snapshot{}, errDashboardUnconfigured
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return dashrpc.Snapshot{}, errDashboardUnauthorized
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return dashrpc.Snapshot{}, fmt.Errorf("snapshot endpoint returned %d: %s", resp.StatusCode, string(body))
	}
	var out dashrpc.Snapshot
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return dashrpc.Snapshot{}, fmt.Errorf("decode snapshot: %w", err)
	}
	return out, nil
}

// errTransport wraps any error returned from c.Do (since at that point we did
// not receive an HTTP response). The dashboard command treats this as "server
// unreachable" and prints "no server running".
var errTransport = errors.New("transport: server unreachable")

func isConnectionRefused(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, errTransport) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "no such host") ||
		strings.Contains(msg, "connect: cannot assign requested address") ||
		strings.Contains(msg, "i/o timeout")
}
