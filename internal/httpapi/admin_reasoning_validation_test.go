package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"testing"

	"github.com/egose/aiproxy/internal/store"
)

func TestAdminEncryptedReasoningPresenceGuardsBeforeMutation(t *testing.T) {
	for _, fields := range []string{
		`"encrypted_reasoning":{"passthrough":null}`,
		`"encrypted_reasoning":{"passthrough":false,"passthrough":true}`,
		`"encrypted_reasoning":{"passthrough":false,"Passthrough":true}`,
		`"encrypted_reasoning":{"unknown":false}`,
		`"encrypted_reasoning":{"match_messages":[null]}`,
		`"encrypted_reasoning":{},"encrypted_reasoning":null`,
		`"encrypted_reasoning":{},"Encrypted_Reasoning":null`,
		`"encrypted_reasoning_effective":{}`,
		`"models":[{"name":"m","encrypted_reasoning":{}}],"models":null`,
		`"models":[{"name":"m","encrypted_reasoning":{}}],"Models":[]`,
		`"model\u017f":[{"name":"m","encrypted_reasoning":{}}]`,
		`"models":[{"name":"m","name":"other","encrypted_reasoning":{}}]`,
		`"models":[{"name":"m","encrypted_reasoning":{},"Encrypted_Reasoning":null}]`,
		`"models":[{"name":"m","encrypted_reasoning_source":"root"}]`,
		`"models":[{"name":"m","unknown":null}]`,
		`"models":[{"name":"m","deferred_transport":{},"deferred_transport":null}]`,
		`"models":[{"name":"m","deferred_transport":{}}],"models":[]`,
		`"models":[null]`,
	} {
		t.Run(fields, func(t *testing.T) {
			var req adminProviderUpsert
			raw := []byte(`{"name":"managed","display_name":"Rejected","api_key":"rejected-private-credential",` + fields + `}`)
			if err := json.Unmarshal(raw, &req); err != nil {
				t.Fatal(err)
			}
			row := store.DBProvider{Name: "managed", Type: "openai", DisplayName: "Original", EncryptedReasoning: json.RawMessage(`{}`)}
			before := row
			if err := (&Handler{}).applyProviderUpsert(row.Name, &row, req); err == nil || !reflect.DeepEqual(row, before) || bytes.Contains([]byte(err.Error()), []byte("private-credential")) {
				t.Fatalf("presence guard missed an invalid declaration or changed a candidate: %v", err)
			}
			if _, err := (&Handler{}).createDBProvider(context.Background(), Dependencies{}, nil, nil, store.Workspace{}, req); err == nil {
				t.Fatal("invalid create reached the store")
			}
			if _, err := (&Handler{}).updateDBProvider(context.Background(), Dependencies{}, nil, nil, req); err == nil {
				t.Fatal("invalid update reached the store")
			}
		})
	}
}

func TestAdminEncryptedReasoningStrictProviderAliasPolicyRequests(t *testing.T) {
	fixture := newAdminReasoningFixture(t, nil)
	requireAdminReasoningStatus(t, fixture, http.MethodPost, "/_internal/admin/providers",
		`{"name":"strict","type":"openai","api_key":"strict-private-credential","encrypted_reasoning":{},"models":[{"name":"m","encrypted_reasoning":{"passthrough":false}}]}`, 201)
	requireAdminReasoningStatus(t, fixture, http.MethodPost, "/_internal/admin/aliases",
		`{"name":"strict-alias","encrypted_reasoning":{},"targets":[{"provider":"strict","model":"m"}]}`, 201)
	provider, models := adminReasoningSavedProvider(t, fixture.store, "strict")
	alias, _ := fixture.store.GetAlias(context.Background(), "strict-alias")
	for _, policy := range []string{
		`false`, `[]`, `42`, `"reset"`, `{"passthrough":null}`, `{"passthrough":"false"}`,
		`{"on_caller_mismatch":null}`, `{"on_caller_mismatch":"invalid"}`,
		`{"match_messages":null}`, `{"match_messages":[null]}`, `{"match_messages":["abc"]}`,
		`{"match_messages":["same mismatch"," SAME MISMATCH "]}`,
		`{"passthrough":false,"passthrough":true}`, `{"passthrough":false,"Passthrough":true}`,
		`{"on_caller_mismatch":"fail","on_caller_mismatch":"strip_and_retry"}`,
		`{"unknown":true}`, `{"encrypted_reasoning_effective":{}}`,
	} {
		t.Run(policy, func(t *testing.T) {
			count := fixture.activations
			for _, path := range []string{"/_internal/admin/providers/strict", "/_internal/admin/aliases/strict-alias"} {
				w, _ := adminReasoningRequest(t, fixture.handler, fixture.access, http.MethodPut, path, `{"encrypted_reasoning":`+policy+`}`)
				if w.Code != 400 || w.Header().Get(catalogSavedHeader) != "" || fixture.activations != count {
					t.Fatalf("invalid policy was saved: %d %s", w.Code, w.Body.String())
				}
			}
			w, _ := adminReasoningRequest(t, fixture.handler, fixture.access, http.MethodPut, "/_internal/admin/providers/strict",
				`{"models":[{"name":"m","encrypted_reasoning":`+policy+`}]}`)
			if w.Code != 400 || fixture.activations != count || w.Header().Get(catalogSavedHeader) != "" {
				t.Fatalf("invalid model policy was saved: %d %s", w.Code, w.Body.String())
			}
			w, _ = adminReasoningRequest(t, fixture.handler, fixture.access, http.MethodPost, "/_internal/admin/providers",
				`{"name":"rejected","type":"openai","api_key":"new-private-credential","encrypted_reasoning":`+policy+`,"models":[{"name":"m"}]}`)
			if w.Code != 400 || fixture.activations != count {
				t.Fatal("invalid create committed or attempted activation")
			}
			if _, err := fixture.store.GetProvider(context.Background(), "rejected"); !errors.Is(err, sql.ErrNoRows) {
				t.Fatal("invalid create left a row")
			}
			assertAdminReasoningProviderUnchanged(t, fixture.store, provider, models)
			after, _ := fixture.store.GetAlias(context.Background(), "strict-alias")
			if !reflect.DeepEqual(alias, after) {
				t.Fatal("invalid alias policy changed saved intent")
			}
		})
	}
	for _, fields := range []string{
		`"encrypted_reasoning":{},"encrypted_reasoning":null`,
		`"encrypted_reasoning":{},"Encrypted_Reasoning":null`,
		`"encrypted_reasoning_effective":null`, `"encrypted_reasoning_source":"root"`,
		`"encrypted_reasoning_inherited":true`, `"unknown":null`,
		`"database_managed":false`,
	} {
		for _, path := range []string{"/_internal/admin/providers/strict", "/_internal/admin/aliases/strict-alias"} {
			w, _ := adminReasoningRequest(t, fixture.handler, fixture.access, http.MethodPut, path, `{`+fields+`}`)
			if w.Code != 400 || w.Header().Get(catalogSavedHeader) != "" {
				t.Fatal("outer keys bypassed writable declaration validation")
			}
		}
	}
	for _, body := range []string{`null`, `{}` + `{}`, `{}` + `null`, `{}` + `false`} {
		w, _ := adminReasoningRequest(t, fixture.handler, fixture.access, http.MethodPut, "/_internal/admin/providers/strict", body)
		if w.Code != 400 || w.Header().Get(catalogSavedHeader) != "" {
			t.Fatal("non-object or trailing JSON body was accepted")
		}
	}
	for _, target := range []string{
		`{"provider":"strict","model":"m","encrypted_reasoning":{}}`,
		`{"provider":"strict","provider":"openai","model":"m"}`,
		`{"provider":"strict","model":"m","Model":"gpt-4o-mini"}`,
		`{"provider":"strict","model":"m","encrypted_reasoning_effective":null}`,
	} {
		w, _ := adminReasoningRequest(t, fixture.handler, fixture.access, http.MethodPut, "/_internal/admin/aliases/strict-alias", `{"targets":[`+target+`]}`)
		if w.Code != 400 {
			t.Fatal("alias target keys silently accepted a hidden or unsupported edit")
		}
	}
	assertAdminReasoningProviderUnchanged(t, fixture.store, provider, models)
}

func TestAdminEncryptedReasoningDerivativeFieldPresenceAndNames(t *testing.T) {
	fixture := newAdminReasoningFixture(t, adminReasoningStaticRuntime(t))
	requireAdminReasoningStatus(t, fixture, http.MethodPost, "/_internal/admin/providers",
		`{"name":"strict-child","type":"openai-compatible","api_key":"child-private-credential","extends":"base","models":[{"name":"m","encrypted_reasoning":{}}]}`, 201)
	before, models := adminReasoningSavedProvider(t, fixture.store, "strict-child")
	entries := []string{
		`{"name":"new","encrypted_reasoning":{}}`, `{"name":"new"}`, `{"name":"new","encrypted_reasoning":null}`,
		`{"name":"m","encrypted_reasoning":{}},{"name":"m","encrypted_reasoning":null}`,
		`{"name":"m"},{"name":"m"}`, `{"name":"m","Name":"reset","encrypted_reasoning":{}}`,
		`{"name":"m","encrypted_reasoning":{},"encrypted_reasoning":null}`,
	}
	for _, field := range []string{
		"display_name", "upstream_name", "protocol", "capabilities", "pricing", "unknown",
		"encrypted_reasoning_effective", "encrypted_reasoning_source", "encrypted_reasoning_inherited",
		"deferred_transport", "deferred_receiver", "DeFeRrEd_TrAnSpOrT",
	} {
		for _, value := range []string{`null`, `""`, `[]`, `{}`} {
			entries = append(entries, `{"name":"m","encrypted_reasoning":{},"`+field+`":`+value+`}`)
		}
	}
	for _, entries := range entries {
		t.Run(entries, func(t *testing.T) {
			count := fixture.activations
			for _, parent := range []string{"models", "Models", "model\u017f"} {
				w, _ := adminReasoningRequest(t, fixture.handler, fixture.access, http.MethodPut, "/_internal/admin/providers/strict-child",
					`{"`+parent+`":[`+entries+`]}`)
				if w.Code != 400 || w.Header().Get(catalogSavedHeader) != "" || fixture.activations != count {
					t.Fatalf("derivative redefinition or duplicate accepted: %d %s", w.Code, w.Body.String())
				}
				assertAdminReasoningProviderUnchanged(t, fixture.store, before, models)
			}
		})
	}
}

func TestAdminEncryptedReasoningSingleLevelBaseRestrictions(t *testing.T) {
	fixture := newAdminReasoningFixture(t, adminReasoningStaticRuntime(t))
	requireAdminReasoningStatus(t, fixture, http.MethodPost, "/_internal/admin/providers",
		`{"name":"managed-child","type":"openai-compatible","api_key":"child-private-credential","extends":"base"}`, 201)
	requireAdminReasoningStatus(t, fixture, http.MethodPost, "/_internal/admin/providers",
		`{"name":"disabled-base","type":"openai-compatible","base_url":"https://disabled.example/v1","api_key":"disabled-private-credential","enabled":false,"models":[{"name":"m"}]}`, 201)
	for _, base := range []string{"static-copy", "managed-child", "disabled-base", "undefined", "rejected-chain"} {
		count := fixture.activations
		w, _ := adminReasoningRequest(t, fixture.handler, fixture.access, http.MethodPost, "/_internal/admin/providers",
			`{"name":"rejected-chain","type":"openai-compatible","api_key":"new-private-credential","extends":"`+base+`","encrypted_reasoning":{}}`)
		if w.Code != 400 || w.Header().Get(catalogSavedHeader) != "" || fixture.activations != count {
			t.Fatalf("invalid inheritance base accepted: %s: %d %s", base, w.Code, w.Body.String())
		}
		if _, err := fixture.store.GetProvider(context.Background(), "rejected-chain"); !errors.Is(err, sql.ErrNoRows) {
			t.Fatal("rejected chain persisted")
		}
	}
	w, _ := adminReasoningRequest(t, fixture.handler, fixture.access, http.MethodPost, "/_internal/admin/providers",
		`{"name":"wrong-type","type":"openai","api_key":"wrong-private-credential","extends":"base"}`)
	if w.Code != 400 || w.Header().Get(catalogSavedHeader) != "" {
		t.Fatal("base type mismatch was accepted")
	}
}
