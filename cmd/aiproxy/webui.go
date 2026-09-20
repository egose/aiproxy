package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/webui"
	"github.com/spf13/cobra"
)

func newWebUICommand() *cobra.Command {
	var cfgPath string
	var openBrowser bool
	cmd := &cobra.Command{
		Use:   "webui",
		Short: "Print the web dashboard URL of a running aiproxy server",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWebUI(cmd.Context(), cfgPath, configFlagExplicit(cmd), openBrowser, cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
	cmd.Flags().StringVarP(&cfgPath, "config", "c", defaultConfigPath(), "path to config file (overrides $AIPROXY_CONFIG)")
	cmd.Flags().BoolVar(&openBrowser, "open", false, "open the dashboard URL in the default browser")
	return cmd
}

func runWebUI(parentCtx context.Context, cfgPath string, explicit bool, openBrowser bool, stdout, stderr io.Writer) error {
	rt, err := config.LoadFileOrEnv(cfgPath, explicit)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	if !rt.WebUI.Enabled {
		fmt.Fprintln(stderr, "web UI not configured: add a 'web_ui' block to your config")
		return errors.New("web UI not configured")
	}
	pageURL := normalizeBaseURL(rt.Listener.Address) + webui.RoutePrefix + "/"
	httpClient := &http.Client{Timeout: 5 * time.Second}
	ctx, cancel := context.WithTimeout(parentCtx, 5*time.Second)
	defer cancel()
	if err := probeWebUI(ctx, httpClient, pageURL); err != nil {
		fmt.Fprintln(stderr, err.Error())
		return err
	}
	fmt.Fprintln(stdout, pageURL)
	if openBrowser {
		if err := openBrowserURL(ctx, pageURL); err != nil {
			return fmt.Errorf("open browser: %w", err)
		}
	}
	return nil
}

var errWebUINotAvailable = errors.New("web UI not available on server")

func probeWebUI(ctx context.Context, client *http.Client, pageURL string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return fmt.Errorf("web UI request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		if isConnectionRefused(fmt.Errorf("%w: %v", errTransport, err)) {
			return errors.New("no server running")
		}
		return fmt.Errorf("web UI request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("%w: enable a 'web_ui' block and restart (or SIGHUP-reload), or upgrade the server binary", errWebUINotAvailable)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("web UI endpoint returned %d", resp.StatusCode)
	}
	if contentType := resp.Header.Get("Content-Type"); !strings.Contains(contentType, "text/html") {
		return fmt.Errorf("web UI endpoint returned unexpected content type %q", contentType)
	}
	return nil
}

func openBrowserURL(ctx context.Context, pageURL string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.CommandContext(ctx, "open", pageURL)
	case "windows":
		cmd = exec.CommandContext(ctx, "rundll32", "url.dll,FileProtocolHandler", pageURL)
	default:
		cmd = exec.CommandContext(ctx, "xdg-open", pageURL)
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		if trimmed := strings.TrimSpace(string(out)); trimmed != "" {
			return fmt.Errorf("%s: %w", trimmed, err)
		}
		return err
	}
	return nil
}
