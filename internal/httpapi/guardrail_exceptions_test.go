package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/accounting"
	"github.com/egose/aiproxy/internal/auth"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/dashrpc"
	"github.com/egose/aiproxy/internal/guardrails"
	"github.com/egose/aiproxy/internal/modelresolver"
	"github.com/egose/aiproxy/internal/observability"
	"github.com/egose/aiproxy/internal/providerhealth"
)

func exceptionTestDeps(t *testing.T, rt *config.Runtime, adapter *countingAdapter, scanner *guardrails.Scanner, q *guardrails.Quarantine, exc *guardrails.Exceptions) Dependencies {
	t.Helper()
	return Dependencies{
		Resolver:   modelresolver.New(rt),
		Adapter:    adapter,
		Auth:       auth.NewAuthenticator(config.Auth{Mode: config.AuthModeNone}),
		Catalog:    rt.Catalog,
		Metrics:    observability.NewMetrics(),
		Guardrails: scanner,
		Quarantine: q,
		Exceptions: exc,
	}
}

func exceptionTestStore(t *testing.T) *guardrails.Exceptions {
	t.Helper()
	exc, err := guardrails.LoadExceptions(filepath.Join(t.TempDir(), "exceptions.json"), "")
	if err != nil {
		t.Fatalf("load exceptions: %v", err)
	}
	return exc
}

func serveChat(t *testing.T, h http.Handler, content string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(guardrailChatBody("openai/gpt-4o-mini", content)))
	h.ServeHTTP(w, r)
	return w
}

func blockFindingSHAs(t *testing.T, q *guardrails.Quarantine, blockID string) []string {
	t.Helper()
	capture, ok := q.Take(blockID)
	if !ok {
		t.Fatalf("quarantine take missed %q", blockID)
	}
	var shas []string
	for _, f := range capture.Findings {
		sha := f.SecretSHA
		if sha == "" {
			sha = guardrails.Fingerprint(f.Secret)
		}
		shas = append(shas, sha)
	}
	if len(shas) == 0 {
		t.Fatalf("capture has no findings")
	}
	return shas
}

func blockIDOf(t *testing.T, body string) string {
	t.Helper()
	var e struct {
		Error struct {
			BlockID string `json:"block_id"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(body), &e); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if e.Error.BlockID == "" {
		t.Fatalf("response has no block_id: %s", body)
	}
	return e.Error.BlockID
}

func TestGuardrailAllowExceptionForwards(t *testing.T) {
	rt := newRT()
	adapter := &countingAdapter{}
	scanner := guardrailTestScanner(t, guardrails.ModeBlock, 0, 0)
	q := guardrails.NewQuarantine(guardrails.QuarantinePolicy{Enabled: true})
	exc := exceptionTestStore(t)
	h := NewHandler(exceptionTestDeps(t, rt, adapter, scanner, q, exc))
	key := guardrailTestKey(t)
	w := serveChat(t, h, "deploy with "+key)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("first request status = %d, want 400", w.Code)
	}
	shas := blockFindingSHAs(t, q, blockIDOf(t, w.Body.String()))
	if _, err := exc.Decide(shas, guardrails.ExceptionActionAllow, "test", nil); err != nil {
		t.Fatalf("decide: %v", err)
	}
	w = serveChat(t, h, "deploy with "+key)
	if w.Code != http.StatusOK {
		t.Fatalf("allowed request status = %d, want 200: %s", w.Code, w.Body.String())
	}
	if adapter.count() != 1 {
		t.Fatalf("upstream calls = %d, want 1", adapter.count())
	}
}

func TestGuardrailRedactExceptionRewritesBody(t *testing.T) {
	rt := newRT()
	adapter := &countingAdapter{}
	scanner := guardrailTestScanner(t, guardrails.ModeBlock, 0, 0)
	q := guardrails.NewQuarantine(guardrails.QuarantinePolicy{Enabled: true})
	exc := exceptionTestStore(t)
	h := NewHandler(exceptionTestDeps(t, rt, adapter, scanner, q, exc))
	key := guardrailTestKey(t)
	w := serveChat(t, h, "deploy with "+key)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("first request status = %d, want 400", w.Code)
	}
	shas := blockFindingSHAs(t, q, blockIDOf(t, w.Body.String()))
	if _, err := exc.Decide(shas, guardrails.ExceptionActionRedact, "test", nil); err != nil {
		t.Fatalf("decide: %v", err)
	}
	w = serveChat(t, h, "deploy with "+key)
	if w.Code != http.StatusOK {
		t.Fatalf("redacted request status = %d, want 200: %s", w.Code, w.Body.String())
	}
	sent := string(adapter.lastBody())
	if strings.Contains(sent, key) {
		t.Fatalf("upstream body still contains the secret")
	}
	if !strings.Contains(sent, guardrails.DefaultRedactPlaceholder) {
		t.Fatalf("upstream body missing placeholder: %s", sent)
	}
}

func TestGuardrailDenyExceptionKeepsBlocking(t *testing.T) {
	rt := newRT()
	adapter := &countingAdapter{}
	scanner := guardrailTestScanner(t, guardrails.ModeBlock, 0, 0)
	q := guardrails.NewQuarantine(guardrails.QuarantinePolicy{Enabled: true})
	exc := exceptionTestStore(t)
	h := NewHandler(exceptionTestDeps(t, rt, adapter, scanner, q, exc))
	key := guardrailTestKey(t)
	w := serveChat(t, h, "deploy with "+key)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("first request status = %d, want 400", w.Code)
	}
	shas := blockFindingSHAs(t, q, blockIDOf(t, w.Body.String()))
	if _, err := exc.Decide(shas, guardrails.ExceptionActionDeny, "test", nil); err != nil {
		t.Fatalf("decide: %v", err)
	}
	w = serveChat(t, h, "deploy with "+key)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("denied request status = %d, want 400", w.Code)
	}
	if got := guardrailErrorType(t, w.Body.String()); got != "secret_blocked" {
		t.Fatalf("error type = %q, want secret_blocked", got)
	}
	if adapter.count() != 0 {
		t.Fatalf("upstream calls = %d, want 0", adapter.count())
	}
}

func TestBlockDecisionEndpoint(t *testing.T) {
	rt := newRT()
	rt.Listener = config.Listener{Address: ":8080"}
	usage := accounting.NewAggregator()
	health := providerhealth.New(nil, config.ProviderHealth{})
	health.SetProviders(rt.Catalog)
	logs := observability.NewLogBuffer(10)
	deps := newDashboardDeps(rt, time.Now(), usage, health, logs)
	exc := exceptionTestStore(t)
	deps.Exceptions = exc
	h := NewHandler(deps)
	sha := guardrails.Fingerprint("sk-test-value")
	body := `{"action":"allow","finding_shas":["` + sha + `"]}`
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, dashrpc.BlockDecisionPath("blk_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), strings.NewReader(body))
	r.Header.Set(dashrpc.AuthHeaderName, dashrpc.AuthScheme+dashboardTestToken)
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("decision status = %d, want 200: %s", w.Code, w.Body.String())
	}
	if entry, ok := exc.Lookup(sha); !ok || entry.Action != guardrails.ExceptionActionAllow {
		t.Fatalf("exception not recorded: %+v %v", entry, ok)
	}
	raw, err := os.ReadFile(exc.Path())
	if err != nil {
		t.Fatalf("read exceptions file: %v", err)
	}
	if strings.Contains(string(raw), "sk-test-value") {
		t.Fatalf("exceptions file retains raw secret text")
	}
	for _, tc := range []struct {
		name   string
		path   string
		body   string
		header string
		want   int
	}{
		{"bad-action", dashrpc.BlockDecisionPath("blk_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), `{"action":"quarantine","finding_shas":["` + sha + `"]}`, dashrpc.AuthScheme + dashboardTestToken, http.StatusBadRequest},
		{"bad-sha", dashrpc.BlockDecisionPath("blk_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), `{"action":"allow","finding_shas":["xyz"]}`, dashrpc.AuthScheme + dashboardTestToken, http.StatusBadRequest},
		{"bad-block", dashrpc.BlockDecisionPath("nope"), `{"action":"allow","finding_shas":["` + sha + `"]}`, dashrpc.AuthScheme + dashboardTestToken, http.StatusBadRequest},
		{"unauth", dashrpc.BlockDecisionPath("blk_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), body, "Bearer wrong", http.StatusUnauthorized},
		{"get-not-allowed", dashrpc.BlockDecisionPath("blk_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), "", dashrpc.AuthScheme + dashboardTestToken, http.StatusMethodNotAllowed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			method := http.MethodPost
			var reader *strings.Reader
			if tc.name == "get-not-allowed" {
				method = http.MethodGet
				reader = strings.NewReader("")
			} else {
				reader = strings.NewReader(tc.body)
			}
			w := httptest.NewRecorder()
			r := httptest.NewRequest(method, tc.path, reader)
			if tc.header != "" {
				r.Header.Set(dashrpc.AuthHeaderName, tc.header)
			}
			h.ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatalf("status = %d, want %d: %s", w.Code, tc.want, w.Body.String())
			}
		})
	}
}
