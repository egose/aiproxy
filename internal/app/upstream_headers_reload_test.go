package app

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func upstreamHeadersAppConfig(primaryURL, fallbackURL, generation, aliasAttributes string) string {
	var b strings.Builder
	fmt.Fprintf(&b, `
listener "http" "public" { address = "127.0.0.1:0" }
auth "main" { mode = "none" }
logging { access_log = false }
upstream_headers = { X-Fixture-Root = %[1]q, X-Fixture-Scope = %[1]q }
`, generation+"-root")
	for _, target := range []struct{ name, url string }{{"primary", primaryURL}, {"fallback", fallbackURL}} {
		fmt.Fprintf(&b, `
provider "openai-compatible" %q {
  base_url = %q
  api_key = "synthetic-upstream-key"
  upstream_headers = { X-Fixture-Provider = %[3]q, x-fixture-scope = %[3]q, %[4]s = %[3]q }
  model "m" { upstream_headers = { X-Fixture-Model = %[5]q, X-FIXTURE-SCOPE = %[5]q } }
}
`, target.name, target.url, generation+"-"+target.name+"-provider", "X-Fixture-"+target.name, generation+"-"+target.name+"-model")
	}
	fmt.Fprintf(&b, `
alias "a" {
  algorithm = "round_robin"
  retry_status_codes = ["429"]
  providers = ["primary", "fallback"]
  model = "m"
  upstream_headers = {
    X-Fixture-Alias = %[1]q
    x-fixture-SCOPE = %[1]q
%[2]s
  }
}
`, generation+"-alias", aliasAttributes)
	return b.String()
}

func buildUpstreamHeadersApp(t *testing.T, path string) *App {
	t.Helper()
	a, err := Build(t.Context(), BuildOptions{ConfigPath: path, Version: "headers-test", LogOutput: io.Discard})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := a.Close(); err != nil {
			t.Error(err)
		}
	})
	return a
}

type appUpstreamHeadersAttempt struct {
	target  string
	headers http.Header
}

func awaitUpstreamHeadersAttempt(t *testing.T, attempts <-chan appUpstreamHeadersAttempt) appUpstreamHeadersAttempt {
	t.Helper()
	select {
	case attempt := <-attempts:
		return attempt
	case <-time.After(5 * time.Second):
		t.Fatal("stub upstream did not receive the request")
		return appUpstreamHeadersAttempt{}
	}
}

func assertAppUpstreamHeaders(t *testing.T, attempt appUpstreamHeadersAttempt, generation string, viaAlias bool) {
	t.Helper()
	want := http.Header{
		"X-Fixture-Root":     {generation + "-root"},
		"X-Fixture-Provider": {generation + "-" + attempt.target + "-provider"},
		"X-Fixture-Model":    {generation + "-" + attempt.target + "-model"},
		"X-Fixture-Scope":    {generation + "-" + attempt.target + "-model"},
	}
	want.Set("X-Fixture-"+attempt.target, generation+"-"+attempt.target+"-provider")
	if viaAlias {
		want.Set("X-Fixture-Alias", generation+"-alias")
		want.Set("X-Fixture-Scope", generation+"-alias")
	}
	got := make(http.Header)
	for name, values := range attempt.headers {
		if strings.HasPrefix(name, "X-Fixture-") {
			got[name] = values
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s/%s (alias=%t) fixed headers = %q, want %q", generation, attempt.target, viaAlias, got, want)
	}
}

func stubUpstreamHeadersSuccess(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, `{"choices":[]}`)
}

func TestReloadScopedUpstreamHeadersActivatesOrRollsBack(t *testing.T) {
	attempts := make(chan appUpstreamHeadersAttempt, 8)
	upstream := func(name string) string {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			attempts <- appUpstreamHeadersAttempt{name, req.Header.Clone()}
			stubUpstreamHeadersSuccess(w)
		}))
		t.Cleanup(s.Close)
		return s.URL
	}
	primaryURL, fallbackURL := upstream("primary"), upstream("fallback")
	path := writeConfigFile(t, upstreamHeadersAppConfig(primaryURL, fallbackURL, "old", ""))
	a := buildUpstreamHeadersApp(t, path)
	check := func(generation string) {
		t.Helper()
		for _, model := range []string{"primary/m", "alias/a"} {
			w := resourceChat(a.Server.Handler, model)
			if w.Code != http.StatusOK {
				t.Fatalf("%s/%s: status %d: %s", generation, model, w.Code, w.Body.String())
			}
			attempt := awaitUpstreamHeadersAttempt(t, attempts)
			if model == "primary/m" && attempt.target != "primary" {
				t.Fatal("direct inference changed targets")
			}
			assertAppUpstreamHeaders(t, attempt, generation, model == "alias/a")
		}
	}
	check("old")
	rewriteConfigFile(t, path, upstreamHeadersAppConfig(primaryURL, fallbackURL, "new", ""))
	if err := a.Reload(); err != nil {
		t.Fatal(err)
	}
	check("new")
	active := a.Config
	var overBudget strings.Builder
	for i := range 30 {
		fmt.Fprintf(&overBudget, "    X-Fixture-Extra-%d = \"extra\"\n", i)
	}
	rewriteConfigFile(t, path, upstreamHeadersAppConfig(primaryURL, fallbackURL, "rejected", overBudget.String()))
	if err := a.Reload(); err == nil || !strings.Contains(err.Error(), "effective upstream_headers") || a.Config != active {
		t.Fatalf("invalid candidate did not retain the active config: %v", err)
	}
	check("new")
}

func TestReloadScopedUpstreamHeadersKeepsAdmittedAliasSnapshot(t *testing.T) {
	attempts := make(chan appUpstreamHeadersAttempt, 8)
	gate := make(chan struct{})
	release := sync.OnceFunc(func() { close(gate) })
	defer release()
	var primaryCalls atomic.Int32
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		attempts <- appUpstreamHeadersAttempt{"primary", req.Header.Clone()}
		if primaryCalls.Add(1) == 1 {
			select {
			case <-gate:
				http.Error(w, `{"error":"retry target"}`, http.StatusTooManyRequests)
			case <-req.Context().Done():
			}
			return
		}
		stubUpstreamHeadersSuccess(w)
	}))
	t.Cleanup(primary.Close)
	fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		attempts <- appUpstreamHeadersAttempt{"fallback", req.Header.Clone()}
		stubUpstreamHeadersSuccess(w)
	}))
	t.Cleanup(fallback.Close)
	path := writeConfigFile(t, upstreamHeadersAppConfig(primary.URL, fallback.URL, "old", ""))
	a := buildUpstreamHeadersApp(t, path)
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	postAlias := func() *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"alias/a","messages":[]}`))
		a.Server.Handler.ServeHTTP(w, req)
		return w
	}
	finished := make(chan *httptest.ResponseRecorder, 1)
	go func() { finished <- postAlias() }()
	first := awaitUpstreamHeadersAttempt(t, attempts)
	if first.target != "primary" {
		t.Fatal("initial alias attempt did not select the first target")
	}
	assertAppUpstreamHeaders(t, first, "old", true)
	rewriteConfigFile(t, path, upstreamHeadersAppConfig(primary.URL, fallback.URL, "new", ""))
	if err := a.Reload(); err != nil {
		t.Fatal(err)
	}
	if w := postAlias(); w.Code != http.StatusOK {
		t.Fatalf("new alias request failed: %d %s", w.Code, w.Body.String())
	}
	assertAppUpstreamHeaders(t, awaitUpstreamHeadersAttempt(t, attempts), "new", true)
	release()
	select {
	case w := <-finished:
		if w.Code != http.StatusOK {
			t.Fatalf("admitted alias request failed: %d %s", w.Code, w.Body.String())
		}
	case <-ctx.Done():
		t.Fatal("admitted alias request did not finish")
	}
	second := awaitUpstreamHeadersAttempt(t, attempts)
	if second.target != "fallback" {
		t.Fatal("admitted request did not fail over to its next target")
	}
	assertAppUpstreamHeaders(t, second, "old", true)
	if len(attempts) != 0 {
		t.Fatal("unexpected extra upstream attempts")
	}
}

func TestUpstreamHeadersAreNotSentOnHealthProbes(t *testing.T) {
	probe := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet || req.URL.Path != "/health" || req.Header.Get("Authorization") != "Bearer sk-test" {
			t.Errorf("unexpected configured health probe: %s %s", req.Method, req.URL.Path)
		}
		for name := range req.Header {
			if strings.HasPrefix(name, "X-Fixture-") {
				t.Errorf("fixed inference header %s reached a health probe", name)
			}
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(probe.Close)
	source := `upstream_headers = { X-Fixture-Root = "root-value" }` + healthcheckTestConfig(probe.URL, "    send_authorization = true\n")
	source = strings.Replace(source, `model "m" {}`, `upstream_headers = { X-Fixture-Provider = "provider-value" }
  model "m" { upstream_headers = { X-Fixture-Model = "model-value" } }`, 1)
	a := buildUpstreamHeadersApp(t, writeConfigFile(t, source))
	waitForHealthcheck(t, a, true)
}
