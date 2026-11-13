package httpapi

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/auth"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/modelresolver"
	"github.com/egose/aiproxy/internal/provider"
	"github.com/egose/aiproxy/internal/tlsprofile"
)

type tlsSelectionAdapter struct {
	mu       sync.Mutex
	requests []provider.Request
	entered  chan struct{}
	release  chan struct{}
	once     sync.Once
}

func (a *tlsSelectionAdapter) Do(ctx context.Context, req provider.Request) (*provider.Result, error) {
	a.mu.Lock()
	a.requests = append(a.requests, req)
	a.mu.Unlock()
	status := http.StatusOK
	if req.APIKey == "synthetic-primary-old" {
		a.once.Do(func() { close(a.entered) })
		select {
		case <-a.release:
			status = http.StatusServiceUnavailable
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return &provider.Result{StatusCode: status, Header: http.Header{"Content-Type": {"application/json"}}, Body: []byte(`{"choices":[]}`)}, nil
}

func TestTLSClientSelectionPinsDirectAndAliasDependencyGeneration(t *testing.T) {
	adapter := &tlsSelectionAdapter{entered: make(chan struct{}), release: make(chan struct{})}
	release := sync.OnceFunc(func() { close(adapter.release) })
	defer release()
	oldPrimary, oldBackup, newPrimary, newBackup := &http.Client{}, &http.Client{}, &http.Client{}, &http.Client{}
	dependencies := func(next bool) Dependencies {
		primarySelector, backupSelector, suffix := "chrome", "safari", "old"
		primaryClient, backupClient := oldPrimary, oldBackup
		if next {
			primarySelector, backupSelector, suffix = "safari", "chrome", "new"
			primaryClient, backupClient = newPrimary, newBackup
		}
		model := config.Model{Name: "m", UpstreamName: "upstream-m", Capabilities: []config.Capability{config.CapabilityChat}}
		primary := config.Provider{Type: config.ProviderTypeOpenAICompatible, Name: "primary", APIKey: "synthetic-primary-" + suffix, TLSClientHello: primarySelector, UpstreamHeaderTimeout: config.DefaultUpstreamHeaderTimeout, Models: []config.Model{model}}
		backup := primary
		backup.Name, backup.APIKey, backup.TLSClientHello = "backup", "synthetic-backup-"+suffix, backupSelector
		runtime := &config.Runtime{Catalog: config.NewCatalog([]config.Provider{primary, backup}, nil, []config.Alias{{Name: "chat", Algorithm: config.AlgorithmRoundRobin, RetryStatusCodes: []int{503}, Targets: []config.AliasTarget{{Provider: "primary", Model: "m"}, {Provider: "backup", Model: "m"}}}})}
		return Dependencies{
			Resolver: modelresolver.New(runtime), Catalog: runtime.Catalog, Adapter: adapter,
			Auth:   auth.NewAuthenticator(config.Auth{Mode: config.AuthModeNone}),
			Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
			ClientForProvider: func(p config.Provider) *http.Client {
				selector, client := primarySelector, primaryClient
				if p.Name == "backup" {
					selector, client = backupSelector, backupClient
				}
				profile, _ := tlsprofile.ResolveBuiltin(selector)
				if p.TLSProfile.Identity() != profile.Identity() || p.UpstreamHeaderTimeout != config.DefaultUpstreamHeaderTimeout || p.APIKey != "synthetic-"+p.Name+"-"+suffix {
					t.Error("client selection crossed effective profile/credential generation")
				}
				return client
			},
		}
	}
	handler := NewHandler(dependencies(false))
	request := func(model string) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"`+model+`","messages":[]}`)))
		return response
	}
	finished := make(chan *httptest.ResponseRecorder, 1)
	go func() { finished <- request("alias/chat") }()
	select {
	case <-adapter.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("alias did not begin")
	}
	handler.UpdateDependencies(dependencies(true))
	if response := request("backup/m"); response.Code != http.StatusOK {
		t.Fatal(response.Body.String())
	}
	release()
	select {
	case response := <-finished:
		if response.Code != http.StatusOK {
			t.Fatal(response.Body.String())
		}
	case <-time.After(3 * time.Second):
		t.Fatal("admitted alias retry did not finish")
	}
	adapter.mu.Lock()
	defer adapter.mu.Unlock()
	if len(adapter.requests) != 3 || adapter.requests[0].Client != oldPrimary || adapter.requests[1].Client != newBackup || adapter.requests[2].Client != oldBackup {
		t.Fatal("direct/alias attempts did not retain their dependency generation")
	}
}
