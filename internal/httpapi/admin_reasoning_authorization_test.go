package httpapi

import (
	"context"
	"net/http"
	"reflect"
	"testing"

	"github.com/egose/aiproxy/internal/store"
)

func TestAdminEncryptedReasoningWorkspaceAuthorizationAndStaticRestrictions(t *testing.T) {
	fixture := newAdminReasoningFixture(t, nil)
	st, h, global := fixture.store, fixture.handler, fixture.access
	workspace := mkWorkspace(t, st, h, global, "reasoning-workspace")
	other := mkWorkspace(t, st, h, global, "reasoning-other")
	_, owner := mkWorkspaceMember(t, st, h, global, "reasoning-owner@example.test", workspace, "admin")
	_, member := mkWorkspaceMember(t, st, h, global, "reasoning-member@example.test", workspace, "member")
	_, outsider := mkWorkspaceMember(t, st, h, global, "reasoning-outsider@example.test", other, "admin")
	w, _ := adminReasoningRequest(t, h, owner, http.MethodPost, "/_internal/admin/providers",
		`{"name":"authorized","workspace_id":"`+workspace+`","type":"openai","api_key":"authorized-private-credential","encrypted_reasoning":{},"models":[{"name":"m","encrypted_reasoning":{"passthrough":false}}]}`)
	if w.Code != 201 || w.Header().Get(catalogSavedHeader) != "true" {
		t.Fatalf("workspace admin could not create policy declarations: %d %s", w.Code, w.Body.String())
	}
	w, _ = adminReasoningRequest(t, h, owner, http.MethodPost, "/_internal/admin/aliases",
		`{"name":"authorized-alias","workspace_id":"`+workspace+`","encrypted_reasoning":{},"targets":[{"provider":"authorized","model":"m"}]}`)
	if w.Code != 201 {
		t.Fatal(w.Body.String())
	}
	for _, access := range []string{global, owner} {
		w, view := adminReasoningRequest(t, h, access, http.MethodPut, "/_internal/admin/providers/authorized",
			`{"encrypted_reasoning":{"on_caller_mismatch":"strip_and_retry"},"models":[{"name":"m","encrypted_reasoning":{}}]}`)
		if w.Code != 200 {
			t.Fatal(w.Body.String())
		}
		assertAdminReasoningView(t, view, `{"on_caller_mismatch":"strip_and_retry"}`, `{"on_caller_mismatch":"strip_and_retry"}`, "provider", false)
		w, _ = adminReasoningRequest(t, h, access, http.MethodPut, "/_internal/admin/aliases/authorized-alias",
			`{"encrypted_reasoning":{"passthrough":false}}`)
		if w.Code != 200 {
			t.Fatal(w.Body.String())
		}
	}
	provider, models := adminReasoningSavedProvider(t, st, "authorized")
	alias, _ := st.GetAlias(context.Background(), "authorized-alias")
	for _, access := range []string{member, outsider, ""} {
		count := fixture.activations
		for _, operation := range []struct {
			path, body string
		}{
			{"/_internal/admin/providers/authorized", `{"encrypted_reasoning":null,"models":[{"name":"m","encrypted_reasoning":{"passthrough":false}}]}`},
			{"/_internal/admin/providers/authorized/credential", `{"api_key":"rejected-private-credential"}`},
			{"/_internal/admin/aliases/authorized-alias", `{"encrypted_reasoning":null}`},
		} {
			w, _ := adminReasoningRequest(t, h, access, http.MethodPut, operation.path, operation.body)
			if w.Code < 400 || w.Header().Get(catalogSavedHeader) != "" || fixture.activations != count {
				t.Fatalf("unauthorized policy/credential edit committed: %d %s", w.Code, w.Body.String())
			}
		}
		assertAdminReasoningProviderUnchanged(t, st, provider, models)
		after, _ := st.GetAlias(context.Background(), "authorized-alias")
		if !reflect.DeepEqual(alias, after) {
			t.Fatal("unauthorized alias mutation changed policy intent")
		}
	}
	w, view := adminReasoningRequest(t, h, member, http.MethodGet, "/_internal/admin/providers/authorized", "")
	if w.Code != 200 {
		t.Fatal("authorized read-only member could not read saved policy metadata")
	}
	assertAdminReasoningView(t, adminReasoningModelView(t, view, "m"), "{}", "{}", "model", false)
	if w, _ := adminReasoningRequest(t, h, outsider, http.MethodGet, "/_internal/admin/providers/authorized", ""); w.Code != 404 {
		t.Fatal("workspace-private policy metadata leaked through the active catalog")
	}
	if w, _ := adminReasoningRequest(t, h, outsider, http.MethodGet, "/_internal/admin/aliases/authorized-alias", ""); w.Code != 404 {
		t.Fatal("workspace-private alias intent leaked through the active catalog")
	}
	if w, _ := adminReasoningRequest(t, h, global, http.MethodGet, "/_internal/admin/providers?workspace_id="+store.SystemWorkspaceID.String(), ""); w.Code != 200 {
		t.Fatal(w.Body.String())
	} else {
		var list map[string]any
		list = requireAdminReasoningStatus(t, fixture, http.MethodGet, "/_internal/admin/providers?workspace_id="+store.SystemWorkspaceID.String(), "", 200)
		if listNames(list, "providers")["authorized"] {
			t.Fatal("database provider was re-exposed as a static system resource")
		}
	}
	list := requireAdminReasoningStatus(t, fixture, http.MethodGet, "/_internal/admin/aliases?workspace_id="+store.SystemWorkspaceID.String(), "", 200)
	if listNames(list, "aliases")["authorized-alias"] {
		t.Fatal("database alias was re-exposed as a static system resource")
	}
	for _, access := range []string{global, owner, member} {
		count := fixture.activations
		for _, path := range []string{"/_internal/admin/providers/openai", "/_internal/admin/providers/openai/credential", "/_internal/admin/aliases/chat_default"} {
			w, _ := adminReasoningRequest(t, h, access, http.MethodPut, path, `{"encrypted_reasoning":{}}`)
			if w.Code < 400 || w.Header().Get(catalogSavedHeader) != "" || fixture.activations != count {
				t.Fatal("static target became administratively writable")
			}
		}
	}
}
