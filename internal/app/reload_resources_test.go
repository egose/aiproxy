package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime/pprof"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/payloadlog"
	"github.com/egose/aiproxy/internal/providerhealth"
	"github.com/egose/aiproxy/internal/store"
	"github.com/google/uuid"
)

func payloadWorkerCount() int {
	var stacks bytes.Buffer
	_ = pprof.Lookup("goroutine").WriteTo(&stacks, 2)
	return strings.Count(stacks.String(), "internal/payloadlog.(*Logger).retentionLoop(")
}

func awaitResourceCondition(t *testing.T, description string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", description)
}

func resourceTestConfig(baseURL, disk, exceptions string) string {
	return healthcheckTestConfig(baseURL, "") + fmt.Sprintf(`
dashboard { token = "resource-test" }
logging {
  access_log = false
  payload_log {
    enabled = true
    dir = %q
  }
}
ingress_guardrails {
  enabled = true
  mode = "audit"
  exceptions_file = %q
}
alias "chat" {
  algorithm = "round_robin"
  target {
    provider = "local"
    model = "m"
  }
}
`, disk, exceptions)
}

func resourceChat(h http.Handler, model string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"`+model+`","messages":[]}`)))
	return w
}

func TestReloadLateFailurePreservesLiveResources(t *testing.T) {
	for _, failure := range []string{"corrupt-reused", "corrupt-new", "unreadable", "disk", "mongo", "token"} {
		t.Run(failure, func(t *testing.T) {
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			var probeStatus atomic.Int32
			probeStatus.Store(http.StatusOK)
			old := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/health" {
					w.WriteHeader(int(probeStatus.Load()))
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"source":"old","choices":[]}`)
			}))
			defer old.Close()
			var candidateCalls atomic.Int32
			next := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				candidateCalls.Add(1)
				w.WriteHeader(http.StatusInternalServerError)
			}))
			defer next.Close()
			disk := t.TempDir()
			exceptions := filepath.Join(t.TempDir(), "exceptions.json")
			path := writeConfigFile(t, resourceTestConfig(old.URL, disk, exceptions))
			a, err := Build(context.Background(), BuildOptions{ConfigPath: path, LogOutput: io.Discard})
			if err != nil {
				t.Fatal(err)
			}
			defer a.Close()
			waitForHealthcheck(t, a, true)
			workers := payloadWorkerCount()
			activeConfig, activeHealth, activeDisk := a.Config, a.health, a.payloadLog
			activeResolver, activeScanner, activeExceptions := a.resolver, a.guardrails, a.exceptions

			nextDisk := disk
			if failure != "corrupt-reused" {
				nextDisk = filepath.Join(t.TempDir(), "candidate-payloads")
			}
			nextExceptions := exceptions
			wantError := "ingress guardrails exceptions"
			switch failure {
			case "corrupt-reused", "corrupt-new":
				rewriteConfigFile(t, exceptions, "{broken json")
			case "unreadable":
				nextExceptions = t.TempDir()
			case "disk":
				rewriteConfigFile(t, nextDisk, "not a directory")
				wantError = "payload log"
			case "mongo":
				wantError = "payload mongodb"
			case "token":
				wantError = "dashboard token"
			}
			candidate := resourceTestConfig(next.URL, nextDisk, nextExceptions)
			candidate = strings.Replace(candidate, `path = "/health"`, `path = "/candidate-health"`, 1)
			candidate += `provider "openai-compatible" "candidate-only" {
  base_url = "http://127.0.0.1:1"
  api_key = "test"
  model "m" {}
}`
			if failure != "corrupt-reused" {
				candidate += "\nprovider_health { cooldown = \"60s\" }\n"
			}
			if failure == "mongo" {
				listener, err := net.Listen("tcp", "127.0.0.1:0")
				if err != nil {
					t.Fatal(err)
				}
				defer listener.Close()
				mongo := fmt.Sprintf("mongodb {\n uri = %q\n timeout = \"25ms\"\n}\n", "mongodb://"+listener.Addr().String())
				candidate = strings.Replace(candidate, "payload_log {", "payload_log {\n"+mongo, 1)
			}
			candidate = strings.Replace(candidate, `dashboard { token = "resource-test" }`, `dashboard {}`, 1)
			if failure == "token" {
				blocked := filepath.Join(t.TempDir(), "blocked")
				rewriteConfigFile(t, blocked, "not a directory")
				t.Setenv("XDG_CONFIG_HOME", blocked)
			}
			rewriteConfigFile(t, path, candidate)
			if err := a.Reload(); err == nil || !strings.Contains(err.Error(), wantError) {
				t.Fatalf("reload error = %v, want %q", err, wantError)
			}
			if a.Config != activeConfig || a.health != activeHealth || a.payloadLog != activeDisk || a.resolver != activeResolver || a.guardrails != activeScanner || a.exceptions != activeExceptions {
				t.Fatal("failed reload replaced live resources")
			}
			awaitResourceCondition(t, "candidate payload worker cleanup", func() bool { return payloadWorkerCount() == workers })
			if failure != "disk" && failure != "corrupt-reused" {
				if info, err := os.Stat(nextDisk); err != nil || !info.IsDir() {
					t.Fatalf("candidate sink was not acquired: %v", err)
				}
			}
			if _, exists := a.health.Snapshot()["candidate-only"]; exists {
				t.Fatal("failed reload mutated the live health catalog")
			}
			if failure != "token" {
				if _, err := os.Stat(filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "aiproxy", "dashboard.token")); !os.IsNotExist(err) {
					t.Fatalf("failed preparation published a dashboard token: %v", err)
				}
			}
			probeStatus.Store(http.StatusServiceUnavailable)
			awaitResourceCondition(t, "old probe to mark old tracker unhealthy", func() bool {
				st, ok := a.healthchecks.StatusFor("local")
				return ok && st.Path == "/health" && st.Checked && !st.Healthy && !activeHealth.IsHealthy("local")
			})
			w := httptest.NewRecorder()
			a.Server.Handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/readyz", nil))
			if w.Code != http.StatusServiceUnavailable {
				t.Fatalf("readiness = %d, want 503", w.Code)
			}
			probeStatus.Store(http.StatusOK)
			awaitResourceCondition(t, "old probe recovery", func() bool {
				st, ok := a.healthchecks.StatusFor("local")
				return ok && st.Healthy && activeHealth.IsHealthy("local")
			})
			for _, model := range []string{"local/m", "alias/chat"} {
				w := resourceChat(a.Server.Handler, model)
				if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"source":"old"`) {
					t.Fatalf("old routing failed: %d %s", w.Code, w.Body.String())
				}
			}
			if got := len(readPayloadLines(t, disk)); got != 2 {
				t.Fatalf("active payload entries = %d, want 2", got)
			}
			if got := candidateCalls.Load(); got != 0 {
				t.Fatalf("rejected candidate received %d calls", got)
			}
		})
	}
}

func TestBuildLateFailureCleansResources(t *testing.T) {
	for _, failure := range []string{"exceptions", "mongo"} {
		t.Run(failure, func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.WriteHeader(http.StatusOK)
			}))
			defer upstream.Close()
			exceptions := filepath.Join(t.TempDir(), "exceptions.json")
			disk := filepath.Join(t.TempDir(), "payloads")
			cfg := resourceTestConfig(upstream.URL, disk, exceptions)
			wantError := "ingress guardrails exceptions"
			if failure == "exceptions" {
				rewriteConfigFile(t, exceptions, "{broken json")
			} else {
				listener, err := net.Listen("tcp", "127.0.0.1:0")
				if err != nil {
					t.Fatal(err)
				}
				defer listener.Close()
				cfg = strings.Replace(cfg, "payload_log {", fmt.Sprintf("payload_log {\n mongodb {\n uri = %q\n timeout = \"25ms\"\n }", "mongodb://"+listener.Addr().String()), 1)
				wantError = "payload mongodb"
			}
			workers := payloadWorkerCount()
			a, err := Build(context.Background(), BuildOptions{ConfigPath: writeConfigFile(t, cfg), LogOutput: io.Discard})
			if a != nil {
				defer a.Close()
				t.Fatal("failed build returned an app")
			}
			if err == nil || !strings.Contains(err.Error(), wantError) {
				t.Fatalf("build error = %v, want %q", err, wantError)
			}
			if info, err := os.Stat(disk); err != nil || !info.IsDir() {
				t.Fatalf("sink was not acquired before failure: %v", err)
			}
			awaitResourceCondition(t, "failed Build payload cleanup", func() bool { return payloadWorkerCount() == workers })
			time.Sleep(50 * time.Millisecond)
			if got := calls.Load(); got != 0 {
				t.Fatalf("failed Build started probes: %d calls", got)
			}
		})
	}
}

func TestCandidateCleanupRetainsErrorsAndSuccessfulOwnership(t *testing.T) {
	closeErr := errors.New("health close failed")
	backend := &countingHealthBackend{closeErr: closeErr}
	tracker := providerhealth.NewWithBackend(nil, config.ProviderHealth{}, backend)
	workers := payloadWorkerCount()
	disk, err := payloadlog.New(config.PayloadLog{Enabled: true, Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer disk.Close()
	awaitResourceCondition(t, "payload worker startup", func() bool { return payloadWorkerCount() == workers+1 })
	candidate := candidateResources{health: tracker, disk: disk}
	var result error
	candidate.cleanup(&result)
	if backend.closes.Load() != 0 || payloadWorkerCount() != workers+1 {
		t.Fatal("successful construction closed the transferred tracker")
	}
	failure := errors.New("preparation failed")
	result = failure
	candidate.cleanup(&result)
	if !errors.Is(result, failure) || !errors.Is(result, closeErr) {
		t.Fatalf("cleanup lost original or close error: %v", result)
	}
	assertHealthClosedOnce(t, backend)
	awaitResourceCondition(t, "candidate worker cleanup despite close error", func() bool { return payloadWorkerCount() == workers })
}

func TestReloadLateFailureDoesNotCloseReusedHealth(t *testing.T) {
	exceptions := filepath.Join(t.TempDir(), "exceptions.json")
	cfg := guardrailTestConfig("http://127.0.0.1:1", fmt.Sprintf("ingress_guardrails {\n enabled = true\n exceptions_file = %q\n}\n", exceptions))
	a, err := Build(context.Background(), BuildOptions{ConfigPath: writeConfigFile(t, cfg), LogOutput: io.Discard})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if err := a.health.Close(); err != nil {
		t.Fatal(err)
	}
	backend := &countingHealthBackend{}
	a.health = providerhealth.NewWithBackend(a.metrics, a.Config.ProviderHealth, backend)
	if err := a.Reload(); err != nil {
		t.Fatal(err)
	}
	rewriteConfigFile(t, exceptions, "{broken json")
	if err := a.Reload(); err == nil || !strings.Contains(err.Error(), "ingress guardrails exceptions") {
		t.Fatalf("reload error = %v", err)
	}
	if backend.closes.Load() != 0 {
		t.Fatal("rollback closed the reused tracker")
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	assertHealthClosedOnce(t, backend)
}

func TestBuildLateFailureClosesAdminStore(t *testing.T) {
	dsn := os.Getenv("AIPROXY_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("AIPROXY_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	admin, err := store.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close() })
	schema := "boundary04_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := admin.DB.ExecContext(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.DB.ExecContext(ctx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Error(err)
		}
	})
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	query := u.Query()
	query.Set("search_path", schema)
	u.RawQuery = query.Encode()
	fixture, err := store.Open(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fixture.Close() })
	if _, err := fixture.MigrateUp(ctx); err != nil {
		t.Fatal(err)
	}
	query.Set("application_name", schema)
	u.RawQuery = query.Encode()
	exceptions := filepath.Join(t.TempDir(), "exceptions.json")
	rewriteConfigFile(t, exceptions, "{broken json")
	cfg := resourceTestConfig("http://127.0.0.1:1", t.TempDir(), exceptions) + fmt.Sprintf("\ndatabase { url = %q }\nmulti_tenancy { enabled = true }\n", u.String())
	path := writeConfigFile(t, cfg)
	a, err := Build(ctx, BuildOptions{ConfigPath: path, LogOutput: io.Discard})
	if a != nil {
		defer a.Close()
		t.Fatal("failed Build returned an app")
	}
	if err == nil || !strings.Contains(err.Error(), "ingress guardrails exceptions") {
		t.Fatalf("Build did not reach the late failure: %v", err)
	}
	awaitResourceCondition(t, "failed Build database connection cleanup", func() bool {
		var count int
		if err := admin.DB.NewRaw("SELECT count(*) FROM pg_stat_activity WHERE application_name = ?", schema).Scan(ctx, &count); err != nil {
			t.Fatal(err)
		}
		return count == 0
	})
	rewriteConfigFile(t, exceptions, `{"version":1,"entries":{}}`)
	a, err = Build(ctx, BuildOptions{ConfigPath: path, LogOutput: io.Discard})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	var count int
	if err := admin.DB.NewRaw("SELECT count(*) FROM pg_stat_activity WHERE application_name = ?", schema).Scan(ctx, &count); err != nil {
		t.Fatal(err)
	}
	if count == 0 {
		t.Fatal("successful Build did not retain its database connection")
	}
	if err := a.adminStore.Ping(ctx); err != nil {
		t.Fatalf("successful Build closed its store: %v", err)
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	if err := a.adminStore.Ping(ctx); err == nil {
		t.Fatal("App.Close left the store open")
	}
}

func TestReloadSuccessfulResourcesWithConcurrentRequests(t *testing.T) {
	var oldProbes, newProbes atomic.Int32
	upstream := func(source string, probes *atomic.Int32) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/health" {
				probes.Add(1)
				w.WriteHeader(http.StatusOK)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"source":%q,"choices":[]}`, source)
		}))
	}
	old, next := upstream("old", &oldProbes), upstream("new", &newProbes)
	defer old.Close()
	defer next.Close()
	exceptions := filepath.Join(t.TempDir(), "exceptions.json")
	disk, nextDisk := t.TempDir(), t.TempDir()
	workers := payloadWorkerCount()
	path := writeConfigFile(t, resourceTestConfig(old.URL, disk, exceptions))
	a, err := Build(context.Background(), BuildOptions{ConfigPath: path, LogOutput: io.Discard})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	waitForHealthcheck(t, a, true)
	oldHealth, oldDisk := a.health, a.payloadLog
	var wg sync.WaitGroup
	stop := make(chan struct{})
	stopRequests := sync.OnceFunc(func() { close(stop); wg.Wait() })
	defer stopRequests()
	var requests atomic.Int32
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				w := resourceChat(a.Server.Handler, "alias/chat")
				if w.Code != http.StatusOK {
					t.Errorf("concurrent request: %d %s", w.Code, w.Body.String())
					return
				}
				requests.Add(1)
			}
		}()
	}
	awaitResourceCondition(t, "concurrent traffic", func() bool { return requests.Load() >= 4 })
	if err := a.Reload(); err != nil {
		t.Fatal(err)
	}
	if a.health != oldHealth || a.payloadLog != oldDisk {
		t.Fatal("unchanged reload replaced reusable resources")
	}
	rewriteConfigFile(t, path, resourceTestConfig(next.URL, disk, exceptions)+"\nprovider_health { cooldown = \"60s\" }\n")
	if err := a.Reload(); err != nil {
		t.Fatal(err)
	}
	if a.health == oldHealth || a.payloadLog != oldDisk {
		t.Fatal("reload did not replace changed health and retain the unchanged sink")
	}
	awaitResourceCondition(t, "new probe activation", func() bool { return newProbes.Load() > 0 })
	waitForHealthcheck(t, a, true)
	for _, model := range []string{"local/m", "alias/chat"} {
		w := resourceChat(a.Server.Handler, model)
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"source":"new"`) {
			t.Fatalf("new routing failed: %d %s", w.Code, w.Body.String())
		}
	}
	stopRequests()
	rewriteConfigFile(t, path, resourceTestConfig(next.URL, nextDisk, exceptions)+"\nprovider_health { cooldown = \"60s\" }\n")
	if err := a.Reload(); err != nil {
		t.Fatal(err)
	}
	if a.payloadLog == oldDisk {
		t.Fatal("changed sink was not replaced")
	}
	if w := resourceChat(a.Server.Handler, "alias/chat"); w.Code != http.StatusOK {
		t.Fatalf("request after sink replacement: %d %s", w.Code, w.Body.String())
	}
	awaitResourceCondition(t, "retired payload worker cleanup", func() bool { return payloadWorkerCount() == workers+1 })
	if len(readPayloadLines(t, nextDisk)) == 0 {
		t.Fatal("new sink did not record traffic")
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	awaitResourceCondition(t, "successful app payload cleanup", func() bool { return payloadWorkerCount() == workers })
}
