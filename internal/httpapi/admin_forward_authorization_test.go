package httpapi

import (
	"context"
	"net/http"
	"testing"

	"github.com/egose/aiproxy/internal/store"
)

func TestAdminForwardAuthorizationPersistence(t *testing.T) {
	st := openValidationStore(t)
	h, access := validationTestHandler(t, st)
	name := uniqName(t, "forward-auth")
	body := validProviderBody(name)
	delete(body, "api_key")
	body["forward_authorization"] = true
	code, _, raw := doAdmin(t, h, access, http.MethodPost, "/_internal/admin/providers", body)
	if code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", code, raw)
	}
	row, err := st.GetProvider(context.Background(), name)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.DeleteProvider(context.Background(), row.ID) })
	if !row.ForwardAuthorization || len(row.APIKeyEncrypted) != 0 {
		t.Fatal("authorization mode not persisted")
	}
	code, _, raw = doAdmin(t, h, access, http.MethodPut, "/_internal/admin/providers/"+name, map[string]any{"display_name": "Preserved"})
	if code != http.StatusOK {
		t.Fatalf("update status=%d body=%s", code, raw)
	}
	row, err = st.GetProvider(context.Background(), name)
	if err != nil || !row.ForwardAuthorization {
		t.Fatal("omitted flag was not preserved")
	}
	code, _, _ = doAdmin(t, h, access, http.MethodPut, "/_internal/admin/providers/"+name, map[string]any{"api_key": "conflict"})
	if code != http.StatusBadRequest {
		t.Fatalf("credential conflict status=%d, want 400", code)
	}
	row, err = st.GetProvider(context.Background(), name)
	if err != nil || len(row.APIKeyEncrypted) != 0 {
		t.Fatal("rejected update changed credentials")
	}
	code, _, raw = doAdmin(t, h, access, http.MethodPut, "/_internal/admin/providers/"+name, map[string]any{"forward_authorization": false, "api_key": "configured"})
	if code != http.StatusOK {
		t.Fatalf("disable status=%d body=%s", code, raw)
	}
	row, err = st.GetProvider(context.Background(), name)
	if err != nil || row.ForwardAuthorization || len(row.APIKeyEncrypted) == 0 {
		t.Fatal("authorization mode not disabled")
	}
	code, _, raw = doAdmin(t, h, access, http.MethodPut, "/_internal/admin/providers/"+name, map[string]any{"forward_authorization": true, "api_key": "", "api_key_ref": map[string]string{"key": "", "path": ""}})
	if code != http.StatusOK {
		t.Fatalf("switch status=%d body=%s", code, raw)
	}
}

func TestApplyProviderUpsertPreservesForwardAuthorization(t *testing.T) {
	h := &Handler{}
	row := store.DBProvider{ForwardAuthorization: true}
	if err := h.applyProviderUpsert("test", &row, adminProviderUpsert{}); err != nil || !row.ForwardAuthorization {
		t.Fatal("omitted flag was not preserved")
	}
	forward := false
	if err := h.applyProviderUpsert("test", &row, adminProviderUpsert{ForwardAuthorization: &forward}); err != nil || row.ForwardAuthorization {
		t.Fatal("explicit false did not disable forwarding")
	}
}
