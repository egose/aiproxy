package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/dbmerge"
	"github.com/egose/aiproxy/internal/store"
	"github.com/egose/aiproxy/internal/upstreamhttp"
)

func TestTwoAppsServingReconciliation(t *testing.T) {
	st, dsn := workspaceInferenceDB(t)
	ctx := context.Background()
	var calls atomic.Int64
	started, release := make(chan struct{}, 1), make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), "hold") {
			if strings.Contains(string(body), `"stream":true`) {
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, "data: {\"id\":\"stream\",\"choices\":[]}\n\n")
				w.(http.Flusher).Flush()
				started <- struct{}{}
				<-release
				fmt.Fprint(w, "data: [DONE]\n\n")
				return
			}
			started <- struct{}{}
			<-release
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"id":"mock","choices":[{"message":{"role":"assistant","content":%q}}]}`, string(body))
	}))
	t.Cleanup(upstream.Close)
	encrypted, err := store.EncryptSecret([]byte("fixture-upstream"))
	if err != nil {
		t.Fatal(err)
	}
	p := store.DBProvider{Name: "managed", Type: "openai-compatible", BaseURL: upstream.URL, APIKeyEncrypted: encrypted, Enabled: true, WorkspaceID: store.SystemWorkspaceID}
	models := []store.DBProviderModel{{Name: "m", UpstreamName: "v1"}}
	if err := st.CreateProviderAggregate(ctx, &p, models); err != nil {
		t.Fatal(err)
	}
	alias := store.DBAlias{Name: "route", Algorithm: "round_robin", WorkspaceID: store.SystemWorkspaceID}
	if err := st.CreateAlias(ctx, &alias, []store.DBAliasTarget{{Provider: "managed", Model: "m"}}); err != nil {
		t.Fatal(err)
	}
	key := store.InboundKey{Name: "db", TokenHash: store.TokenHash("db-token"), TokenPrefix: "db", Enabled: true, WorkspaceID: store.SystemWorkspaceID}
	if err := st.CreateInboundKey(ctx, &key); err != nil {
		t.Fatal(err)
	}
	cfg := fmt.Sprintf(`listener "http" "public" { address = "127.0.0.1:0" }
auth "bearer_static" {
  mode = "bearer_static"
  client "static" { token = "static-token" }
}

provider "openai-compatible" "local" {
  base_url = %q
  api_key = "fixture"
  model "m" { upstream_name = "local" }
}
database { url = %q }
multi_tenancy { enabled = true }
`, upstream.URL, dsn)
	path := writeConfigFile(t, cfg)
	build := func() (*App, *httptest.Server) {
		a, err := Build(ctx, BuildOptions{ConfigPath: path, LogOutput: io.Discard})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := a.Close(); err != nil {
				t.Error(err)
			}
		})
		proxy := httptest.NewServer(a.Server.Handler)
		t.Cleanup(proxy.Close)
		return a, proxy
	}
	a, first := build()
	b, second := build()
	call := func(proxy *httptest.Server, method, route, token, body string) appDeferredReply {
		if strings.HasPrefix(route, "/_internal/deferred/") {
			r, err := http.NewRequest(method, proxy.URL+route, strings.NewReader(body))
			if err != nil {
				t.Fatal(err)
			}
			r.Header.Set("Authorization", "Bearer "+token)
			r.Header.Set("X-Aiproxy-Deferred-Version", "1")
			response, err := proxy.Client().Do(r)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			data, err := io.ReadAll(response.Body)
			if err != nil {
				t.Fatal(err)
			}
			return appDeferredReply{status: response.StatusCode, header: response.Header, body: data}
		}
		return workspaceInferenceRequest(t, proxy, method, route, token, body)
	}
	chat := func(proxy *httptest.Server, token, model string) appDeferredReply {
		return call(proxy, "POST", "/v1/chat/completions", token, fmt.Sprintf(`{"model":%q,"messages":[]}`, model))
	}
	wait := func(predicate func() bool) {
		t.Helper()
		deadline := time.Now().Add(12 * time.Second)
		for !predicate() {
			if time.Now().After(deadline) {
				t.Fatal("replica did not converge")
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	if chat(second, "db-token", "alias/route").status != 200 {
		t.Fatal("initial alias failed")
	}
	models[0].UpstreamName = "v2"
	if err := st.UpdateProviderAggregate(ctx, &p, &models); err != nil {
		t.Fatal(err)
	}
	if err := a.Reload(); err != nil {
		t.Fatal(err)
	}
	revision := a.handler.SnapshotDependencies().ServingRevisions
	wait(func() bool { return b.handler.SnapshotDependencies().ServingRevisions == revision })
	if reply := chat(second, "db-token", "managed/m"); reply.status != 200 || !strings.Contains(string(reply.body), "v2") {
		t.Fatal("catalog convergence", reply.status, string(reply.body))
	}
	converge := func() {
		t.Helper()
		if err := a.Reload(); err != nil {
			t.Fatal(err)
		}
		revision = a.handler.SnapshotDependencies().ServingRevisions
		wait(func() bool { return b.handler.SnapshotDependencies().ServingRevisions == revision })
	}
	extra := store.DBProvider{Name: "created", Type: "openai-compatible", BaseURL: upstream.URL, APIKeyEncrypted: encrypted, Enabled: true, WorkspaceID: store.SystemWorkspaceID}
	if err := st.CreateProviderAggregate(ctx, &extra, []store.DBProviderModel{{Name: "m", UpstreamName: "v3"}}); err != nil {
		t.Fatal(err)
	}
	createdAlias := store.DBAlias{Name: "created-route", Algorithm: "round_robin", WorkspaceID: store.SystemWorkspaceID}
	if err := st.CreateAlias(ctx, &createdAlias, []store.DBAliasTarget{{Provider: "managed", Model: "m"}}); err != nil {
		t.Fatal(err)
	}
	extraKey := store.InboundKey{Name: "created-key", TokenHash: store.TokenHash("created-token"), TokenPrefix: "created", Enabled: true, WorkspaceID: store.SystemWorkspaceID}
	if err := st.CreateInboundKey(ctx, &extraKey); err != nil {
		t.Fatal(err)
	}
	converge()
	if chat(second, "created-token", "created/m").status != 200 || chat(second, "created-token", "alias/created-route").status != 200 {
		t.Fatal("create did not converge")
	}
	if err := st.UpdateAlias(ctx, &createdAlias, []store.DBAliasTarget{{Provider: "created", Model: "m"}}); err != nil {
		t.Fatal(err)
	}
	converge()
	if reply := chat(second, "created-token", "alias/created-route"); reply.status != 200 || !strings.Contains(string(reply.body), "v3") {
		t.Fatal("alias target update did not converge", reply.status)
	}
	if err := st.DeleteAlias(ctx, createdAlias.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteProvider(ctx, extra.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteInboundKey(ctx, extraKey.ID); err != nil {
		t.Fatal(err)
	}
	converge()
	beforeDeleted := calls.Load()
	if chat(second, "created-token", "managed/m").status != 401 || chat(second, "db-token", "created/m").status != 404 || chat(second, "db-token", "alias/created-route").status != 404 || calls.Load() != beforeDeleted {
		t.Fatal("delete did not converge without upstream calls")
	}
	if _, err := st.DB.ExecContext(ctx, "UPDATE inbound_keys SET allowed_models = ? WHERE id = ?", `["alias/route"]`, key.ID); err != nil {
		t.Fatal(err)
	}
	if err := a.Reload(); err != nil {
		t.Fatal(err)
	}
	revision = a.handler.SnapshotDependencies().ServingRevisions
	wait(func() bool { return b.handler.SnapshotDependencies().ServingRevisions.Keys == revision.Keys })
	before := calls.Load()
	if chat(second, "db-token", "managed/m").status != 403 || calls.Load() != before {
		t.Fatal("key policy did not converge")
	}
	b.reconcileCancel()
	<-b.reconcileDone
	if err := st.RotateInboundKey(ctx, key.ID, store.TokenHash("rotated-token"), "rotated"); err != nil {
		t.Fatal(err)
	}
	if err := b.ReconcileServing(); err != nil {
		t.Fatal(err)
	}
	if chat(second, "db-token", "alias/route").status != 401 || chat(second, "rotated-token", "alias/route").status != 200 {
		t.Fatal("missed-event reconciliation/rotation")
	}
	_, restarted := build()
	if chat(restarted, "rotated-token", "alias/route").status != 200 || call(restarted, "GET", "/readyz", "", "").status != 200 {
		t.Fatal("restart not current/ready")
	}
	if _, err := st.DB.ExecContext(ctx, "UPDATE db_providers SET type = 'invalid' WHERE id = ?", p.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.SetInboundKeyEnabled(ctx, key.ID, false); err != nil {
		t.Fatal(err)
	}
	if err := b.ReconcileServing(); err == nil {
		t.Fatal("invalid catalog applied")
	}
	if chat(second, "rotated-token", "alias/route").status != 401 {
		t.Fatal("invalid catalog blocked revocation")
	}
	if b.handler.SnapshotDependencies().ServingRevisions.Catalog != revision.Catalog {
		t.Fatal("invalid catalog certified freshness")
	}
	if err := st.SetInboundKeyEnabled(ctx, key.ID, true); err != nil {
		t.Fatal(err)
	}
	catalogConfirmed := b.handler.SnapshotDependencies().CatalogConfirmedAt
	for time.Now().Before(catalogConfirmed.Add(30 * time.Second)) {
		if err := b.ReconcileServing(); err == nil {
			t.Fatal("invalid catalog unexpectedly recovered")
		}
		time.Sleep(time.Second)
	}
	before = calls.Load()
	for _, model := range []string{"managed/m", "alias/route"} {
		token := "rotated-token"
		if model == "managed/m" {
			token = "static-token"
		}
		if reply := chat(second, token, model); reply.status != 503 {
			t.Fatal("invalid catalog exceeded admission deadline", model, reply.status)
		}
	}
	for _, route := range []string{"/v1/models", "/v1/billing/usage"} {
		if reply := call(second, "GET", route, "rotated-token", ""); reply.status != 503 {
			t.Fatal("stale catalog read admitted", route, reply.status)
		}
	}
	status := b.handler.ServingStatus()
	if status["keys_fresh"] != true || status["catalog_fresh"] != false || calls.Load() != before {
		t.Fatal("invalid catalog conflated key freshness or reached upstream", status)
	}
	if chat(second, "static-token", "local/m").status != 200 {
		t.Fatal("invalid catalog disabled trusted HCL")
	}
	if _, err := st.DB.ExecContext(ctx, "UPDATE db_providers SET type = 'openai-compatible' WHERE id = ?", p.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.SetInboundKeyEnabled(ctx, key.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := b.ReconcileServing(); err != nil {
		t.Fatal(err)
	}
	admitted := make(chan int, 1)
	go func() {
		r, _ := http.NewRequest("POST", second.URL+"/v1/chat/completions", strings.NewReader(`{"model":"alias/route","messages":[{"role":"user","content":"hold"}]}`))
		r.Header.Set("Authorization", "Bearer rotated-token")
		r.Header.Set("Content-Type", "application/json")
		response, err := second.Client().Do(r)
		if err != nil {
			admitted <- 0
			return
		}
		defer response.Body.Close()
		_, _ = io.ReadAll(response.Body)
		admitted <- response.StatusCode
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("request not admitted")
	}
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	streamFinished := make(chan bool, 1)
	go func() {
		r, _ := http.NewRequest("POST", second.URL+"/v1/chat/completions", strings.NewReader(`{"model":"alias/route","messages":[{"role":"user","content":"hold"}],"stream":true}`))
		r.Header.Set("Authorization", "Bearer rotated-token")
		r.Header.Set("Content-Type", "application/json")
		response, err := second.Client().Do(r)
		if err != nil {
			streamFinished <- false
			return
		}
		defer response.Body.Close()
		data, err := io.ReadAll(response.Body)
		streamFinished <- err == nil && response.StatusCode == 200 && strings.Contains(string(data), "[DONE]")
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("stream not established")
	}
	if _, err := st.DB.ExecContext(ctx, "ALTER TABLE serving_revisions RENAME TO unavailable_serving_revisions"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = st.DB.ExecContext(ctx, "ALTER TABLE IF EXISTS unavailable_serving_revisions RENAME TO serving_revisions")
	})
	confirmed := b.handler.SnapshotDependencies().KeysConfirmedAt
	if err := b.ReconcileServing(); err == nil {
		t.Fatal("failed revision read renewed freshness")
	}
	if b.handler.SnapshotDependencies().KeysConfirmedAt != confirmed {
		t.Fatal("poll success falsely recorded")
	}
	time.Sleep(time.Until(confirmed.Add(30*time.Second)) + 10*time.Millisecond)
	before = calls.Load()
	for _, route := range []string{"/v1/models", "/v1/billing/usage", "/_internal/deferred/v1/capabilities"} {
		if reply := call(second, "GET", route, "rotated-token", ""); reply.status != 503 {
			t.Fatalf("stale %s: %d %s", route, reply.status, reply.body)
		}
	}
	for _, suffix := range []string{"", "/read", "/result"} {
		if reply := call(second, "GET", "/_internal/deferred/v1/jobs/fixture"+suffix, "rotated-token", ""); reply.status != 503 {
			t.Fatal("stale peer control/replay admitted", suffix, reply.status)
		}
	}
	if reply := call(second, "DELETE", "/_internal/deferred/v1/jobs/fixture", "rotated-token", ""); reply.status != 503 {
		t.Fatal("stale peer delete admitted", reply.status)
	}
	for _, model := range []string{"managed/m", "alias/route", "local/m"} {
		if reply := chat(second, "rotated-token", model); reply.status != 503 {
			t.Fatal("stale key admitted", model, reply.status)
		}
	}
	if chat(second, "static-token", "managed/m").status != 503 || calls.Load() != before {
		t.Fatal("stale serving reached upstream")
	}
	if call(second, "GET", "/readyz", "", "").status != 503 || call(second, "GET", "/healthz", "", "").status != 200 {
		t.Fatal("readiness/liveness semantics")
	}
	if chat(second, "static-token", "local/m").status != 200 {
		t.Fatal("trusted HCL unavailable")
	}
	close(release)
	select {
	case status := <-admitted:
		if status != 200 {
			t.Fatal("admitted work canceled", status)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("admitted work did not finish")
	}
	select {
	case ok := <-streamFinished:
		if !ok {
			t.Fatal("established SSE interrupted")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("SSE did not finish")
	}
	if _, err := st.DB.ExecContext(ctx, "ALTER TABLE unavailable_serving_revisions RENAME TO serving_revisions"); err != nil {
		t.Fatal(err)
	}
	if err := b.ReconcileServing(); err != nil {
		t.Fatal(err)
	}
	if chat(second, "rotated-token", "alias/route").status != 200 || call(second, "GET", "/readyz", "", "").status != 200 {
		t.Fatal("freshness recovery failed")
	}
	_ = first
}

func TestServingCandidateRechecksRevisionBeforePublication(t *testing.T) {
	a, _, _, _ := catalogRuntimeFixture(t)
	a.reconcileCancel()
	<-a.reconcileDone
	before := a.handler.SnapshotDependencies()
	previous := a.Config
	var changed atomic.Bool
	var mutationTime time.Time
	a.buildOpt.transportFactory = func(options upstreamhttp.Options) (upstreamhttp.Transport, error) {
		if changed.CompareAndSwap(false, true) {
			mutationTime = time.Now()
			if _, err := a.adminStore.DB.ExecContext(context.Background(), "UPDATE db_aliases SET name = name WHERE FALSE"); err != nil {
				return nil, err
			}
		}
		return upstreamhttp.NewTransport(options)
	}
	if err := a.Reload(); !errors.Is(err, dbmerge.ErrSnapshotChanged) {
		t.Fatal("prepared old candidate published after new revision", err)
	}
	after := a.handler.SnapshotDependencies()
	if a.Config != previous || after.ServingRevisions.Catalog != before.ServingRevisions.Catalog || !after.CatalogConfirmedAt.Before(mutationTime) {
		t.Fatal("rejected candidate replaced or renewed old catalog")
	}
	a.buildOpt.transportFactory = nil
	if err := a.Reload(); err != nil {
		t.Fatal("stable candidate did not recover", err)
	}
}
