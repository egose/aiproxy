package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/accounting"
	"github.com/egose/aiproxy/internal/auth"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/dashrpc"
	"github.com/egose/aiproxy/internal/modelresolver"
	"github.com/egose/aiproxy/internal/observability"
	"github.com/egose/aiproxy/internal/providerhealth"
)

const dashboardTestToken = "sekret"

func newDashboardDeps(rt *config.Runtime, startTime time.Time, usage *accounting.Aggregator, health *providerhealth.Tracker, logs *observability.LogBuffer) Dependencies {
	return Dependencies{
		Resolver:           modelresolver.New(rt),
		Auth:               auth.NewAuthenticator(config.Auth{Mode: config.AuthModeNone}),
		Providers:          rt.ProviderByName,
		Metrics:            observability.NewMetrics(),
		Health:             health,
		Usage:              usage,
		Dashboard:          config.Dashboard{Token: dashboardTestToken, Enabled: true},
		Logs:               logs,
		DashboardVersion:   "test",
		DashboardAddress:   rt.Listener.Address,
		DashboardAuthMode:  string(rt.Auth.Mode),
		DashboardStartTime: startTime,
		DashboardProviders: rt.Providers,
		DashboardDisabled:  rt.DisabledProviders,
		DashboardAliases:   rt.Aliases,
	}
}

func TestDashboardSnapshotEndpointRejectsUnauthenticatedRequests(t *testing.T) {
	rt := newRT()
	rt.Listener = config.Listener{Address: ":8080"}
	usage := accounting.NewAggregator()
	health := providerhealth.New(nil, config.ProviderHealth{})
	health.SetProviders(rt.ProviderByName)
	logs := observability.NewLogBuffer(10)
	start := time.Now()

	h := NewHandler(newDashboardDeps(rt, start, usage, health, logs))

	cases := []struct {
		name   string
		header string
		want   int
	}{
		{"missing-header", "", http.StatusUnauthorized},
		{"wrong-scheme", dashboardTestToken, http.StatusUnauthorized},
		{"wrong-token", "Bearer nope", http.StatusUnauthorized},
		{"empty-token", "Bearer ", http.StatusUnauthorized},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, dashrpc.SnapshotPath, nil)
			if tc.header != "" {
				r.Header.Set(dashrpc.AuthHeaderName, tc.header)
			}
			h.ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatalf("%s: status = %d, want %d", tc.name, w.Code, tc.want)
			}
		})
	}
}

func TestDashboardSnapshotEndpointServesJSON(t *testing.T) {
	rt := newRT()
	rt.Listener = config.Listener{Address: ":8080"}
	usage := accounting.NewAggregator()
	usage.Record(accounting.Event{
		Model: "openai/gpt-4o-mini", Operation: "chat_completions",
		StatusCode: 200, Duration: 50 * time.Millisecond,
	})
	health := providerhealth.New(nil, config.ProviderHealth{})
	health.SetProviders(rt.ProviderByName)
	logs := observability.NewLogBuffer(10)
	logs.Add(observability.LogEntry{Level: slog.LevelInfo, Message: "starting"})
	start := time.Now().Add(-time.Minute)

	h := NewHandler(newDashboardDeps(rt, start, usage, health, logs))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, dashrpc.SnapshotPath, nil)
	r.Header.Set(dashrpc.AuthHeaderName, "Bearer "+dashboardTestToken)
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}

	var snap dashrpc.Snapshot
	if err := json.Unmarshal(w.Body.Bytes(), &snap); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if snap.Version != "test" {
		t.Fatalf("version = %q, want test", snap.Version)
	}
	if snap.Address != ":8080" {
		t.Fatalf("address = %q, want :8080", snap.Address)
	}
	if len(snap.Providers) != 1 || snap.Providers[0].Name != "openai" {
		t.Fatalf("providers = %+v", snap.Providers)
	}
	if len(snap.Health) == 0 || !snap.Health["openai"] {
		t.Fatalf("health = %+v, want openai=true", snap.Health)
	}
	if len(snap.Usage) != 1 {
		t.Fatalf("usage = %+v, want 1 row", snap.Usage)
	}
	if len(snap.Logs) != 1 {
		t.Fatalf("logs = %+v, want 1 entry", snap.Logs)
	}
}

func TestDashboardLogsEndpointReturnsNewEntries(t *testing.T) {
	rt := newRT()
	rt.Listener = config.Listener{Address: ":8080"}
	usage := accounting.NewAggregator()
	health := providerhealth.New(nil, config.ProviderHealth{})
	health.SetProviders(rt.ProviderByName)
	logs := observability.NewLogBuffer(50)
	logs.Add(observability.LogEntry{Level: slog.LevelInfo, Message: "first"})
	logs.Add(observability.LogEntry{Level: slog.LevelInfo, Message: "second"})
	start := time.Now()

	h := NewHandler(newDashboardDeps(rt, start, usage, health, logs))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, dashrpc.LogsPath, nil)
	r.Header.Set(dashrpc.AuthHeaderName, "Bearer "+dashboardTestToken)
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("logs status = %d", w.Code)
	}
	var resp struct {
		Logs    []observability.LogEntry `json:"logs"`
		LastSeq uint64                   `json:"last_seq"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Logs) != 2 {
		t.Fatalf("logs = %+v, want 2 entries", resp.Logs)
	}
	if resp.LastSeq == 0 {
		t.Fatal("LastSeq should be non-zero")
	}

	// Polling with the returned LastSeq should yield zero new entries.
	w2 := httptest.NewRecorder()
	r2 := httptest.NewRequest(http.MethodGet, dashrpc.LogsPath+"?since="+strconv.FormatUint(resp.LastSeq, 10), nil)
	r2.Header.Set(dashrpc.AuthHeaderName, "Bearer "+dashboardTestToken)
	h.ServeHTTP(w2, r2)
	if w2.Code != http.StatusOK {
		t.Fatalf("poll status = %d", w2.Code)
	}
	var resp2 struct {
		Logs    []observability.LogEntry `json:"logs"`
		LastSeq uint64                   `json:"last_seq"`
	}
	_ = json.Unmarshal(w2.Body.Bytes(), &resp2)
	if len(resp2.Logs) != 0 {
		t.Fatalf("polled logs = %+v, want 0", resp2.Logs)
	}
}
