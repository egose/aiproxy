package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/upstreamhttp"
)

func tlsAppHealthConfig(base, selector, source, agent string) string {
	contents := tlsAppConfig(base, selector, source)
	health := fmt.Sprintf(`  user_agent = %q
  healthcheck {
    path = "/health"
    interval = "1s"
    timeout = "500ms"
    send_authorization = true
    expected_body = "*"
  }
  model "m" {}`, agent)
	return strings.Replace(contents, `  model "m" {}`, health, 1)
}

func TestTLSHealthRuntimeReloadAndOwnedLifecycle(t *testing.T) {
	if os.Getenv("AIPROXY_APP_HEALTH_TLS_CHILD") != "1" {
		cmd := exec.Command(os.Args[0], "-test.run=^TestTLSHealthRuntimeReloadAndOwnedLifecycle$", "-test.timeout=40s")
		for _, value := range os.Environ() {
			if !strings.HasPrefix(value, "AIPROXY_TEST_DATABASE_URL=") && !strings.HasPrefix(value, "AIPROXY_APP_HEALTH_TLS_CHILD=") {
				cmd.Env = append(cmd.Env, value)
			}
		}
		cmd.Env = append(cmd.Env, "AIPROXY_APP_HEALTH_TLS_CHILD=1")
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("app health TLS child: %v\n%s", err, output)
		}
		return
	}
	cert := tlsAppTestCertificate(t)
	t.Run("retained-probes-and-same-path-custom", func(t *testing.T) {
		var agentsMu sync.Mutex
		var agents []string
		server, observations := tlsAppServer(t, cert, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if req.URL.Path == "/health" {
				agentsMu.Lock()
				agents = append(agents, req.UserAgent())
				agentsMu.Unlock()
				if req.Header.Get("Authorization") != "Bearer synthetic-local" || req.Header.Get("Cookie") != "" {
					t.Error("health authorization contract changed")
				}
				_, _ = io.WriteString(w, "ready")
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"choices":[]}`)
		}))
		one, two := appTestCustomTLSProfile(t, false), appTestCustomTLSProfile(t, true)
		source := filepath.Join(t.TempDir(), "health.json")
		rewriteConfigFile(t, source, string(one.CanonicalJSON()))
		path := writeConfigFile(t, tlsAppHealthConfig(server.URL, "custom", source, "health-initial"))
		app := tlsAppBuild(t, path)
		waitForHealthcheck(t, app, true)
		initial := observations.snapshot()[0]
		requireAppTLSProfile(t, one, initial)
		if response := resourceChat(app.Server.Handler, "local/m"); response.Code != http.StatusOK {
			t.Fatal(response.Body.String())
		}
		requests := observations.snapshot()
		inference := requests[len(requests)-1]
		if inference.hello.remote == initial.hello.remote {
			t.Fatal("health borrowed the inference generation's transport")
		}
		for i := 0; i < 5; i++ {
			rewriteConfigFile(t, path, tlsAppHealthConfig(server.URL, "custom", source, fmt.Sprintf("health-reload-%d", i)))
			if err := app.Reload(); err != nil {
				t.Fatal(err)
			}
		}
		awaitResourceCondition(t, "retired inference connection", func() bool { return observations.connectionClosed(inference.hello.remote) })
		awaitResourceCondition(t, "retained health snapshot refresh", func() bool {
			agentsMu.Lock()
			defer agentsMu.Unlock()
			return len(agents) > 1 && agents[len(agents)-1] == "health-reload-4"
		})
		if observations.connectionClosed(initial.hello.remote) {
			t.Fatal("unchanged health was retained through a retired app client callback")
		}
		latest := observations.snapshot()
		if latest[len(latest)-1].hello.remote != initial.hello.remote {
			t.Fatal("unchanged probe lost its owned reusable client")
		}
		before := len(latest)
		rewriteConfigFile(t, source, string(two.CanonicalJSON()))
		if err := app.Reload(); err != nil {
			t.Fatal(err)
		}
		awaitResourceCondition(t, "changed custom health probe", func() bool { return len(observations.snapshot()) > before })
		latest = observations.snapshot()
		changed := latest[len(latest)-1]
		requireAppTLSProfile(t, two, changed)
		if changed.proto != 2 || changed.hello.remote == initial.hello.remote {
			t.Fatal("health same-path custom reload used a stale transport/profile")
		}
		awaitResourceCondition(t, "retired old health connection", func() bool { return observations.connectionClosed(initial.hello.remote) })
		activeConfig, activeClients := app.Config, app.clients
		before = len(latest)
		rewriteConfigFile(t, source, "{invalid replacement")
		if err := app.Reload(); err == nil || app.Config != activeConfig || app.clients != activeClients {
			t.Fatal("failed custom reload changed active health/runtime")
		}
		server.CloseClientConnections()
		awaitResourceCondition(t, "cached custom health replay after failed reload", func() bool { return len(observations.snapshot()) > before })
		latest = observations.snapshot()
		requireAppTLSProfile(t, two, latest[len(latest)-1])
		waitForHealthcheck(t, app, true)
		if err := app.Close(); err != nil {
			t.Fatal(err)
		}
		last := latest[len(latest)-1]
		awaitResourceCondition(t, "health client cleanup on app close", func() bool { return observations.connectionClosed(last.hello.remote) })
	})
	t.Run("active-probe-canceled-before-new-profile", func(t *testing.T) {
		entered, exited := make(chan struct{}), make(chan struct{})
		var calls atomic.Int32
		server, observations := tlsAppServer(t, cert, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if calls.Add(1) == 1 {
				close(entered)
				<-req.Context().Done()
				close(exited)
				return
			}
			_, _ = io.WriteString(w, "ready")
		}))
		one, two := appTestCustomTLSProfile(t, false), appTestCustomTLSProfile(t, true)
		source := filepath.Join(t.TempDir(), "health.json")
		rewriteConfigFile(t, source, string(one.CanonicalJSON()))
		app := tlsAppBuild(t, writeConfigFile(t, tlsAppHealthConfig(server.URL, "custom", source, "health")))
		select {
		case <-entered:
		case <-time.After(3 * time.Second):
			t.Fatal("initial health request did not start")
		}
		rewriteConfigFile(t, source, string(two.CanonicalJSON()))
		if err := app.Reload(); err != nil {
			t.Fatal(err)
		}
		select {
		case <-exited:
		case <-time.After(time.Second):
			t.Fatal("profile reload did not cancel retired health work")
		}
		waitForHealthcheck(t, app, true)
		requests := observations.snapshot()
		if len(requests) < 2 {
			t.Fatal("new health profile did not activate")
		}
		requireAppTLSProfile(t, one, requests[0])
		requireAppTLSProfile(t, two, requests[1])
		awaitResourceCondition(t, "canceled old health connection", func() bool { return observations.connectionClosed(requests[0].hello.remote) })
	})
}

func TestHealthTransportPreparationFailureRollsBackBuildAndReload(t *testing.T) {
	for _, scenario := range []string{"build", "reload"} {
		t.Run(scenario, func(t *testing.T) {
			var reject atomic.Bool
			reject.Store(scenario == "build")
			failure := errors.New("synthetic health-only transport failure")
			var made []*appTLSFixtureTransport
			factory := func(options upstreamhttp.Options) (upstreamhttp.Transport, error) {
				if reject.Load() && options.ResponseHeaderTimeout == 0 {
					return nil, failure
				}
				transport := &appTLSFixtureTransport{profile: options.Profile.CanonicalName()}
				made = append(made, transport)
				return transport, nil
			}
			contents := strings.Replace(tlsAppHealthConfig("http://127.0.0.1:1", "chrome", "", "health"), `interval = "1s"`, `interval = "24h"`, 1)
			path := writeConfigFile(t, contents)
			app, err := Build(context.Background(), BuildOptions{ConfigPath: path, LogOutput: io.Discard, transportFactory: factory})
			if scenario == "build" {
				if app != nil || !errors.Is(err, failure) {
					t.Fatalf("health preparation failure: %v", err)
				}
				for _, transport := range made {
					if transport.closes.Load() == 0 {
						t.Fatal("failed build retained its inference candidate")
					}
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			defer app.Close()
			waitForHealthcheck(t, app, true)
			status, _ := app.healthchecks.StatusFor("local")
			activeConfig, activeClients, activeTracker := app.Config, app.clients, app.health
			activeMade := len(made)
			reject.Store(true)
			disk := filepath.Join(t.TempDir(), "must-not-be-created")
			rewriteConfigFile(t, path, strings.Replace(contents, `tls_client_hello = "chrome"`, `tls_client_hello = "safari"`, 1)+fmt.Sprintf("\nlogging {\n payload_log {\n enabled = true\n dir = %q\n }\n}\n", disk))
			if err := app.Reload(); !errors.Is(err, failure) {
				t.Fatalf("health-only reload preparation failure: %v", err)
			}
			current, _ := app.healthchecks.StatusFor("local")
			if app.Config != activeConfig || app.clients != activeClients || app.health != activeTracker || current != status {
				t.Fatal("health preparation failure changed current probes/runtime/tracker")
			}
			for _, transport := range made[:activeMade] {
				if transport.closes.Load() != 0 {
					t.Fatal("failed health preparation retired live resources")
				}
			}
			for _, transport := range made[activeMade:] {
				if transport.closes.Load() == 0 {
					t.Fatal("failed health preparation retained candidate clients")
				}
			}
			if _, err := os.Stat(disk); !os.IsNotExist(err) {
				t.Fatal("health preparation failure started candidate sinks")
			}
			reject.Store(false)
			rewriteConfigFile(t, path, contents)
			if err := app.Reload(); err != nil {
				t.Fatal("discarded health candidate kept its activation lock")
			}
		})
	}
}
