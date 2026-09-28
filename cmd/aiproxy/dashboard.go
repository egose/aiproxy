package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	neturl "net/url"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/dashboard"
	"github.com/egose/aiproxy/internal/dashrpc"
	"github.com/egose/aiproxy/internal/payloadlog"
	"github.com/spf13/cobra"
)

const dashPollInterval = 2 * time.Second

func newDashboardCommand() *cobra.Command {
	var cfgPath string
	cmd := &cobra.Command{
		Use:   "dashboard",
		Short: "Attach an interactive dashboard to a running aiproxy server",
		Long: `Attach a local interactive dashboard. Minimum terminal: 80x12.
Below 30 rows, only the focused pane is shown. Tab/Shift-Tab cycles panes.
1/2/3/4/5 selects Aliases/Logs/Payloads/Blocks/Requests; [ previous and ] next wraps tabs.
Enter opens/closes detail (Usage: top group, n/N cycles identities); z toggles pane zoom. Navigation closes detail first.
Provider detail shows sanitized endpoint, configured models and probe diagnostics; HOST never runs DNS.
? or h opens scrollable help. Esc closes help, then detail, then zoom; otherwise quits.
q quits outside search; Ctrl+C quits in every mode. Help consumes pane keys; numbers never record decisions.
Requests: last <=200 completed operations, independent of disk payload logs; no attempt/in-flight history.
Request detail includes ID, tenant/client, public/resolved model, status, duration and reported tokens.
/ searches retained Requests/Logs/Payloads metadata (256 characters, AND substrings, field:value).
Enter applies search, Esc cancels, Ctrl+U clears. q/p/h/?/numbers/brackets are text while editing.
Requests fields: id/client/tenant/model/resolved/provider/status/op. Logs: id/level.
Payload fields: id/model/resolved/provider/status/method/path. Filters combine with pane status/level.
Request detail l/v opens exact-ID logs/payloads; Esc returns with context. Missing/expired data is explained.
Payload/block detail wraps long lines; j/k, PgUp/PgDn, Home/End inspect within-cap text.
Payload pretty output is capped at 64 KiB with explicit truncation notices; block snippet truncation is unknown.
Blocks: Enter consumes take-once capture (re-open unavailable). n/N selects the next/previous finding.
a/s/d allows non-secret / redacts / denies ONLY the selected hash, persisting GLOBAL future-match behavior.
Decisions never replay the request. Pending locks finding selection and duplicate actions; success suppresses repeats.
Failed outcomes may be unknown; a/s/d deliberately retries. Missing/invalid hashes cannot be decided.
Usage t/e/u filters and log l/order keys require that pane's focus.
Transient disconnects keep stale data and retry after 2–30 seconds. Ctrl+R retries now.
Auth/config denial stops automatic retries; fix the server then Ctrl+R, or quit and re-attach to reload credentials.
Press p to freeze data and the display clock; resume applies the latest buffered results.
Connection status stays live. Browse cached rows while paused; resume before remote refresh/filter/detail/decisions.
Refresh keeps row identities; removed rows use the nearest clamped position, removed tenants select all.
Rates are global (last 60/300 complete seconds); provider counters are lifetime.
Usage and estimated costs use rolling 24-hour, one-minute retention buckets.
P95/n uses positive durations in up to 200 recent completions, with no time window.
Older servers may report measurement data as unavailable.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDashboardWithInput(cmd.Context(), cfgPath, configFlagExplicit(cmd), cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
	cmd.Flags().StringVarP(&cfgPath, "config", "c", defaultConfigPath(), "path to config file (overrides $AIPROXY_CONFIG)")
	return cmd
}

func runDashboard(parentCtx context.Context, cfgPath string, explicit bool, stdout, stderr io.Writer) error {
	return runDashboardWithInput(parentCtx, cfgPath, explicit, os.Stdin, stdout, stderr)
}

func runDashboardWithInput(parentCtx context.Context, cfgPath string, explicit bool, stdin io.Reader, stdout, stderr io.Writer) error {
	rt, err := config.LoadFileOrEnv(cfgPath, explicit)
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
	if err := validateDashboardTransport(baseURL); err != nil {
		fmt.Fprintln(stderr, err.Error())
		return err
	}
	httpClient := &http.Client{Timeout: 2 * time.Second, CheckRedirect: dashboardNoRedirect}
	defer httpClient.CloseIdleConnections()

	ctx, stop := signal.NotifyContext(parentCtx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	initial, err := fetchSnapshot(ctx, httpClient, baseURL, token)
	if err != nil {
		if ctx.Err() != nil {
			return nil
		}
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

	fetcher := &dashboardPayloadFetcher{client: dashrpc.NewClient(baseURL, token)}
	fetcher.client.HTTP.CheckRedirect = dashboardNoRedirect
	defer fetcher.client.HTTP.CloseIdleConnections()
	return attachDashboard(ctx, initial, fetcher, dashboard.RunOptions{Input: stdin, Output: stdout, SignalsHandled: true},
		func(ctx context.Context) (dashrpc.Snapshot, error) {
			return fetchSnapshot(ctx, httpClient, baseURL, token)
		}, startDashboardProgram, dashPollInterval)
}

// dashboardPayloadFetcher adapts the dashrpc HTTP client to the dashboard
// TUI's payload viewer: newest-first on-disk payload log summaries plus
// on-demand prettified single entries.
type dashboardPayloadFetcher struct {
	client *dashrpc.AuthenticatedClient
	denied atomic.Bool
}

func (f *dashboardPayloadFetcher) ListPayloads(ctx context.Context, limit int, errorsOnly bool) (dashrpc.PayloadList, error) {
	if f != nil && f.denied.Load() {
		return dashrpc.PayloadList{}, errDashboardDenied
	}
	if f == nil || f.client == nil {
		return dashrpc.PayloadList{}, errors.New("payload client not configured")
	}
	return f.client.FetchPayloads(ctx, limit, errorsOnly)
}

func (f *dashboardPayloadFetcher) GetPayload(ctx context.Context, requestID string) (string, error) {
	if f != nil && f.denied.Load() {
		return "", errDashboardDenied
	}
	if f == nil || f.client == nil {
		return "", errors.New("payload client not configured")
	}
	raw, err := f.client.FetchPayload(ctx, requestID)
	if err != nil {
		return "", err
	}
	return payloadlog.Pretty(raw, 64<<10), nil
}

func (f *dashboardPayloadFetcher) ListBlocks(ctx context.Context) (dashrpc.BlockList, error) {
	if f != nil && f.denied.Load() {
		return dashrpc.BlockList{}, errDashboardDenied
	}
	if f == nil || f.client == nil {
		return dashrpc.BlockList{}, errors.New("block client not configured")
	}
	return f.client.FetchBlocks(ctx)
}

func (f *dashboardPayloadFetcher) GetBlock(ctx context.Context, blockID string) (dashrpc.BlockCapture, error) {
	if f != nil && f.denied.Load() {
		return dashrpc.BlockCapture{}, errDashboardDenied
	}
	if f == nil || f.client == nil {
		return dashrpc.BlockCapture{}, errors.New("block client not configured")
	}
	return f.client.FetchBlock(ctx, blockID)
}

func (f *dashboardPayloadFetcher) DecideBlock(ctx context.Context, blockID, action string, shas []string) (dashrpc.BlockDecisionResponse, error) {
	if f != nil && f.denied.Load() {
		return dashrpc.BlockDecisionResponse{}, errDashboardDenied
	}
	if f == nil || f.client == nil {
		return dashrpc.BlockDecisionResponse{}, errors.New("block client not configured")
	}
	return f.client.DecideBlock(ctx, blockID, action, shas)
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
	return "http://" + addr
}

var errDashboardInsecureTransport = errors.New("dashboard transport insecure")

func validateDashboardTransport(baseURL string) error {
	u, err := neturl.Parse(baseURL)
	if err != nil || u.Host == "" {
		return fmt.Errorf("dashboard: cannot parse listener address %q", baseURL)
	}
	if u.Scheme != "http" {
		return fmt.Errorf("dashboard: unsupported listener scheme %q (only local http is supported)", u.Scheme)
	}
	if isLoopbackURLHost(u.Hostname()) {
		return nil
	}
	return fmt.Errorf("%w: dashboard command is local-only; listener %q is non-loopback", errDashboardInsecureTransport, baseURL)
}

func isLoopbackURLHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

var errDashboardUnconfigured = errors.New("dashboard not configured")
var errDashboardUnauthorized = errors.New("dashboard token mismatch")
var errDashboardForbidden = errors.New("dashboard access forbidden")
var errDashboardDenied = errors.New("dashboard access paused: fix server then Ctrl+R; re-attach to reload credentials")

func fetchSnapshot(ctx context.Context, c *http.Client, baseURL, token string) (dashrpc.Snapshot, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+dashrpc.SnapshotPath, nil)
	if err != nil {
		return dashrpc.Snapshot{}, err
	}
	req.Header.Set(dashrpc.AuthHeaderName, dashrpc.AuthScheme+token)
	resp, err := c.Do(req)
	if err != nil {
		return dashrpc.Snapshot{}, fmt.Errorf("%w: %w", errTransport, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return dashrpc.Snapshot{}, errDashboardUnconfigured
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return dashrpc.Snapshot{}, errDashboardUnauthorized
	}
	if resp.StatusCode == http.StatusForbidden {
		return dashrpc.Snapshot{}, errDashboardForbidden
	}
	if resp.StatusCode != http.StatusOK {
		return dashrpc.Snapshot{}, snapshotStatusError(resp.StatusCode)
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
