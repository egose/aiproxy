package dbmerge

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/store"
)

func testDBURL(t *testing.T) string {
	t.Helper()
	dbURL := os.Getenv("AIPROXY_TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("AIPROXY_TEST_DATABASE_URL not set")
	}
	return dbURL
}

func testBases() map[string]config.Provider {
	return map[string]config.Provider{
		"base": {
			Type: "openai", Name: "base", BaseURL: "https://api.openai.com/v1",
			DisplayName: "Base", APIKey: "base-key", Enabled: true,
			UpstreamHeaderTimeout: 90 * time.Second,
			Models:                []config.Model{{Name: "gpt-4o-mini", UpstreamName: "gpt-4o-mini"}},
			ModelByName: map[string]config.Model{
				"gpt-4o-mini": {Name: "gpt-4o-mini", UpstreamName: "gpt-4o-mini"},
			},
		},
	}
}

func TestBuildProviderDirect(t *testing.T) {
	t.Setenv("AIPROXY_DB_ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef")
	enc, err := store.EncryptSecret([]byte("live-key"))
	if err != nil {
		t.Fatal(err)
	}
	timeout := int64(30000)
	row := store.DBProvider{
		Name: "db-openai", Type: "openai", DisplayName: "DB",
		BaseURL: "https://api.openai.com/v1", UpstreamTimeoutMs: &timeout,
		UserAgent: "test/1.0", ForwardHeaders: []string{"x-custom"},
		APIKeyEncrypted: enc, Enabled: true,
	}
	models := []store.DBProviderModel{
		{Name: "gpt-4o-mini", Capabilities: []string{"chat"}},
	}
	p, err := BuildProvider(row, models, testBases())
	if err != nil {
		t.Fatalf("BuildProvider = %v", err)
	}
	if p.APIKey != "live-key" {
		t.Errorf("APIKey = %q, want decrypted value", p.APIKey)
	}
	if p.UpstreamHeaderTimeout != 30*time.Second {
		t.Errorf("timeout = %v, want 30s", p.UpstreamHeaderTimeout)
	}
	if len(p.Models) != 1 || p.Models[0].UpstreamName != "gpt-4o-mini" {
		t.Errorf("models = %+v, want upstream defaulted", p.Models)
	}
	if _, ok := p.ModelByName["gpt-4o-mini"]; !ok {
		t.Errorf("ModelByName missing entry")
	}
	if err := config.ValidateDynamicProvider(p); err != nil {
		t.Errorf("built provider fails validation: %v", err)
	}
}

func TestBuildProviderExtends(t *testing.T) {
	t.Setenv("AIPROXY_DB_ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef")
	enc, err := store.EncryptSecret([]byte("derived-key"))
	if err != nil {
		t.Fatal(err)
	}
	row := store.DBProvider{Name: "derived", Type: "openai", Extends: "base", APIKeyEncrypted: enc, Enabled: false}
	p, err := BuildProvider(row, nil, testBases())
	if err != nil {
		t.Fatalf("BuildProvider = %v", err)
	}
	if p.BaseURL != "https://api.openai.com/v1" {
		t.Errorf("BaseURL = %q, want inherited", p.BaseURL)
	}
	if len(p.Models) != 1 {
		t.Errorf("models = %d, want inherited base models", len(p.Models))
	}
	if p.APIKey != "derived-key" {
		t.Errorf("APIKey = %q, want row credential", p.APIKey)
	}
	if p.Enabled {
		t.Errorf("Enabled = true, want row flag honored")
	}
	if p.Name != "derived" || p.DisplayName != "Base" {
		t.Errorf("name/display = %q/%q", p.Name, p.DisplayName)
	}
}

func TestBuildProviderExtendsErrors(t *testing.T) {
	row := store.DBProvider{Name: "x", Type: "openai", Extends: "ghost", Enabled: true}
	if _, err := BuildProvider(row, nil, testBases()); err == nil {
		t.Errorf("missing base err = nil")
	}
	row = store.DBProvider{Name: "base", Type: "openai", Extends: "base", Enabled: true}
	if _, err := BuildProvider(row, nil, testBases()); err == nil {
		t.Errorf("self extends err = nil")
	}
	row = store.DBProvider{Name: "x", Type: "anthropic", Extends: "base", Enabled: true}
	if _, err := BuildProvider(row, nil, testBases()); err == nil {
		t.Errorf("type mismatch err = nil")
	}
}

func TestBuildProviderBadTimeout(t *testing.T) {
	zero := int64(0)
	row := store.DBProvider{Name: "x", Type: "openai", UpstreamTimeoutMs: &zero, Enabled: false}
	if _, err := BuildProvider(row, nil, testBases()); err == nil {
		t.Errorf("zero timeout err = nil")
	}
}

func TestMergeCatalogEndToEnd(t *testing.T) {
	dbURL := testDBURL(t)
	t.Setenv("AIPROXY_DB_ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef")
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	provName := "merge-db-openai-" + suffix
	aliasName := "merge-db-alias-" + suffix
	keyName := "merge-db-key-" + suffix
	ctx := context.Background()
	st, err := store.Open(ctx, dbURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if _, err := st.MigrateUp(ctx); err != nil {
		t.Fatal(err)
	}
	enc, err := store.EncryptSecret([]byte("db-key"))
	if err != nil {
		t.Fatal(err)
	}
	providerRow := &store.DBProvider{Name: provName, Type: "openai", APIKeyEncrypted: enc, Enabled: true}
	if err := st.CreateProvider(ctx, providerRow); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if p, err := st.GetProvider(context.Background(), provName); err == nil {
			_ = st.DeleteProvider(context.Background(), p.ID)
		}
		if a, err := st.GetAlias(context.Background(), aliasName); err == nil {
			_ = st.DeleteAlias(context.Background(), a.ID)
		}
	})
	if err := st.ReplaceProviderModels(ctx, providerRow.ID, []store.DBProviderModel{
		{Name: "gpt-4o-mini", Capabilities: []string{"chat"}},
	}); err != nil {
		t.Fatal(err)
	}
	aliasRow := &store.DBAlias{Name: aliasName, Algorithm: "round_robin", RetryStatusCodes: []int{500}}
	if err := st.CreateAlias(ctx, aliasRow, []store.DBAliasTarget{{Provider: provName, Model: "gpt-4o-mini"}}); err != nil {
		t.Fatal(err)
	}
	keyRow := &store.InboundKey{Name: keyName, TokenHash: "merge-hash-" + suffix, TokenPrefix: "merge", Enabled: true}
	if err := st.CreateInboundKey(ctx, keyRow); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		rows, _ := st.ListInboundKeys(context.Background())
		for _, k := range rows {
			if k.Name == keyName {
				_ = st.DeleteInboundKey(context.Background(), k.ID)
			}
		}
	})
	static := config.NewCatalog([]config.Provider{
		{
			Type: "openai", Name: "static-openai", BaseURL: "https://api.openai.com/v1",
			APIKey: "static", Enabled: true,
			Models:      []config.Model{{Name: "gpt-4o", UpstreamName: "gpt-4o"}},
			ModelByName: map[string]config.Model{"gpt-4o": {Name: "gpt-4o", UpstreamName: "gpt-4o"}},
		},
	}, nil, nil)
	merged, err := MergeCatalog(ctx, st, static)
	if err != nil {
		t.Fatalf("MergeCatalog = %v", err)
	}
	names := map[string]bool{}
	for _, p := range merged.Providers {
		names[p.Name] = true
		if p.Name == provName && p.APIKey != "db-key" {
			t.Errorf("merged APIKey = %q, want decrypted", p.APIKey)
		}
	}
	if !names[provName] || !names["static-openai"] {
		t.Errorf("merged providers = %v, want both", names)
	}
	found := false
	for _, a := range merged.Aliases {
		if a.Name == aliasName {
			found = true
			if len(a.Targets) != 1 || len(a.RetryStatusCodes) != 1 {
				t.Errorf("merged alias = %+v", a)
			}
		}
	}
	if !found {
		t.Errorf("merged aliases missing db alias")
	}
	foundKey := false
	for _, k := range merged.Keys {
		if k.Name == keyName {
			foundKey = true
		}
	}
	if !foundKey {
		t.Errorf("merged keys missing db key %q = %+v", keyName, merged.Keys)
	}
}
