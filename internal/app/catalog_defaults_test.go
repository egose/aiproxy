package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestCatalogProviderRootDefaultsRoundTrip(t *testing.T) {
	a, path, cfg, request := catalogRuntimeFixture(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(r.Header)
	}))
	t.Cleanup(upstream.Close)
	cfg = strings.Replace(cfg, `api_key = "fixture"`, "api_key = \"fixture\"\n user_agent = \"base/1\"\n upstream_header_timeout = \"19s\"\n forward_headers = [\"x-base\"]", 1)
	setRoots := func(agent string, seconds int, forward bool, headers string) {
		t.Helper()
		rewriteConfigFile(t, path, fmt.Sprintf("user_agent = %q\nupstream_header_timeout = %q\nforward_user_agent = %v\nforward_headers = %s\n%s", agent, fmt.Sprintf("%ds", seconds), forward, headers, cfg))
		if err := a.Reload(); err != nil {
			t.Fatal(err)
		}
	}
	check := func(app *App, agent string, seconds int, forward bool, headers []string, wireAgent string) {
		t.Helper()
		p, ok := app.Config.Catalog.Provider("dynamic")
		if !ok || p.UserAgent != agent || p.UpstreamHeaderTimeout != time.Duration(seconds)*time.Second || p.ForwardUserAgent != forward || !reflect.DeepEqual(p.ForwardHeaders, headers) {
			t.Fatalf("effective defaults: present=%v agent=%q timeout=%s forwarding=%v headers=%v", ok, p.UserAgent, p.UpstreamHeaderTimeout, p.ForwardUserAgent, p.ForwardHeaders)
		}
		r := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"dynamic/m","messages":[]}`))
		r.Header.Set("Authorization", "Bearer static-fixture")
		r.Header.Set("User-Agent", "caller/1")
		for _, header := range []string{"x-root", "x-shared", "x-local", "x-next", "x-unlisted"} {
			r.Header.Set(header, "caller-value")
		}
		w := httptest.NewRecorder()
		app.Server.Handler.ServeHTTP(w, r)
		var got http.Header
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &got) != nil {
			t.Fatalf("inference = %d %s", w.Code, w.Body.String())
		}
		if got.Get("User-Agent") != wireAgent || got.Get("Authorization") != "Bearer fixture-secret" {
			t.Fatal("upstream user-agent or untouched credential changed")
		}
		for _, header := range []string{"x-root", "x-shared", "x-local", "x-next", "x-unlisted"} {
			want := ""
			for _, allowed := range headers {
				if strings.EqualFold(header, allowed) {
					want = "caller-value"
				}
			}
			if got.Get(header) != want {
				t.Errorf("upstream %s = %q, want %q", header, got.Get(header), want)
			}
		}
	}
	setRoots("root/1", 37, true, `["x-root", "x-shared"]`)
	body := fmt.Sprintf(`{"name":"dynamic","type":"openai-compatible","base_url":%q,"api_key":"fixture-secret","models":[{"name":"m"}]}`, upstream.URL)
	if w := request("POST", "/_internal/admin/providers", body); w.Code != 201 {
		t.Fatalf("create = %d %s", w.Code, w.Body.String())
	}
	check(a, "root/1", 37, true, []string{"x-root", "x-shared"}, "root/1")
	before, err := a.adminStore.GetProvider(context.Background(), "dynamic")
	if err != nil {
		t.Fatal(err)
	}
	if w := request("PUT", "/_internal/admin/providers/dynamic", `{"user_agent":"local/1","upstream_header_timeout":"11s","forward_user_agent":true,"forward_headers":["X-Shared","x-local"]}`); w.Code != 200 {
		t.Fatalf("override = %d %s", w.Code, w.Body.String())
	}
	check(a, "local/1", 11, true, []string{"x-root", "x-shared", "x-local"}, "local/1")
	if w := request("PUT", "/_internal/admin/providers/dynamic", `{"user_agent":"","upstream_header_timeout":"","forward_user_agent":false,"forward_headers":[]}`); w.Code != 200 {
		t.Fatalf("reset = %d %s", w.Code, w.Body.String())
	}
	check(a, "root/1", 37, true, []string{"x-root", "x-shared"}, "root/1")
	after, err := a.adminStore.GetProvider(context.Background(), "dynamic")
	if err != nil {
		t.Fatal(err)
	}
	if after.UserAgent != "" || after.UpstreamTimeoutMs != nil || after.ForwardUserAgent || len(after.ForwardHeaders) != 0 || !reflect.DeepEqual(before.APIKeyEncrypted, after.APIKeyEncrypted) {
		t.Fatal("reset persisted effective defaults or changed untouched credential")
	}
	w := request("GET", "/_internal/admin/providers/dynamic", "")
	var view map[string]interface{}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &view) != nil || view["forward_user_agent"] != false || view["user_agent"] != nil || view["upstream_header_timeout"] != nil || view["forward_headers"] != nil {
		t.Fatalf("local reopened view = %d %s", w.Code, w.Body.String())
	}
	setRoots("", 49, true, `["x-next"]`)
	check(a, "", 49, true, []string{"x-next"}, "caller/1")
	if w := request("POST", "/_internal/admin/providers", `{"name":"derived","type":"openai-compatible","extends":"local","api_key":"derived-secret","enabled":false}`); w.Code != 201 {
		t.Fatalf("inherited create = %d %s", w.Code, w.Body.String())
	}
	checkDerived := func(app *App) {
		t.Helper()
		for _, p := range app.Config.Catalog.DisabledProviders() {
			if p.Name == "derived" {
				if p.Enabled || p.UserAgent != "base/1" || p.UpstreamHeaderTimeout != 19*time.Second || p.APIKey != "derived-secret" || len(p.Models) != 2 || !reflect.DeepEqual(p.ForwardHeaders, []string{"x-next", "x-base"}) {
					t.Fatal("inherited settings, local enabled or local credential changed")
				}
				return
			}
		}
		t.Fatal("disabled derived provider missing")
	}
	checkDerived(a)
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	restarted, err := Build(context.Background(), BuildOptions{ConfigPath: path, LogOutput: io.Discard})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = restarted.Close() })
	check(restarted, "", 49, true, []string{"x-next"}, "caller/1")
	checkDerived(restarted)
	rewriteConfigFile(t, path, "user_agent = \"root/2\"\nupstream_header_timeout = \"53s\"\nforward_user_agent = false\nforward_headers = []\n"+cfg)
	if err := restarted.Reload(); err != nil {
		t.Fatal(err)
	}
	check(restarted, "root/2", 53, false, []string{}, "root/2")
}
