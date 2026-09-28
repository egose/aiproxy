package httpapi

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/auth"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/modelresolver"
	"github.com/egose/aiproxy/internal/store"
	"github.com/google/uuid"
)

func TestQuotaSpendSurvivesKeyDeletion(t *testing.T) {
	for _, scopeType := range []string{"user", "team"} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%v", scopeType, stream), func(t *testing.T) {
				st, admin, ownerAccess, workspaceID, memberAccess, teamAccess, memberID, teamAdminID := quotaSetup(t)
				ctx := context.Background()
				workspaceUUID := mustParseUUID(workspaceID)
				teams, err := st.ListTeamsByWorkspace(ctx, workspaceUUID)
				if err != nil || len(teams) != 1 {
					t.Fatalf("teams = %+v, %v", teams, err)
				}
				scopeID, managerAccess := memberID, memberAccess
				if scopeType == "team" {
					scopeID, managerAccess = teams[0].ID.String(), teamAccess
				}
				scope := quotaScope{typ: scopeType, id: mustParseUUID(scopeID)}
				quotaPath := "/_internal/admin/workspaces/" + workspaceID + "/" + scopeType + "s/" + scopeID + "/quota"
				keyName := "retained-" + uuid.NewString()
				createKey := func() (uuid.UUID, string, auth.DynamicClient) {
					t.Helper()
					code, body := callAdmin(t, admin, managerAccess, http.MethodPost, "/_internal/admin/keys", map[string]interface{}{
						"name": keyName, "workspace_id": workspaceID, "owner_type": scopeType, "owner_id": scopeID,
						"user_ids": []string{memberID, teamAdminID}, "team_ids": []string{teams[0].ID.String()},
					})
					if code != http.StatusCreated {
						t.Fatalf("create key = %d, %v", code, body)
					}
					key := body["key"].(map[string]interface{})
					id, token := mustParseUUID(key["id"].(string)), body["token"].(string)
					dyn := auth.DynamicClient{KeyID: id, WorkspaceID: workspaceUUID, Name: keyName, TokenHash: store.TokenHash(token)}
					if scopeType == "user" {
						dyn.OwnerUserID = scope.id
					} else {
						dyn.OwnerTeamID = scope.id
					}
					if key["usage"].(map[string]interface{})["requests"] != float64(0) {
						t.Fatalf("new key inherited historical per-key usage: %v", key)
					}
					return id, token, dyn
				}
				keyID, token, dyn := createKey()
				if code, body := callAdmin(t, admin, ownerAccess, http.MethodPut, quotaPath, map[string]interface{}{"budget_micros": 10000}); code != http.StatusOK {
					t.Fatalf("set budget = %d, %v", code, body)
				}

				admitted := make(chan int64, 3)
				gates := []chan struct{}{make(chan struct{}), make(chan struct{})}
				var releases [2]sync.Once
				release := func(i int) { releases[i].Do(func() { close(gates[i]) }) }
				var calls atomic.Int64
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					n := calls.Add(1)
					admitted <- n
					if n <= 2 {
						select {
						case <-gates[n-1]:
						case <-r.Context().Done():
							return
						}
					}
					if stream {
						w.Header().Set("Content-Type", "text/event-stream")
						_, _ = io.WriteString(w, "data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"model\":\"m\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hi\"},\"finish_reason\":null}]}\n\ndata: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"model\":\"m\",\"choices\":[],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":5,\"total_tokens\":15}}\n\ndata: [DONE]\n\n")
					} else {
						w.Header().Set("Content-Type", "application/json")
						_, _ = io.WriteString(w, `{"id":"chatcmpl-1","object":"chat.completion","model":"m","choices":[{"index":0,"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`)
					}
				}))
				t.Cleanup(upstream.Close)
				t.Cleanup(func() { release(0); release(1) })
				rt := &config.Runtime{
					Auth: config.Auth{Mode: config.AuthModeBearerStatic},
					Catalog: config.NewCatalog([]config.Provider{{
						Name: "openai", Type: config.ProviderTypeOpenAI, BaseURL: upstream.URL, APIKey: "test",
						Models: []config.Model{{Name: "m", UpstreamName: "m", Pricing: &config.ModelPricing{InputPerMillion: 1000, OutputPerMillion: 2000}}},
					}}, nil, nil),
				}
				tracker := NewQuotaTracker(st)
				deps := Dependencies{
					Resolver: modelresolver.New(rt), Catalog: rt.Catalog, MultiTenancy: config.MultiTenancy{Enabled: true},
					AdminStore: st, Quota: tracker, Auth: auth.NewAuthenticatorWithClients(rt.Auth, []auth.DynamicClient{dyn}),
				}
				proxy := NewHandler(deps)
				callProxy := func(token string) *httptest.ResponseRecorder {
					w := httptest.NewRecorder()
					r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(fmt.Sprintf(`{"model":"openai/m","stream":%v,"messages":[{"role":"user","content":"hi"}]}`, stream)))
					r.Header.Set("Authorization", "Bearer "+token)
					r.Header.Set("Content-Type", "application/json")
					proxy.ServeHTTP(w, r)
					return w
				}
				startRequest := func() <-chan *httptest.ResponseRecorder {
					t.Helper()
					done := make(chan *httptest.ResponseRecorder, 1)
					go func() { done <- callProxy(token) }()
					select {
					case <-admitted:
					case w := <-done:
						t.Fatalf("request not admitted: %d %s", w.Code, w.Body.String())
					case <-time.After(5 * time.Second):
						t.Fatal("upstream admission timed out")
					}
					return done
				}
				finishRequest := func(done <-chan *httptest.ResponseRecorder) {
					t.Helper()
					select {
					case w := <-done:
						if w.Code != http.StatusOK {
							t.Fatalf("completion = %d %s", w.Code, w.Body.String())
						}
					case <-time.After(5 * time.Second):
						t.Fatal("completion timed out")
					}
				}
				first, late := startRequest(), startRequest()
				release(0)
				finishRequest(first)
				assertSpend := func(want int64) {
					t.Helper()
					if sum, err := st.SumScopeSpend(ctx, workspaceUUID, scopeType, scope.id); err != nil || sum != want {
						t.Fatalf("%s spend = %d, %v; want %d", scopeType, sum, err, want)
					}
				}
				assertSpend(20000)
				if code, body := callAdmin(t, admin, managerAccess, http.MethodDelete, "/_internal/admin/keys/"+keyID.String(), nil); code != http.StatusOK {
					t.Fatalf("owner delete = %d, %v", code, body)
				}
				assertSpend(20000)
				replacementID, replacementToken, replacement := createKey()
				if replacementID == keyID {
					t.Fatal("replacement reused deleted identity")
				}
				deps.Auth = auth.NewAuthenticatorWithClients(rt.Auth, []auth.DynamicClient{replacement})
				proxy.UpdateDependencies(deps)
				expireSpendCache := func() {
					tracker.mu.Lock()
					for key, value := range tracker.spend {
						value.at = time.Now().Add(-quotaSpendTTL)
						tracker.spend[key] = value
					}
					tracker.mu.Unlock()
				}
				assertBlocked := func() {
					t.Helper()
					before := calls.Load()
					w := callProxy(replacementToken)
					if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "budget_exceeded") || calls.Load() != before {
						t.Fatalf("replacement budget = %d %s; upstream calls %d -> %d", w.Code, w.Body.String(), before, calls.Load())
					}
				}
				expireSpendCache()
				assertBlocked()
				fresh, err := store.Open(ctx, os.Getenv("AIPROXY_TEST_DATABASE_URL"))
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = fresh.Close() })
				tracker = NewQuotaTracker(fresh)
				deps.AdminStore, deps.Quota = fresh, tracker
				proxy.UpdateDependencies(deps)
				assertBlocked()
				if code, body := callAdmin(t, admin, managerAccess, http.MethodPut, quotaPath, map[string]interface{}{"reset_spend": true}); code != http.StatusForbidden {
					t.Fatalf("non-workspace-admin reset = %d, %v", code, body)
				}
				assertSpend(20000)
				if code, body := callAdmin(t, admin, ownerAccess, http.MethodPut, quotaPath, map[string]interface{}{"reset_spend": true}); code != http.StatusOK || body["spend_micros"] != float64(0) {
					t.Fatalf("explicit reset = %d, %v", code, body)
				}
				assertSpend(20000)
				principal := &auth.Principal{KeyID: replacementID, WorkspaceID: workspaceUUID, OwnerUserID: replacement.OwnerUserID, OwnerTeamID: replacement.OwnerTeamID}
				if !proxy.allowQuota(deps, httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil), principal, "openai/m") {
					t.Fatal("explicit reset did not restore budget headroom")
				}
				release(1)
				finishRequest(late)
				assertSpend(40000)
				expireSpendCache()
				assertBlocked()
				if code, body := callAdmin(t, admin, ownerAccess, http.MethodGet, quotaPath, nil); code != http.StatusOK || body["spend_micros"] != float64(20000) {
					t.Fatalf("post-reset late spend = %d, %v", code, body)
				}
				usage, err := fresh.KeyUsageByWorkspace(ctx, workspaceUUID)
				if err != nil || len(usage) != 1 || usage[0].KeyID != keyID || usage[0].Requests != 2 || usage[0].Tokens != 30 || usage[0].Spend != 40000 {
					t.Fatalf("historical usage = %+v, %v", usage, err)
				}
				for _, other := range []quotaScope{{typ: "user", id: mustParseUUID(memberID)}, {typ: "user", id: mustParseUUID(teamAdminID)}, {typ: "team", id: teams[0].ID}} {
					if other == scope {
						continue
					}
					if sum, err := fresh.SumScopeSpend(ctx, workspaceUUID, other.typ, other.id); err != nil || sum != 0 {
						t.Fatalf("shared access charged non-owner %+v: %d, %v", other, sum, err)
					}
				}
				if sum, err := fresh.SumScopeSpend(ctx, uuid.New(), scopeType, scope.id); err != nil || sum != 0 {
					t.Fatalf("spend crossed workspace: %d, %v", sum, err)
				}
				if calls.Load() != 2 {
					t.Fatalf("upstream calls = %d, want only two admitted requests", calls.Load())
				}
			})
		}
	}
}
