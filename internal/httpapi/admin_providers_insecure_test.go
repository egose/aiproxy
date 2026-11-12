package httpapi

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/store"
)

func TestAdminProviderAllowInsecureBaseURL(t *testing.T) {
	st := openValidationStore(t)
	h, access := validationTestHandler(t, st)
	ctx := context.Background()
	name := uniqName(t, "insecure")

	code, _, raw := doAdmin(t, h, access, http.MethodPost, "/_internal/admin/providers", map[string]interface{}{
		"name": name, "type": "openai-compatible",
		"base_url":                "http://llm.internal.svc.cluster.local/v1",
		"allow_insecure_base_url": true,
		"api_key":                 "secret",
		"models":                  []interface{}{map[string]interface{}{"name": "m"}},
	})
	if code != http.StatusCreated {
		t.Fatalf("create with flag status = %d, want 201: %s", code, raw)
	}
	t.Cleanup(func() {
		if p, err := st.GetProvider(ctx, name); err == nil {
			_ = st.DeleteProvider(ctx, p.ID)
		}
	})

	code, view, _ := doAdmin(t, h, access, http.MethodGet, "/_internal/admin/providers/"+name, nil)
	if code != http.StatusOK {
		t.Fatalf("get status = %d", code)
	}
	if flag, _ := view["allow_insecure_base_url"].(bool); !flag {
		t.Fatalf("view allow_insecure_base_url = %v, want true", view["allow_insecure_base_url"])
	}

	code, _, _ = doAdmin(t, h, access, http.MethodPut, "/_internal/admin/providers/"+name, map[string]interface{}{
		"base_url":                "https://example.com/v1",
		"allow_insecure_base_url": false,
	})
	if code != http.StatusOK {
		t.Fatalf("update status = %d", code)
	}
	code, view, _ = doAdmin(t, h, access, http.MethodGet, "/_internal/admin/providers/"+name, nil)
	if flag, _ := view["allow_insecure_base_url"].(bool); flag {
		t.Fatal("view allow_insecure_base_url = true after disabling, want false")
	}
}

func TestAdminCreateProviderRejectsInsecureWithoutFlag(t *testing.T) {
	st := openValidationStore(t)
	h, access := validationTestHandler(t, st)
	code, _, raw := doAdmin(t, h, access, http.MethodPost, "/_internal/admin/providers", map[string]interface{}{
		"name": uniqName(t, "insecure"), "type": "openai-compatible",
		"base_url": "http://llm.internal.svc.cluster.local/v1",
		"api_key":  "secret",
		"models":   []interface{}{map[string]interface{}{"name": "m"}},
	})
	if code != http.StatusBadRequest || !strings.Contains(raw, "non-HTTPS base_url") {
		t.Fatalf("status = %d body = %q, want loopback rejection", code, raw)
	}
}

func TestAdminProviderViewReportsStaticAllowInsecureBaseURL(t *testing.T) {
	st := openValidationStore(t)
	h, access := validationTestHandler(t, st)
	deps := h.current()
	deps.Catalog = config.NewCatalog([]config.Provider{
		{
			Type: config.ProviderTypeOpenAICompatible, Name: "static-insecure",
			BaseURL: "http://llm.internal.svc.cluster.local/v1", AllowInsecureBaseURL: true,
			APIKey: "sk", Models: []config.Model{{Name: "m", UpstreamName: "m"}},
		},
	}, nil, nil)
	h.UpdateDependencies(deps)

	code, body, _ := doAdmin(t, h, access, http.MethodGet, "/_internal/admin/providers", nil)
	if code != http.StatusOK {
		t.Fatalf("list status = %d", code)
	}
	providers, _ := body["providers"].([]interface{})
	for _, entry := range providers {
		view, _ := entry.(map[string]interface{})
		if view["name"] == "static-insecure" {
			if flag, _ := view["allow_insecure_base_url"].(bool); !flag {
				t.Fatalf("static view allow_insecure_base_url = %v, want true", view["allow_insecure_base_url"])
			}
			return
		}
	}
	t.Fatal("static provider missing from merged view")
}

func TestApplyProviderUpsertRejectsDerivedAllowInsecureBaseURL(t *testing.T) {
	h := &Handler{}
	for _, existing := range []bool{false, true} {
		for _, flag := range []bool{false, true} {
			row := store.DBProvider{Name: "derived", Type: "openai"}
			req := adminProviderUpsert{AllowInsecureBaseURL: &flag}
			extends := "openai"
			if existing {
				row.Extends = extends
			} else {
				req.Extends = &extends
			}
			before := row
			err := h.applyProviderUpsert(row.Name, &row, req)
			if err == nil || !strings.Contains(err.Error(), "derived provider cannot declare allow_insecure_base_url") {
				t.Fatalf("existing=%t flag=%t: expected rejection, got %v", existing, flag, err)
			}
			if row.Extends != before.Extends || row.AllowInsecureBaseURL != before.AllowInsecureBaseURL {
				t.Fatal("rejected declaration mutated the candidate row")
			}
		}
	}
}

func TestApplyProviderUpsertClearsInheritedTransportOverride(t *testing.T) {
	h := &Handler{}
	extends := "openai"
	row := store.DBProvider{Name: "derived", Type: "openai", AllowInsecureBaseURL: true}
	if err := h.applyProviderUpsert(row.Name, &row, adminProviderUpsert{Extends: &extends}); err != nil {
		t.Fatal(err)
	}
	if row.Extends != extends || row.AllowInsecureBaseURL {
		t.Fatal("becoming derived must clear the omitted local transport override")
	}
	concrete, flag := "", true
	if err := h.applyProviderUpsert(row.Name, &row, adminProviderUpsert{Extends: &concrete, AllowInsecureBaseURL: &flag}); err != nil {
		t.Fatal(err)
	}
	if row.Extends != "" || !row.AllowInsecureBaseURL {
		t.Fatal("becoming concrete must allow an explicit transport opt-in")
	}
}

func TestAdminDerivedProviderRejectsLocalAllowInsecureBaseURL(t *testing.T) {
	st := openValidationStore(t)
	h, access := validationTestHandler(t, st)
	ctx := context.Background()
	name := uniqName(t, "derived-insecure")
	code, _, raw := doAdmin(t, h, access, http.MethodPost, "/_internal/admin/providers", map[string]interface{}{
		"name": name, "type": "openai", "extends": "openai", "api_key": "secret",
	})
	if code != http.StatusCreated {
		t.Fatalf("create derived: %d %s", code, raw)
	}
	t.Cleanup(func() {
		if p, err := st.GetProvider(ctx, name); err == nil {
			_ = st.DeleteProvider(ctx, p.ID)
		}
	})
	for _, flag := range []bool{false, true} {
		badName := uniqName(t, "invalid-derived")
		code, _, raw = doAdmin(t, h, access, http.MethodPost, "/_internal/admin/providers", map[string]interface{}{
			"name": badName, "type": "openai", "extends": "openai", "api_key": "secret", "allow_insecure_base_url": flag,
		})
		if code != http.StatusBadRequest || !strings.Contains(raw, "derived provider cannot declare allow_insecure_base_url") {
			t.Fatalf("create flag=%t: %d %s", flag, code, raw)
		}
		if p, err := st.GetProvider(ctx, badName); err == nil {
			_ = st.DeleteProvider(ctx, p.ID)
			t.Fatal("invalid derived declaration was persisted")
		}
		code, _, raw = doAdmin(t, h, access, http.MethodPut, "/_internal/admin/providers/"+name, map[string]interface{}{"allow_insecure_base_url": flag})
		if code != http.StatusBadRequest || !strings.Contains(raw, "derived provider cannot declare allow_insecure_base_url") {
			t.Fatalf("update flag=%t: %d %s", flag, code, raw)
		}
		row, err := st.GetProvider(ctx, name)
		if err != nil || row.AllowInsecureBaseURL {
			t.Fatalf("rejected update changed the row: flag=%t err=%v", row.AllowInsecureBaseURL, err)
		}
	}
}

func TestAdminProviderBecomingDerivedClearsLocalAllowInsecureBaseURL(t *testing.T) {
	st := openValidationStore(t)
	h, access := validationTestHandler(t, st)
	ctx := context.Background()
	name := uniqName(t, "insecure-transition")
	code, _, raw := doAdmin(t, h, access, http.MethodPost, "/_internal/admin/providers", map[string]interface{}{
		"name": name, "type": "openai", "api_key": "secret",
		"base_url": "http://llm.internal/v1", "allow_insecure_base_url": true,
		"models": []interface{}{map[string]interface{}{"name": "m"}},
	})
	if code != http.StatusCreated {
		t.Fatalf("create concrete: %d %s", code, raw)
	}
	t.Cleanup(func() {
		if p, err := st.GetProvider(ctx, name); err == nil {
			_ = st.DeleteProvider(ctx, p.ID)
		}
	})
	code, _, raw = doAdmin(t, h, access, http.MethodPut, "/_internal/admin/providers/"+name, map[string]interface{}{"extends": "openai"})
	if code != http.StatusOK {
		t.Fatalf("become derived: %d %s", code, raw)
	}
	row, err := st.GetProvider(ctx, name)
	if err != nil || row.Extends != "openai" || row.AllowInsecureBaseURL {
		t.Fatalf("transition did not clear the persisted flag: extends=%s flag=%t err=%v", row.Extends, row.AllowInsecureBaseURL, err)
	}
}
