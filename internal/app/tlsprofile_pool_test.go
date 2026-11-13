package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/tlsprofile"
	"github.com/egose/aiproxy/internal/upstreamhttp"
)

func appTestCustomTLSProfile(t *testing.T, h2 bool) tlsprofile.Profile {
	t.Helper()
	data, err := os.ReadFile("../tlsprofile/testdata/bun-1.4.4-fetch.json")
	if err != nil {
		t.Fatal(err)
	}
	if h2 {
		data = []byte(strings.Replace(string(data), `["http/1.1"]`, `["h2","http/1.1"]`, 1))
	}
	profile, err := tlsprofile.ParseCustom(data)
	if err != nil {
		t.Fatal(err)
	}
	return profile
}

func TestUpstreamPoolPrebuildsAndIsolatesProfilesAndContent(t *testing.T) {
	custom1, custom2 := appTestCustomTLSProfile(t, false), appTestCustomTLSProfile(t, true)
	providers := []config.Provider{
		{Name: "native", UpstreamHeaderTimeout: config.DefaultUpstreamHeaderTimeout},
		{Name: "chrome", TLSClientHello: "chrome"},
		{Name: "chrome-pinned", TLSClientHello: "chrome-133"},
		{Name: "safari", TLSClientHello: "safari"},
		{Name: "custom-one", TLSClientHello: "custom", TLSClientHelloSpecFile: "same.json", TLSProfile: custom1},
		{Name: "custom-two", TLSClientHello: "custom", TLSClientHelloSpecFile: "same.json", TLSProfile: custom2},
		{Name: "different-timeout", TLSClientHello: "chrome", UpstreamHeaderTimeout: 30 * time.Second},
	}
	catalog := config.NewCatalog(providers, nil, nil)
	var created atomic.Int32
	pool, err := prepareUpstreamClientPool(catalog, func(options upstreamhttp.Options) (upstreamhttp.Transport, error) {
		created.Add(1)
		if options.ResponseHeaderTimeout <= 0 {
			t.Error("inference default header timeout was not resolved")
		}
		return upstreamhttp.NewTransport(options)
	})
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Retire()
	if created.Load() != 6 || len(pool.clients) != 6 {
		t.Fatalf("unbounded/lazy pool preparation: created=%d entries=%d", created.Load(), len(pool.clients))
	}
	client := func(name string) *http.Client {
		t.Helper()
		provider, ok := catalog.Provider(name)
		if !ok {
			t.Fatal("missing provider")
		}
		return pool.ClientForProvider(provider)
	}
	if client("chrome") != client("chrome-pinned") || client("chrome") == client("safari") || client("custom-one") == client("custom-two") || client("different-timeout") == client("chrome") || client("native") != pool.Client(0) {
		t.Fatal("pool key does not contain effective timeout and immutable profile content")
	}
	for _, provider := range catalog.Providers() {
		if client(provider.Name).Timeout != 0 {
			t.Fatal("inference acquired a whole-request timeout")
		}
	}
	for range 20 {
		for _, provider := range catalog.Providers() {
			_ = pool.ClientForProvider(provider)
		}
	}
	if created.Load() != 6 {
		t.Fatal("requests created transports")
	}
	missing := config.Provider{TLSProfile: custom2, UpstreamHeaderTimeout: 123 * time.Second}
	req, _ := http.NewRequest(http.MethodGet, "https://not-contacted.test", nil)
	if _, err := pool.ClientForProvider(missing).Do(req); err == nil || !strings.Contains(err.Error(), "not prepared") || created.Load() != 6 {
		t.Fatalf("missing profile fell back to native/networking: %v", err)
	}
}

type appTLSFixtureTransport struct {
	profile string
	closes  atomic.Int32
}

func (t *appTLSFixtureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Body != nil {
		_ = req.Body.Close()
	}
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"choices":[],"profile":"` + t.profile + `"}`))}, nil
}

func (t *appTLSFixtureTransport) CloseIdleConnections() { t.closes.Add(1) }

func TestBuildAndReloadPrepareTLSBeforeRuntimeSideEffectsAndRollback(t *testing.T) {
	for _, scenario := range []string{"build", "reload"} {
		t.Run(scenario, func(t *testing.T) {
			dir := t.TempDir()
			payloads := filepath.Join(dir, "must-not-be-created")
			path := writeConfigFile(t, tlsAppConfig("http://127.0.0.1:1", "chrome", ""))
			failure := errors.New("synthetic transport preparation failure")
			var reject atomic.Bool
			reject.Store(scenario == "build")
			var made []*appTLSFixtureTransport
			factory := func(options upstreamhttp.Options) (upstreamhttp.Transport, error) {
				if reject.Load() && !options.Profile.IsNative() {
					return nil, failure
				}
				transport := &appTLSFixtureTransport{profile: options.Profile.CanonicalName()}
				made = append(made, transport)
				return transport, nil
			}
			if scenario == "build" {
				rewriteConfigFile(t, path, tlsAppConfig("http://127.0.0.1:1", "chrome", "")+fmt.Sprintf("\nlogging {\n payload_log {\n enabled = true\n dir = %q\n }\n}\n", payloads))
				app, err := Build(context.Background(), BuildOptions{ConfigPath: path, LogOutput: io.Discard, transportFactory: factory})
				if app != nil || !errors.Is(err, failure) {
					t.Fatalf("Build preparation failure: %v", err)
				}
				if _, err := os.Stat(payloads); !os.IsNotExist(err) || len(made) != 1 || made[0].closes.Load() == 0 {
					t.Fatal("failed preparation started sinks or retained candidate transport")
				}
				return
			}
			app, err := Build(context.Background(), BuildOptions{ConfigPath: path, LogOutput: io.Discard, transportFactory: factory})
			if err != nil {
				t.Fatal(err)
			}
			defer app.Close()
			activeConfig, activePool := app.Config, app.clients
			activeClient := app.handler.SnapshotDependencies().Client
			reject.Store(true)
			rewriteConfigFile(t, path, tlsAppConfig("http://127.0.0.1:1", "safari", "")+fmt.Sprintf("\nlogging {\n payload_log {\n enabled = true\n dir = %q\n }\n}\n", payloads))
			if err := app.Reload(); !errors.Is(err, failure) {
				t.Fatalf("Reload preparation failure: %v", err)
			}
			if app.Config != activeConfig || app.clients != activePool || app.handler.SnapshotDependencies().Client != activeClient || made[0].closes.Load() != 0 || made[1].closes.Load() != 0 {
				t.Fatal("failed preparation changed or retired active dependencies")
			}
			if _, err := os.Stat(payloads); !os.IsNotExist(err) || len(made) != 3 || made[2].closes.Load() == 0 {
				t.Fatal("failed reload started sinks or retained candidate transport")
			}
			if response := resourceChat(app.Server.Handler, "local/m"); response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "chrome-133") {
				t.Fatal("failed reload lost the original provider client")
			}
			reject.Store(false)
			rewriteConfigFile(t, path, tlsAppConfig("http://127.0.0.1:1", "safari", ""))
			if err := app.Reload(); err != nil {
				t.Fatal(err)
			}
			if app.clients == activePool || made[0].closes.Load() == 0 || made[1].closes.Load() == 0 {
				t.Fatal("successful reload did not atomically replace and retire clients")
			}
			if response := resourceChat(app.Server.Handler, "local/m"); response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "safari-16.0") {
				t.Fatal("successful reload did not activate the selected profile")
			}
		})
	}
}

func TestUpstreamPoolCleansFailedFactoryAndRejectsUnresolvedProfiles(t *testing.T) {
	created := &appTLSFixtureTransport{}
	failure := errors.New("factory failure")
	if _, err := prepareUpstreamClientPool(config.Catalog{}, func(upstreamhttp.Options) (upstreamhttp.Transport, error) { return created, failure }); !errors.Is(err, failure) || created.closes.Load() != 1 {
		t.Fatal("partial factory failure did not close its transport")
	}
	if _, err := prepareUpstreamClientPool(config.Catalog{}, func(upstreamhttp.Options) (upstreamhttp.Transport, error) { return nil, nil }); err == nil {
		t.Fatal("missing prepared transport accepted")
	}
	for _, provider := range []config.Provider{
		{Name: "invalid", TLSClientHello: "unsupported"},
		{Name: "invalid", TLSClientHello: "custom", TLSClientHelloSpecFile: "not-read.json"},
		{Name: "invalid", TLSClientHello: "native", TLSClientHelloSpecFile: "not-read.json", TLSProfile: appTestCustomTLSProfile(t, false)},
	} {
		if _, err := newUpstreamClientPool(config.NewCatalog([]config.Provider{provider}, nil, nil)); err == nil || strings.Contains(err.Error(), "not-read.json") {
			t.Fatalf("unresolved/mismatched provider accepted or source echoed: %v", err)
		}
	}
}

func TestTLSRuntimeCloseSerializesCandidateActivationAndPreventsNewGenerations(t *testing.T) {
	path := writeConfigFile(t, tlsAppConfig("http://127.0.0.1:1", "chrome", ""))
	var block atomic.Bool
	entered, proceed := make(chan struct{}), make(chan struct{})
	continueOnce := sync.OnceFunc(func() { close(proceed) })
	defer continueOnce()
	var made []*appTLSFixtureTransport
	factory := func(options upstreamhttp.Options) (upstreamhttp.Transport, error) {
		if block.Load() && options.Profile.IsNative() {
			close(entered)
			<-proceed
		}
		transport := &appTLSFixtureTransport{profile: options.Profile.CanonicalName()}
		made = append(made, transport)
		return transport, nil
	}
	app, err := Build(context.Background(), BuildOptions{ConfigPath: path, LogOutput: io.Discard, transportFactory: factory})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { continueOnce(); _ = app.Close() }()
	rewriteConfigFile(t, path, tlsAppConfig("http://127.0.0.1:1", "safari", ""))
	block.Store(true)
	reloaded := make(chan error, 1)
	go func() { reloaded <- app.Reload() }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("reload did not begin candidate preparation")
	}
	closed := make(chan error, 1)
	go func() { closed <- app.Close() }()
	select {
	case err := <-closed:
		t.Fatalf("Close raced past candidate activation: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	continueOnce()
	if err := <-reloaded; err != nil {
		t.Fatal(err)
	}
	if err := <-closed; err != nil {
		t.Fatal(err)
	}
	for _, transport := range made {
		if transport.closes.Load() == 0 {
			t.Fatal("Close left a concurrently activated generation unretired")
		}
	}
	count := len(made)
	if err := app.Reload(); err == nil || len(made) != count {
		t.Fatal("Reload after Close created a new generation")
	}
}
