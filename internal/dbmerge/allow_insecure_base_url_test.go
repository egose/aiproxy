package dbmerge

import (
	"strings"
	"testing"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/store"
)

func TestBuildProviderAllowInsecureBaseURL(t *testing.T) {
	t.Setenv("AIPROXY_DB_ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef")
	enc, err := store.EncryptSecret([]byte("live-key"))
	if err != nil {
		t.Fatal(err)
	}
	row := store.DBProvider{
		Name: "incluster", Type: "openai-compatible", Enabled: true,
		BaseURL: "http://llm.internal.svc.cluster.local/v1", AllowInsecureBaseURL: true,
		APIKeyEncrypted: enc,
	}
	models := []store.DBProviderModel{{Name: "m", Capabilities: []string{"chat"}}}
	p, err := BuildProvider(row, models, testBases())
	if err != nil {
		t.Fatal(err)
	}
	if !p.AllowInsecureBaseURL {
		t.Fatal("allow_insecure_base_url = false, want true")
	}
	if err := config.ValidateDynamicProvider(p); err != nil {
		t.Fatal(err)
	}
	row.AllowInsecureBaseURL = false
	p, err = BuildProvider(row, models, testBases())
	if err != nil {
		t.Fatal(err)
	}
	if err := config.ValidateDynamicProvider(p); err == nil || !strings.Contains(err.Error(), "non-HTTPS base_url") {
		t.Fatalf("expected loopback rejection without the flag, got %v", err)
	}
}

func TestBuildProviderDerivedInheritsAllowInsecureBaseURL(t *testing.T) {
	t.Setenv("AIPROXY_DB_ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef")
	enc, err := store.EncryptSecret([]byte("derived-key"))
	if err != nil {
		t.Fatal(err)
	}
	bases := testBases()
	base := bases["base"]
	base.Type = config.ProviderTypeOpenAICompatible
	base.BaseURL = "http://llm.internal.svc.cluster.local/v1"
	base.AllowInsecureBaseURL = true
	base.APIKey = "base-key"
	bases["base"] = base
	p, err := BuildProvider(store.DBProvider{Name: "derived", Type: "openai-compatible", Extends: "base", Enabled: true, APIKeyEncrypted: enc}, nil, bases)
	if err != nil {
		t.Fatal(err)
	}
	if p.BaseURL != base.BaseURL || !p.AllowInsecureBaseURL {
		t.Fatalf("derived = %+v, want inherited base_url and flag", p)
	}
	if err := config.ValidateDynamicProvider(p); err != nil {
		t.Fatal(err)
	}
}

func TestBuildProviderDerivedRejectsLocalAllowInsecureBaseURL(t *testing.T) {
	for _, inherited := range []bool{false, true} {
		bases := testBases()
		base := bases["base"]
		base.AllowInsecureBaseURL = inherited
		bases["base"] = base
		_, err := BuildProvider(store.DBProvider{
			Name: "derived", Type: "openai", Extends: "base", Enabled: true,
			AllowInsecureBaseURL: true,
		}, nil, bases)
		if err == nil || !strings.Contains(err.Error(), "derived provider cannot declare allow_insecure_base_url") {
			t.Fatalf("inherited=%t: expected local flag rejection, got %v", inherited, err)
		}
	}
}
