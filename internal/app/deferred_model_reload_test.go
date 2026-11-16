package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/httpapi"
)

func callerModelReloadConfig(next bool) string {
	base, key, first, second := "http://127.0.0.1:1/v1", "old-peer-token", "", `
    deferred_transport {
      control_timeout = "200ms"
      max_wait = "3s"
      poll_interval = "50ms"
      max_control_calls = 4096
      max_control_retries = 8
    }`
	if next {
		base, key, first, second = "http://127.0.0.1:2/v1", "next-peer-token", `
    deferred_transport {
      control_timeout = "100ms"
      max_wait = "1s"
      poll_interval = "50ms"
      max_control_calls = 4
      max_control_retries = 0
    }`, ""
	}
	return fmt.Sprintf(`
listener "http" "public" { address = "127.0.0.1:0" }
auth "main" { mode = "none" }
logging { access_log = false }
provider "openai-compatible" "a" {
  base_url = %q
  api_key = %q
  model "model-1" {
    upstream_name = "p/model-1"
    %s
  }
  model "model-2" {
    upstream_name = "p/model-2"
    %s
  }
}
alias "mixed" {
  algorithm = "round_robin"
  target {
    provider = "a"
    model = "model-1"
  }
  target {
    provider = "a"
    model = "model-2"
  }
}
`, base, key, first, second)
}

func assertCallerModelReloadPolicy(t *testing.T, deps httpapi.Dependencies, next bool) {
	t.Helper()
	base, key, optedIn := "http://127.0.0.1:1/v1", "old-peer-token", "model-2"
	options := config.DefaultDeferredTransport()
	options.ControlTimeout, options.MaxWait, options.PollInterval = 200*time.Millisecond, 3*time.Second, 50*time.Millisecond
	if next {
		base, key, optedIn = "http://127.0.0.1:2/v1", "next-peer-token", "model-1"
		options.ControlTimeout, options.MaxWait, options.MaxControlCalls, options.MaxControlRetries = 100*time.Millisecond, time.Second, 4, 0
	}
	for _, name := range []string{"model-1", "model-2"} {
		p, model, ok := deps.Catalog.Model("a", name)
		resolved, err := deps.Resolver.Resolve("a/" + name)
		if !ok || err != nil || p.BaseURL != base || p.APIKey != key || p.DeferredTransport != nil ||
			model.UpstreamName != "p/"+name || resolved.Provider.BaseURL != base || resolved.Provider.APIKey != key {
			t.Fatalf("reload policy snapshot lost catalog identity: model=%s provider=%+v err=%v", name, p, err)
		}
		for _, effective := range []*config.DeferredTransport{
			config.EffectiveDeferredTransport(p, model),
			config.EffectiveDeferredTransport(resolved.Provider, resolved.Model),
		} {
			if name == optedIn {
				if effective == nil || *effective != options {
					t.Fatalf("model %s options=%+v, want %+v", name, effective, options)
				}
			} else if effective != nil {
				t.Fatalf("ordinary model %s inherited caller transport: %+v", name, effective)
			}
		}
	}
}

func TestDeferredCallerModelPolicyReloadPreservesLeasedSnapshot(t *testing.T) {
	path := writeConfigFile(t, callerModelReloadConfig(false))
	a, err := Build(context.Background(), BuildOptions{ConfigPath: path, LogOutput: io.Discard})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Close() })
	if a.DeferredStore() != nil {
		t.Fatal("caller model opt-in allocated a receiver store")
	}
	old, release, ok := a.handler.SnapshotDependenciesWithLease()
	if !ok {
		t.Fatal("old caller model policy was unavailable")
	}
	defer release()
	assertCallerModelReloadPolicy(t, old, false)
	oldProvider, _ := old.Catalog.Provider("a")
	oldClient, oldConfig := old.ClientForProvider(oldProvider), a.Config
	rewriteConfigFile(t, path, callerModelReloadConfig(true)+"\ninvalid {")
	if err := a.Reload(); err == nil {
		t.Fatal("invalid model policy activated")
	}
	current := a.handler.SnapshotDependencies()
	if a.Config != oldConfig || current.RuntimeResources != old.RuntimeResources || current.ClientForProvider(oldProvider) != oldClient {
		t.Fatal("failed reload changed the caller snapshot or owned resources")
	}
	assertCallerModelReloadPolicy(t, current, false)
	rewriteConfigFile(t, path, callerModelReloadConfig(true))
	if err := a.Reload(); err != nil {
		t.Fatal(err)
	}
	current = a.handler.SnapshotDependencies()
	assertCallerModelReloadPolicy(t, current, true)
	assertCallerModelReloadPolicy(t, old, false)
	currentProvider, _ := current.Catalog.Provider("a")
	if current.RuntimeResources == old.RuntimeResources || current.ClientForProvider(currentProvider) == oldClient || a.DeferredStore() != nil {
		t.Fatal("model policy reload reused the old transport generation or added receiver resources")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	err = old.RuntimeResources.Wait(ctx)
	cancel()
	if !errors.Is(err, httpapi.ErrResourcesIncomplete) {
		t.Fatalf("reload retired a leased caller snapshot early: %v", err)
	}
	release()
	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := old.RuntimeResources.Wait(ctx); err != nil {
		t.Fatal(err)
	}
}
