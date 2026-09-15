package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/auth"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/dashrpc"
	"github.com/egose/aiproxy/internal/guardrails"
	"github.com/egose/aiproxy/internal/modelresolver"
	"github.com/egose/aiproxy/internal/observability"
)

func quarantineTestDeps(t *testing.T, rt *config.Runtime, adapter *countingAdapter, q *guardrails.Quarantine) (http.Handler, *guardrails.Quarantine) {
	t.Helper()
	if q == nil {
		q = guardrails.NewQuarantine(guardrails.QuarantinePolicy{Enabled: true})
	}
	rt.Listener = config.Listener{Address: ":8080"}
	dashboard := dashrpc.NewRuntimeSource(config.Dashboard{Token: dashboardTestToken, Enabled: true}, "test", rt.Listener.Address, string(rt.Auth.Mode), time.Now(), rt.Catalog, nil, nil, nil)
	dashboard.SetBlockSource(quarantineTestAdapter{q})
	h := NewHandler(Dependencies{
		Resolver:   modelresolver.New(rt),
		Adapter:    adapter,
		Auth:       auth.NewAuthenticator(config.Auth{Mode: config.AuthModeNone}),
		Catalog:    rt.Catalog,
		Metrics:    observability.NewMetrics(),
		Guardrails: guardrailTestScanner(t, guardrails.ModeBlock, 0, 0),
		Quarantine: q,
		Dashboard:  dashboard,
	})
	return h, q
}

type quarantineTestAdapter struct {
	q *guardrails.Quarantine
}

func (a quarantineTestAdapter) ListBlocks() []dashrpc.BlockSummary {
	var out []dashrpc.BlockSummary
	for _, s := range a.q.List() {
		out = append(out, dashrpc.BlockSummary{
			BlockID:      s.BlockID,
			Operation:    s.Operation,
			PublicModel:  s.PublicModel,
			RuleIDs:      s.RuleIDs,
			FindingCount: s.FindingCount,
		})
	}
	return out
}

func (a quarantineTestAdapter) TakeBlock(blockID string) (dashrpc.BlockCapture, bool) {
	c, ok := a.q.Take(blockID)
	if !ok {
		return dashrpc.BlockCapture{}, false
	}
	out := dashrpc.BlockCapture{BlockID: c.BlockID, Operation: c.Operation, PublicModel: c.PublicModel, RuleIDs: c.RuleIDs}
	for _, f := range c.Findings {
		out.Findings = append(out.Findings, dashrpc.BlockFinding{RuleID: f.RuleID, Secret: f.Secret, Match: f.Match, Line: f.Line})
	}
	return out, true
}

func guardrailErrorBlockID(t *testing.T, body string) string {
	t.Helper()
	var e struct {
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
			BlockID string `json:"block_id"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(body), &e); err != nil {
		t.Fatalf("decode error body %q: %v", body, err)
	}
	return e.Error.BlockID
}

func TestQuarantineBlockStoresCaptureAndReturnsBlockID(t *testing.T) {
	rt := newRT()
	adapter := &countingAdapter{}
	h, q := quarantineTestDeps(t, rt, adapter, nil)
	key := guardrailTestKey(t)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(guardrailChatBody("openai/gpt-4o-mini", "deploy with "+key)))
	r.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	if got := guardrailErrorType(t, w.Body.String()); got != "secret_blocked" {
		t.Fatalf("error type = %q, want secret_blocked", got)
	}
	blockID := guardrailErrorBlockID(t, w.Body.String())
	if blockID == "" {
		t.Fatalf("blocked response carries no block_id")
	}
	if !strings.HasPrefix(blockID, "blk_") {
		t.Fatalf("block_id = %q, want blk_ prefix", blockID)
	}
	if strings.Contains(w.Body.String(), key) {
		t.Fatalf("error body leaks secret text")
	}
	if n := adapter.count(); n != 0 {
		t.Fatalf("upstream calls = %d, want 0", n)
	}
	if q.Len() != 1 {
		t.Fatalf("quarantine len = %d, want 1", q.Len())
	}
}

func TestQuarantineDisabledOmitsBlockID(t *testing.T) {
	rt := newRT()
	adapter := &countingAdapter{}
	h := guardrailTestHandler(t, rt, adapter, guardrailTestScanner(t, guardrails.ModeBlock, 0, 0))
	key := guardrailTestKey(t)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(guardrailChatBody("openai/gpt-4o-mini", "deploy with "+key)))
	r.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	if got := guardrailErrorBlockID(t, w.Body.String()); got != "" {
		t.Fatalf("block_id = %q, want empty when quarantine disabled", got)
	}
}

func TestQuarantineIncompleteCarriesNoBlockID(t *testing.T) {
	rt := newRT()
	adapter := &countingAdapter{}
	h, _ := quarantineTestDeps(t, rt, adapter, nil)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":`))
	r.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	if got := guardrailErrorBlockID(t, w.Body.String()); got != "" {
		t.Fatalf("block_id = %q, want empty for incomplete scans", got)
	}
}

func TestDashboardBlocksListAndTakeOnce(t *testing.T) {
	rt := newRT()
	adapter := &countingAdapter{}
	h, _ := quarantineTestDeps(t, rt, adapter, nil)
	key := guardrailTestKey(t)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(guardrailChatBody("openai/gpt-4o-mini", "deploy with "+key)))
	r.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, r)
	blockID := guardrailErrorBlockID(t, w.Body.String())
	if blockID == "" {
		t.Fatalf("no block_id to look up")
	}

	authed := func(path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set(dashrpc.AuthHeaderName, "Bearer "+dashboardTestToken)
		h.ServeHTTP(rec, req)
		return rec
	}

	lw := authed(dashrpc.BlocksPath)
	if lw.Code != http.StatusOK {
		t.Fatalf("blocks status = %d, body=%s", lw.Code, lw.Body.String())
	}
	var list dashrpc.BlockList
	if err := json.Unmarshal(lw.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if !list.Enabled || len(list.Blocks) != 1 {
		t.Fatalf("list = %+v, want enabled with 1 block", list)
	}
	if list.Blocks[0].BlockID != blockID {
		t.Fatalf("list block = %q, want %q", list.Blocks[0].BlockID, blockID)
	}
	listRaw, _ := json.Marshal(list)
	if strings.Contains(string(listRaw), key) {
		t.Fatalf("block list leaks secret text")
	}

	dw := authed(dashrpc.BlockPathPrefix + blockID)
	if dw.Code != http.StatusOK {
		t.Fatalf("detail status = %d, body=%s", dw.Code, dw.Body.String())
	}
	var capture dashrpc.BlockCapture
	if err := json.Unmarshal(dw.Body.Bytes(), &capture); err != nil {
		t.Fatalf("decode detail: %v", err)
	}
	if capture.BlockID != blockID || len(capture.Findings) == 0 {
		t.Fatalf("capture = %+v", capture)
	}
	if !strings.Contains(dw.Body.String(), key) {
		t.Fatalf("detail should contain the captured secret for triage, body=%s", dw.Body.String())
	}

	again := authed(dashrpc.BlockPathPrefix + blockID)
	if again.Code != http.StatusNotFound {
		t.Fatalf("second take status = %d, want 404 (take-once)", again.Code)
	}
	empty := authed(dashrpc.BlocksPath)
	var after dashrpc.BlockList
	if err := json.Unmarshal(empty.Body.Bytes(), &after); err != nil {
		t.Fatalf("decode after: %v", err)
	}
	if len(after.Blocks) != 0 {
		t.Fatalf("blocks after take = %+v, want empty", after.Blocks)
	}
}

func TestDashboardBlocksAuthAndValidation(t *testing.T) {
	rt := newRT()
	adapter := &countingAdapter{}
	h, _ := quarantineTestDeps(t, rt, adapter, nil)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, dashrpc.BlocksPath, nil)
	h.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unauth status = %d, want 401", w.Code)
	}

	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodGet, dashrpc.BlockPathPrefix+"not-a-block", nil)
	r.Header.Set(dashrpc.AuthHeaderName, "Bearer "+dashboardTestToken)
	h.ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bad id status = %d, want 400", w.Code)
	}

	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodGet, dashrpc.BlockPathPrefix+"blk_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", nil)
	r.Header.Set(dashrpc.AuthHeaderName, "Bearer "+dashboardTestToken)
	h.ServeHTTP(w, r)
	if w.Code != http.StatusNotFound {
		t.Fatalf("absent status = %d, want 404", w.Code)
	}
}

func TestDashboardBlocksDisabled(t *testing.T) {
	rt := newRT()
	rt.Listener = config.Listener{Address: ":8080"}
	h := NewHandler(newDashboardDeps(rt, time.Now(), nil, nil, nil))
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, dashrpc.BlocksPath, nil)
	r.Header.Set(dashrpc.AuthHeaderName, "Bearer "+dashboardTestToken)
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var list dashrpc.BlockList
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if list.Enabled || len(list.Blocks) != 0 {
		t.Fatalf("list = %+v, want disabled empty", list)
	}
}
