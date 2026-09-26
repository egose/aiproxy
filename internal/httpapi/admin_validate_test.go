package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/adminauth"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/store"
)

var validateTestSeq atomic.Int64

func openValidationStore(t *testing.T) *store.Store {
	t.Helper()
	dbURL := os.Getenv("AIPROXY_TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("AIPROXY_TEST_DATABASE_URL not set")
	}
	t.Setenv("AIPROXY_JWT_SECRET", "test-jwt-secret-1234567890")
	t.Setenv("AIPROXY_DB_ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef")
	ctx := context.Background()
	st, err := store.Open(ctx, dbURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if _, err := st.MigrateUp(ctx); err != nil {
		t.Fatal(err)
	}
	return st
}

func validationTestHandler(t *testing.T, st *store.Store) (*Handler, string) {
	t.Helper()
	rt := newRT()
	deps := newWebUIDeps(false)
	deps.Catalog = rt.Catalog
	deps.MultiTenancy = config.MultiTenancy{Enabled: true}
	deps.AdminStore = st
	h := NewHandler(deps)
	access, _, err := adminauth.IssueAccess("admin-1", "admin@example.com", true)
	if err != nil {
		t.Fatal(err)
	}
	return h, access
}

func doAdmin(t *testing.T, h *Handler, access, method, path string, body interface{}) (int, map[string]interface{}, string) {
	t.Helper()
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Authorization", "Bearer "+access)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	raw := w.Body.String()
	var parsed map[string]interface{}
	if len(w.Body.Bytes()) > 0 {
		_ = json.Unmarshal(w.Body.Bytes(), &parsed)
	}
	if parsed == nil {
		parsed = map[string]interface{}{}
	}
	return w.Code, parsed, raw
}

func uniqName(t *testing.T, prefix string) string {
	t.Helper()
	return fmt.Sprintf("%s-%d-%d", prefix, time.Now().UnixNano(), validateTestSeq.Add(1))
}

func validProviderBody(name string) map[string]interface{} {
	return map[string]interface{}{
		"name":    name,
		"type":    "openai",
		"api_key": "secret",
		"models":  []interface{}{map[string]interface{}{"name": "gpt-4o-mini"}},
	}
}

func TestAdminCreateProviderValidation(t *testing.T) {
	st := openValidationStore(t)
	h, access := validationTestHandler(t, st)

	cases := []struct {
		name   string
		mutate func(map[string]interface{})
		want   string
	}{
		{"unknown type", func(b map[string]interface{}) { b["type"] = "nope" }, "unknown provider type"},
		{"reserved name", func(b map[string]interface{}) { b["name"] = "alias" }, "reserved"},
		{"static collision", func(b map[string]interface{}) { b["name"] = "openai" }, "static config"},
		{"bad name", func(b map[string]interface{}) { b["name"] = "Bad Name" }, "lowercase"},
		{"compatible needs base url", func(b map[string]interface{}) { b["type"] = "openai-compatible" }, "base_url is required"},
		{"bad base url", func(b map[string]interface{}) { b["base_url"] = "http://example.com" }, "loopback"},
		{"copilot with api key", func(b map[string]interface{}) {
			b["type"] = "github-copilot"
			b["credential_ref"] = map[string]interface{}{"name": "c"}
		}, "not supported by github-copilot"},
		{"copilot without ref", func(b map[string]interface{}) { b["type"] = "github-copilot" }, "credential_ref"},
		{"missing credential", func(b map[string]interface{}) { delete(b, "api_key") }, "require a non-empty api_key"},
		{"zero models", func(b map[string]interface{}) { b["models"] = []interface{}{} }, "at least one model"},
		{"bad model name", func(b map[string]interface{}) {
			b["models"] = []interface{}{map[string]interface{}{"name": "Bad"}}
		}, "each '/'-separated segment"},
		{"protocol on openai", func(b map[string]interface{}) {
			b["models"] = []interface{}{map[string]interface{}{"name": "m", "protocol": "chat"}}
		}, "only supported by opencode-zen"},
		{"bad capability", func(b map[string]interface{}) {
			b["models"] = []interface{}{map[string]interface{}{"name": "m", "capabilities": []string{"nope"}}}
		}, "invalid capability"},
		{"unsupported capability", func(b map[string]interface{}) {
			b["type"] = "anthropic"
			b["models"] = []interface{}{map[string]interface{}{"name": "m", "capabilities": []string{"images"}}}
		}, "not supported by provider type"},
		{"negative pricing", func(b map[string]interface{}) {
			b["models"] = []interface{}{map[string]interface{}{"name": "m", "pricing": map[string]float64{"input_per_million": -1}}}
		}, "must not be negative"},
		{"unknown pricing rate", func(b map[string]interface{}) {
			b["models"] = []interface{}{map[string]interface{}{"name": "m", "pricing": map[string]float64{"nope": 1}}}
		}, "unknown pricing rate"},
		{"extends missing base", func(b map[string]interface{}) { b["extends"] = "ghost" }, "not defined"},
		{"extends self", func(b map[string]interface{}) { b["extends"] = b["name"] }, "references itself"},
	}
	for _, tc := range cases {
		name := uniqName(t, "vprov")
		body := validProviderBody(name)
		if strings.Contains(tc.want, "references itself") {
			body["extends"] = name
		} else {
			tc.mutate(body)
		}
		code, _, raw := doAdmin(t, h, access, http.MethodPost, "/_internal/admin/providers", body)
		if code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", tc.name, code)
			continue
		}
		if !strings.Contains(raw, tc.want) {
			t.Errorf("%s: body = %q, want substring %q", tc.name, raw, tc.want)
		}
	}
}

func TestAdminCreateProviderAcceptedShapes(t *testing.T) {
	st := openValidationStore(t)
	h, access := validationTestHandler(t, st)
	ctx := context.Background()

	zen := uniqName(t, "zen")
	code, _, _ := doAdmin(t, h, access, http.MethodPost, "/_internal/admin/providers", map[string]interface{}{
		"name": zen, "type": "opencode-zen",
		"models": []interface{}{map[string]interface{}{"name": "m", "protocol": "chat"}},
	})
	if code != http.StatusCreated {
		t.Fatalf("zen keyless status = %d, want 201", code)
	}
	t.Cleanup(func() {
		if p, err := st.GetProvider(ctx, zen); err == nil {
			_ = st.DeleteProvider(ctx, p.ID)
		}
	})

	copilot := uniqName(t, "copilot")
	code, _, _ = doAdmin(t, h, access, http.MethodPost, "/_internal/admin/providers", map[string]interface{}{
		"name": copilot, "type": "github-copilot",
		"credential_ref": map[string]interface{}{"name": "c"},
		"models":         []interface{}{map[string]interface{}{"name": "m"}},
	})
	if code != http.StatusCreated {
		t.Fatalf("copilot status = %d, want 201", code)
	}
	t.Cleanup(func() {
		if p, err := st.GetProvider(ctx, copilot); err == nil {
			_ = st.DeleteProvider(ctx, p.ID)
		}
	})

	dotted := uniqName(t, "my.alias")
	code, _, _ = doAdmin(t, h, access, http.MethodPost, "/_internal/admin/providers", map[string]interface{}{
		"name": dotted, "type": "openai", "api_key": "secret",
		"models": []interface{}{map[string]interface{}{"name": "m"}},
	})
	if code != http.StatusCreated {
		t.Fatalf("dotted name status = %d, want 201", code)
	}
	t.Cleanup(func() {
		if p, err := st.GetProvider(ctx, dotted); err == nil {
			_ = st.DeleteProvider(ctx, p.ID)
		}
	})
}

func TestAdminUpdateProviderPreservesFields(t *testing.T) {
	st := openValidationStore(t)
	h, access := validationTestHandler(t, st)
	ctx := context.Background()
	name := uniqName(t, "uprov")

	code, _, _ := doAdmin(t, h, access, http.MethodPost, "/_internal/admin/providers", map[string]interface{}{
		"name": name, "type": "openai", "api_key": "secret",
		"display_name": "Nice", "base_url": "https://example.com/v1",
		"models": []interface{}{map[string]interface{}{"name": "m"}},
	})
	if code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201", code)
	}
	t.Cleanup(func() {
		if p, err := st.GetProvider(ctx, name); err == nil {
			_ = st.DeleteProvider(ctx, p.ID)
		}
	})

	code, body, _ := doAdmin(t, h, access, http.MethodPut, "/_internal/admin/providers/"+name, map[string]interface{}{
		"enabled": false,
	})
	if code != http.StatusOK {
		t.Fatalf("update status = %d, want 200", code)
	}
	if body["display_name"] != "Nice" {
		t.Errorf("display_name = %v, want Nice (must survive partial update)", body["display_name"])
	}
	if body["base_url"] != "https://example.com/v1" {
		t.Errorf("base_url = %v, want preserved", body["base_url"])
	}
	if body["enabled"] != false {
		t.Errorf("enabled = %v, want false", body["enabled"])
	}

	code, _, _ = doAdmin(t, h, access, http.MethodPut, "/_internal/admin/providers/"+name, map[string]interface{}{
		"type": "nope",
	})
	if code != http.StatusBadRequest {
		t.Errorf("bad type update status = %d, want 400", code)
	}
}

func TestAdminCredentialEndpointValidation(t *testing.T) {
	st := openValidationStore(t)
	h, access := validationTestHandler(t, st)
	ctx := context.Background()
	name := uniqName(t, "cprov")

	code, _, _ := doAdmin(t, h, access, http.MethodPost, "/_internal/admin/providers", validProviderBody(name))
	if code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201", code)
	}
	t.Cleanup(func() {
		if p, err := st.GetProvider(ctx, name); err == nil {
			_ = st.DeleteProvider(ctx, p.ID)
		}
	})

	code, _, _ = doAdmin(t, h, access, http.MethodPut, "/_internal/admin/providers/"+name+"/credential", map[string]interface{}{
		"api_key": "new", "api_key_ref_key": "k",
	})
	if code != http.StatusBadRequest {
		t.Errorf("api_key+ref status = %d, want 400", code)
	}

	code, _, _ = doAdmin(t, h, access, http.MethodPut, "/_internal/admin/providers/"+name+"/credential", map[string]interface{}{
		"api_key": "new-secret",
	})
	if code != http.StatusOK {
		t.Errorf("rotate status = %d, want 200", code)
	}
}

func TestAdminProviderTypesEndpoint(t *testing.T) {
	st := openValidationStore(t)
	h, access := validationTestHandler(t, st)

	code, body, _ := doAdmin(t, h, access, http.MethodGet, "/_internal/admin/provider-types", nil)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	items, ok := body["provider_types"].([]interface{})
	if !ok || len(items) != 8 {
		t.Fatalf("provider_types has %d items, want 8", len(items))
	}
	byType := map[string]map[string]interface{}{}
	for _, item := range items {
		m := item.(map[string]interface{})
		byType[m["type"].(string)] = m
	}
	if byType["openai-compatible"]["requires_base_url"] != true {
		t.Errorf("openai-compatible must require base_url")
	}
	if byType["github-copilot"]["credential"] != "credential_ref" {
		t.Errorf("copilot credential = %v", byType["github-copilot"]["credential"])
	}
	if byType["opencode-zen"]["credential"] != "optional" {
		t.Errorf("zen credential = %v", byType["opencode-zen"]["credential"])
	}
}
