package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"reflect"
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

const quotaFaultDetail = "FLOW01_PRIVATE database credential detail"

func installQuotaReadFault(t *testing.T, st *store.Store, stage string) func() {
	t.Helper()
	ctx := context.Background()
	name := "flow01_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	table := "scope_quotas"
	predicate := "model = ''"
	if stage == "model" {
		predicate = "model <> ''"
	}
	columns := "workspace_id, scope_type, scope_id, model, CASE WHEN " + predicate + " THEN " + name + "(budget_micros) ELSE budget_micros END AS budget_micros, tpm_ceiling, tpm_effective, spent_offset_micros, created_at, updated_at"
	if stage == "spend" {
		table = "spend_ledger"
		columns = "id, workspace_id, key_id, user_id, team_id, model, tokens, " + name + "(cost_micros) AS cost_micros, created_at"
	}
	tx, err := st.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	for _, query := range []string{
		"CREATE FUNCTION " + name + "(bigint) RETURNS bigint LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION '" + quotaFaultDetail + "'; END $$",
		"ALTER TABLE " + table + " RENAME TO " + name,
		"CREATE VIEW " + table + " AS SELECT " + columns + " FROM " + name,
	} {
		if _, err := tx.ExecContext(ctx, query); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	var once sync.Once
	restore := func() {
		once.Do(func() {
			tx, err := st.DB.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			for _, query := range []string{
				"DROP VIEW " + table,
				"ALTER TABLE " + name + " RENAME TO " + table,
				"DROP FUNCTION " + name + "(bigint)",
			} {
				if _, err := tx.ExecContext(ctx, query); err != nil {
					t.Fatal(err)
				}
			}
			if err := tx.Commit(); err != nil {
				t.Fatal(err)
			}
		})
	}
	t.Cleanup(restore)
	return restore
}

type quotaFailureFixture struct {
	st        *store.Store
	admin     *Handler
	access    string
	quotaPath string
	workspace uuid.UUID
	scope     quotaScope
	proxy     *Handler
	calls     atomic.Int64
	logs      bytes.Buffer
	token     string
}

func newQuotaFailureFixture(t *testing.T, owner string) *quotaFailureFixture {
	t.Helper()
	st, admin, access, workspaceID, _, _, memberID, _ := quotaSetup(t)
	f := &quotaFailureFixture{st: st, admin: admin, access: access, workspace: mustParseUUID(workspaceID), scope: quotaScope{typ: owner, id: mustParseUUID(memberID)}}
	if owner == "team" {
		teams, err := st.ListTeamsByWorkspace(context.Background(), f.workspace)
		if err != nil || len(teams) != 1 {
			t.Fatalf("teams = %v, %v", teams, err)
		}
		f.scope.id = teams[0].ID
	}
	f.quotaPath = "/_internal/admin/workspaces/" + workspaceID + "/" + owner + "s/" + f.scope.id.String() + "/quota"
	code, body := callAdmin(t, admin, access, http.MethodPost, "/_internal/admin/keys", map[string]interface{}{
		"name": uuid.NewString(), "workspace_id": workspaceID, "owner_type": owner, "owner_id": f.scope.id.String(),
	})
	if code != http.StatusCreated {
		t.Fatalf("create key = %d %v", code, body)
	}
	f.token = body["token"].(string)
	key := body["key"].(map[string]interface{})
	dyn := auth.DynamicClient{Name: key["name"].(string), KeyID: mustParseUUID(key["id"].(string)), WorkspaceID: f.workspace, TokenHash: store.TokenHash(f.token)}
	entry := store.SpendEntry{WorkspaceID: f.workspace, KeyID: dyn.KeyID, Model: "openai/m", CostMicros: 150}
	if owner == "user" {
		dyn.OwnerUserID, entry.UserID = f.scope.id, &f.scope.id
	} else {
		dyn.OwnerTeamID, entry.TeamID = f.scope.id, &f.scope.id
	}
	if err := st.InsertSpendEntry(context.Background(), &entry); err != nil {
		t.Fatal(err)
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.calls.Add(1)
		var body struct{ Stream bool }
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Stream {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "data: {\"id\":\"chatcmpl-test\",\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
		} else {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"id":"chatcmpl-test","choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`)
		}
	}))
	t.Cleanup(upstream.Close)
	rt := &config.Runtime{
		Auth: config.Auth{Mode: config.AuthModeBearerStatic},
		Catalog: config.NewCatalog([]config.Provider{{Name: "openai", Type: config.ProviderTypeOpenAI, BaseURL: upstream.URL, APIKey: "upstream-private",
			Models: []config.Model{{Name: "m", UpstreamName: "m", Capabilities: []config.Capability{
				config.CapabilityChat, config.CapabilityResponses, config.CapabilityEmbeddings,
				config.CapabilityImages, config.CapabilityAudioTranscriptions, config.CapabilityAudioSpeech,
			}}},
		}}, nil, []config.Alias{{Name: "fast", Algorithm: config.AlgorithmRoundRobin, Targets: []config.AliasTarget{{Provider: "openai", Model: "m"}}}}),
	}
	f.proxy = NewHandler(Dependencies{
		Resolver: modelresolver.New(rt), Catalog: rt.Catalog, MultiTenancy: config.MultiTenancy{Enabled: true}, AdminStore: st,
		Auth: auth.NewAuthenticatorWithClients(rt.Auth, []auth.DynamicClient{dyn}), Logger: slog.New(slog.NewTextHandler(&f.logs, nil)),
	})
	deps := admin.current()
	deps.Logger = f.proxy.current().Logger
	admin.UpdateDependencies(deps)
	return f
}

func (f *quotaFailureFixture) setPolicy(t *testing.T, budget, tpm int64) {
	t.Helper()
	for _, model := range []string{"", "openai/m", "alias/fast"} {
		row := store.ScopeQuota{WorkspaceID: f.workspace, ScopeType: f.scope.typ, ScopeID: f.scope.id, Model: model}
		if model == "" {
			row.BudgetMicros = budget
		} else {
			row.TPMCeiling = tpm
		}
		if err := f.st.UpsertScopeQuota(context.Background(), &row); err != nil {
			t.Fatal(err)
		}
	}
}

func (f *quotaFailureFixture) request(model string, stream bool) *httptest.ResponseRecorder {
	return quotaProxyRequest(f.proxy, f.token, "/v1/chat/completions", fmt.Sprintf(`{"model":%q,"stream":%v,"messages":[{"role":"user","content":"hi"}]}`, model, stream))
}

func quotaProxyRequest(h *Handler, token, path, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, r)
	return w
}

func assertQuotaUnavailable(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	want := `{"error":{"type":"quota_unavailable","message":"quota data temporarily unavailable"}}`
	if w.Code != http.StatusServiceUnavailable || strings.TrimSpace(w.Body.String()) != want || w.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("want controlled JSON 503; got %d %s (%s)", w.Code, w.Body.String(), w.Header().Get("Content-Type"))
	}
}

func TestQuotaReadFailures(t *testing.T) {
	for _, owner := range []string{"user", "team"} {
		t.Run(owner, func(t *testing.T) {
			f := newQuotaFailureFixture(t, owner)
			for _, stage := range []string{"budget", "model", "spend"} {
				for _, cache := range []string{"cold", "expired"} {
					t.Run(stage+"/"+cache, func(t *testing.T) {
						f.setPolicy(t, 100, 0)
						if stage == "model" {
							f.setPolicy(t, 1000, 10)
						}
						tracker := NewQuotaTracker(f.st)
						key := f.workspace.String() + "|" + owner + "|" + f.scope.id.String()
						stale := cachedSpend{sum: 0, at: time.Now().Add(-2 * quotaSpendTTL)}
						if cache == "expired" {
							tracker.spend[key] = stale
						}
						for _, model := range []string{"openai/m", "alias/fast"} {
							tracker.recordTokens(f.workspace, f.scope, model, 10)
						}
						deps := f.proxy.current()
						deps.Quota = tracker
						f.proxy.UpdateDependencies(deps)
						restore := installQuotaReadFault(t, f.st, stage)
						var err error
						if stage == "spend" {
							_, err = f.st.SumScopeSpend(context.Background(), f.workspace, owner, f.scope.id)
						} else {
							model, other := "", "openai/m"
							if stage == "model" {
								model, other = other, model
							}
							_, err = f.st.GetScopeQuota(context.Background(), f.workspace, owner, f.scope.id, model)
							if _, otherErr := f.st.GetScopeQuota(context.Background(), f.workspace, owner, f.scope.id, other); otherErr != nil {
								t.Fatalf("fault also affected independent row: %v", otherErr)
							}
						}
						if err == nil || !strings.Contains(err.Error(), quotaFaultDetail) {
							t.Fatalf("PG fault was not exercised: %v", err)
						}
						before := f.calls.Load()
						for _, model := range []string{"openai/m", "alias/fast"} {
							for _, stream := range []bool{false, true} {
								t.Run(fmt.Sprintf("%s/stream=%v", model, stream), func(t *testing.T) {
									assertQuotaUnavailable(t, f.request(model, stream))
								})
							}
						}
						if f.calls.Load() != before {
							t.Fatalf("quota failure dispatched upstream: %d -> %d", before, f.calls.Load())
						}
						if !strings.Contains(f.logs.String(), quotaFaultDetail) {
							t.Fatal("server log omitted underlying cause")
						}
						f.logs.Reset()
						if stage == "spend" {
							cached, exists := tracker.spend[key]
							if (cache == "cold" && exists) || (cache == "expired" && cached != stale) {
								t.Fatalf("failed refresh published/extended cache: %+v, %v", cached, exists)
							}
						}
						t.Logf("PG %s read failure: direct/alias JSON/SSE rejected without upstream calls", stage)
						restore()
						status, typ := http.StatusForbidden, "budget_exceeded"
						if stage == "model" {
							status, typ = http.StatusTooManyRequests, "tpm_exceeded"
						}
						for _, model := range []string{"openai/m", "alias/fast"} {
							for _, stream := range []bool{false, true} {
								w := f.request(model, stream)
								if w.Code != status || !strings.Contains(w.Body.String(), typ) || f.calls.Load() != before {
									t.Fatalf("recovered enforcement = %d %s; calls=%d", w.Code, w.Body.String(), f.calls.Load())
								}
								if stage == "model" && w.Header().Get("Retry-After") == "" {
									t.Fatal("recovered TPM missing Retry-After")
								}
							}
						}
						f.setPolicy(t, 1000, 0)
						for _, model := range []string{"openai/m", "alias/fast"} {
							for _, stream := range []bool{false, true} {
								w := f.request(model, stream)
								if w.Code != http.StatusOK {
									t.Fatalf("recovered admission = %d %s", w.Code, w.Body.String())
								}
							}
						}
						if f.calls.Load() != before+4 {
							t.Fatalf("recovered admission calls = %d, want %d", f.calls.Load(), before+4)
						}
					})
				}
			}
		})
	}
}

func TestQuotaFreshSpendCache(t *testing.T) {
	for _, owner := range []string{"user", "team"} {
		t.Run(owner, func(t *testing.T) {
			f := newQuotaFailureFixture(t, owner)
			f.setPolicy(t, 1000, 0)
			tracker := f.proxy.current().Quota
			if spent, err := tracker.scopeSpend(context.Background(), f.workspace, f.scope); err != nil || spent != 150 {
				t.Fatalf("prime cache = %d, %v", spent, err)
			}
			key := f.workspace.String() + "|" + owner + "|" + f.scope.id.String()
			cached := tracker.spend[key]
			restore := installQuotaReadFault(t, f.st, "spend")
			if _, err := f.st.SumScopeSpend(context.Background(), f.workspace, owner, f.scope.id); err == nil || !strings.Contains(err.Error(), quotaFaultDetail) {
				t.Fatalf("spend fault not active: %v", err)
			}
			before := f.calls.Load()
			for _, model := range []string{"openai/m", "alias/fast"} {
				for _, stream := range []bool{false, true} {
					if w := f.request(model, stream); w.Code != http.StatusOK {
						t.Fatalf("fresh cache headroom = %d %s", w.Code, w.Body.String())
					}
				}
			}
			if f.calls.Load() != before+4 {
				t.Fatal("fresh cache did not permit all requests")
			}
			f.setPolicy(t, 100, 0)
			before = f.calls.Load()
			for _, model := range []string{"openai/m", "alias/fast"} {
				for _, stream := range []bool{false, true} {
					w := f.request(model, stream)
					if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "budget_exceeded") {
						t.Fatalf("fresh cached exhaustion = %d %s", w.Code, w.Body.String())
					}
				}
			}
			if tracker.spend[key] != cached || f.calls.Load() != before {
				t.Fatal("fresh cache extended or exhausted budget dispatched")
			}
			cached.at = time.Now().Add(-quotaSpendTTL)
			tracker.spend[key] = cached
			for _, model := range []string{"openai/m", "alias/fast"} {
				for _, stream := range []bool{false, true} {
					assertQuotaUnavailable(t, f.request(model, stream))
				}
			}
			if tracker.spend[key] != cached || f.calls.Load() != before {
				t.Fatal("failed expiry refresh extended cache or dispatched")
			}
			restore()
			if w := f.request("openai/m", false); w.Code != http.StatusForbidden || f.calls.Load() != before {
				t.Fatalf("recovered refresh = %d %s", w.Code, w.Body.String())
			}
			if refreshed := tracker.spend[key]; refreshed.sum != 150 || !refreshed.at.After(cached.at) {
				t.Fatalf("recovered spend not cached: %+v", refreshed)
			}
			f.setPolicy(t, 1000, 10)
			for _, stage := range []string{"budget", "model"} {
				t.Run("fresh/"+stage, func(t *testing.T) {
					restore := installQuotaReadFault(t, f.st, stage)
					for _, model := range []string{"openai/m", "alias/fast"} {
						for _, stream := range []bool{false, true} {
							assertQuotaUnavailable(t, f.request(model, stream))
						}
					}
					if f.calls.Load() != before {
						t.Fatal("fresh spend cache bypassed missing policy data")
					}
					restore()
				})
			}
		})
	}
}

func TestQuotaUnlimitedAndStaticControls(t *testing.T) {
	for _, owner := range []string{"user", "team"} {
		t.Run(owner, func(t *testing.T) {
			f := newQuotaFailureFixture(t, owner)
			ctx := context.Background()
			for _, model := range []string{"", "openai/m", "alias/fast"} {
				if _, err := f.st.GetScopeQuota(ctx, f.workspace, owner, f.scope.id, model); !errors.Is(err, sql.ErrNoRows) {
					t.Fatalf("genuine absent row = %v", err)
				}
				if _, ok, err := f.proxy.current().Quota.quotaRow(ctx, f.workspace, f.scope, model); err != nil || ok {
					t.Fatalf("absent policy = %v, %v", ok, err)
				}
			}
			code, body := callAdmin(t, f.admin, f.access, http.MethodGet, f.quotaPath, nil)
			if code != http.StatusOK || body["budget_micros"] != float64(0) || body["spend_micros"] != float64(150) {
				t.Fatalf("no-budget admin spend = %d %v", code, body)
			}
			for _, policy := range []string{"absent", "zero"} {
				t.Run(policy, func(t *testing.T) {
					if policy == "zero" {
						f.setPolicy(t, 0, 0)
					}
					restore := installQuotaReadFault(t, f.st, "spend")
					before := f.calls.Load()
					for _, model := range []string{"openai/m", "alias/fast"} {
						for _, stream := range []bool{false, true} {
							if w := f.request(model, stream); w.Code != http.StatusOK {
								t.Fatalf("unlimited request = %d %s", w.Code, w.Body.String())
							}
						}
					}
					if f.calls.Load() != before+4 {
						t.Fatal("unlimited requests did not all dispatch")
					}
					w := membershipRequest(ctx, f.admin, f.access, http.MethodGet, f.quotaPath, "")
					if w.Code != http.StatusInternalServerError || w.Body.String() != "could not load quota\n" {
						t.Fatalf("unlimited admin hid spend failure = %d %s", w.Code, w.Body.String())
					}
					restore()
				})
			}
			f.setPolicy(t, 100, 10)
			deps := f.proxy.current()
			deps.Auth = auth.NewAuthenticator(config.Auth{Mode: config.AuthModeBearerStatic, Clients: map[string]config.Client{"static": {Name: "static", Token: "static-private"}}})
			f.proxy.UpdateDependencies(deps)
			f.token = "static-private"
			for _, stage := range []string{"budget", "model", "spend"} {
				t.Run("static/"+stage, func(t *testing.T) {
					restore := installQuotaReadFault(t, f.st, stage)
					before := f.calls.Load()
					for _, model := range []string{"openai/m", "alias/fast"} {
						for _, stream := range []bool{false, true} {
							if w := f.request(model, stream); w.Code != http.StatusOK {
								t.Fatalf("static request = %d %s", w.Code, w.Body.String())
							}
						}
					}
					if f.calls.Load() != before+4 {
						t.Fatal("static requests did not all dispatch")
					}
					restore()
				})
			}
		})
	}
}

func TestAdminQuotaReadFailures(t *testing.T) {
	for _, owner := range []string{"user", "team"} {
		t.Run(owner, func(t *testing.T) {
			f := newQuotaFailureFixture(t, owner)
			ctx := context.Background()
			f.setPolicy(t, 100, 10)
			if _, err := f.st.DB.NewUpdate().Model((*store.ScopeQuota)(nil)).Set("spent_offset_micros = 50").Where("workspace_id = ? AND scope_type = ? AND scope_id = ? AND model = ''", f.workspace, owner, f.scope.id).Exec(ctx); err != nil {
				t.Fatal(err)
			}
			before, err := f.st.ListScopeQuotasByScope(ctx, f.workspace, owner, f.scope.id)
			if err != nil {
				t.Fatal(err)
			}
			for _, stage := range []string{"budget", "model", "spend"} {
				t.Run(stage, func(t *testing.T) {
					restore := installQuotaReadFault(t, f.st, stage)
					body := `{"budget_micros":200}`
					var err error
					if stage == "model" {
						body = `{"tpm":[{"model":"openai/m","effective":5}]}`
						value := int64(5)
						err = f.admin.applyQuotaTPM(f.admin.current(), ctx, f.workspace, f.scope, []adminTPMUpsert{{Model: "openai/m", Effective: &value}}, true)
					} else {
						if stage == "spend" {
							body = `{"reset_spend":true}`
						}
						err = f.admin.applyQuotaBudget(f.admin.current(), ctx, f.workspace, f.scope, 200, stage == "spend")
					}
					if err == nil || !strings.Contains(err.Error(), quotaFaultDetail) {
						t.Fatalf("admin read error did not propagate before write: %v", err)
					}
					for _, method := range []string{http.MethodGet, http.MethodPut} {
						f.logs.Reset()
						w := membershipRequest(ctx, f.admin, f.access, method, f.quotaPath, body)
						want := "could not load quota\n"
						if method == http.MethodPut {
							want = "could not update quota\n"
						}
						if w.Code != http.StatusInternalServerError || w.Body.String() != want || !strings.Contains(f.logs.String(), quotaFaultDetail) {
							t.Fatalf("admin %s = %d %s; log=%s", method, w.Code, w.Body.String(), f.logs.String())
						}
					}
					restore()
					after, err := f.st.ListScopeQuotasByScope(ctx, f.workspace, owner, f.scope.id)
					if err != nil || !reflect.DeepEqual(before, after) {
						t.Fatalf("failed reads changed policy: before=%+v after=%+v err=%v", before, after, err)
					}
				})
			}
			restore := installQuotaReadFault(t, f.st, "spend")
			w := membershipRequest(ctx, f.admin, f.access, http.MethodPut, f.quotaPath, `{"budget_micros":200}`)
			if w.Code != http.StatusInternalServerError || w.Body.String() != "quota saved but could not load updated quota\n" {
				t.Fatalf("saved quota view failure = %d %s", w.Code, w.Body.String())
			}
			restore()
			row, err := f.st.GetScopeQuota(ctx, f.workspace, owner, f.scope.id, "")
			if err != nil || row.BudgetMicros != 200 || row.SpentOffsetMicros != 50 {
				t.Fatalf("saved policy/offset = %+v, %v", row, err)
			}
			code, body := callAdmin(t, f.admin, f.access, http.MethodGet, f.quotaPath, nil)
			if code != http.StatusOK || body["budget_micros"] != float64(200) || body["spend_micros"] != float64(100) {
				t.Fatalf("recovered admin view = %d %v", code, body)
			}
		})
	}
}

func TestQuotaReadFailureOtherOperations(t *testing.T) {
	for _, owner := range []string{"user", "team"} {
		t.Run(owner, func(t *testing.T) {
			f := newQuotaFailureFixture(t, owner)
			f.setPolicy(t, 100, 0)
			restore := installQuotaReadFault(t, f.st, "budget")
			for _, model := range []string{"openai/m", "alias/fast"} {
				for _, tc := range []struct{ path, body string }{
					{"/v1/responses", `{"model":%q,"input":"hi"}`},
					{"/v1/responses", `{"model":%q,"input":"hi","stream":true}`},
					{"/v1/embeddings", `{"model":%q,"input":"hi"}`},
					{"/v1/images/generations", `{"model":%q,"prompt":"hi"}`},
					{"/v1/audio/speech", `{"model":%q,"input":"hi","voice":"alloy"}`},
				} {
					assertQuotaUnavailable(t, quotaProxyRequest(f.proxy, f.token, tc.path, fmt.Sprintf(tc.body, model)))
				}
				var body bytes.Buffer
				form := multipart.NewWriter(&body)
				if err := form.WriteField("model", model); err != nil {
					t.Fatal(err)
				}
				part, err := form.CreateFormFile("file", "clip.wav")
				if err != nil {
					t.Fatal(err)
				}
				if _, err := io.WriteString(part, "fixture audio"); err != nil {
					t.Fatal(err)
				}
				if err := form.Close(); err != nil {
					t.Fatal(err)
				}
				r := httptest.NewRequest(http.MethodPost, "/v1/audio/transcriptions", &body)
				r.Header.Set("Authorization", "Bearer "+f.token)
				r.Header.Set("Content-Type", form.FormDataContentType())
				w := httptest.NewRecorder()
				f.proxy.ServeHTTP(w, r)
				assertQuotaUnavailable(t, w)
			}
			if f.calls.Load() != 0 {
				t.Fatalf("alternate operation bypassed quota gate: %d upstream calls", f.calls.Load())
			}
			restore()
		})
	}
}
