package dbmerge

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/copilotlogin"
	"github.com/egose/aiproxy/internal/identity"
	"github.com/egose/aiproxy/internal/store"
	"github.com/google/uuid"
)

func TestTenantProviderOwnEncryptedCredentialsAndEffectivePolicy(t *testing.T) {
	t.Setenv("AIPROXY_DB_ENCRYPTION_KEY", "tenant-own-credential-fixture")
	owner := uuid.New()
	encrypted, err := store.EncryptSecret([]byte("own-api-fixture"))
	if err != nil {
		t.Fatal(err)
	}
	row := store.DBProvider{Name: "own", Type: "openai", WorkspaceID: owner, Enabled: true, APIKeyEncrypted: encrypted}
	models := []store.DBProviderModel{{Name: "m"}}
	if _, err := BuildProvider(row, models, nil); err == nil || !strings.Contains(err.Error(), "operator-approved") {
		t.Fatalf("tenant default did not deny: %v", err)
	}
	p, err := BuildProvider(row, models, nil, "https://api.openai.com")
	if err != nil || p.APIKey != "own-api-fixture" || p.OutboundPolicy == nil || config.ValidateDynamicProvider(p) != nil {
		t.Fatalf("own DB API key did not work: %v", err)
	}
	if p.OutboundPolicy.Check("https://api.openai.com/v1/chat/completions") != nil {
		t.Fatal("approved effective provider default was not retained")
	}
	p.Name, p.APIKey = "base", "base-must-not-be-copied"
	child := row
	child.Name, child.Extends, child.TLSClientHello = "child", "base", ""
	derived, err := BuildProvider(child, nil, map[string]config.Provider{"base": p}, "https://api.openai.com")
	if err != nil || derived.APIKey != "own-api-fixture" || derived.Workspace != identity.InWorkspace(owner) ||
		derived.OutboundPolicy == nil || len(derived.Models) != 1 {
		t.Fatalf("same-workspace inheritance lost own credential/policy: %v", err)
	}
	cred, err := copilotlogin.NewCredential("public-client-id", "own-copilot-fixture", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	copilotEncrypted, err := store.EncryptCopilotCredential(cred, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	copilot := store.DBProvider{Name: "copilot", Type: "github-copilot", WorkspaceID: owner, Enabled: true, CopilotCredentialEncrypted: copilotEncrypted}
	p, err = BuildProvider(copilot, models, nil, "https://api.githubcopilot.com")
	if err != nil || p.CopilotToken != cred.AccessToken || p.CopilotCredentialRef != nil || p.APIKey != "" || p.OutboundPolicy == nil {
		t.Fatalf("own encrypted Copilot DB credential did not work: %v", err)
	}
	zen := store.DBProvider{Name: "keyless", Type: "opencode-zen", WorkspaceID: owner, Enabled: true}
	p, err = BuildProvider(zen, []store.DBProviderModel{{Name: "m", Protocol: "chat"}}, nil, "https://opencode.ai")
	if err != nil || p.APIKey != "" || p.OutboundPolicy == nil {
		t.Fatalf("approved keyless Zen provider stopped working: %v", err)
	}
}

func TestTenantProviderPolicyRejectsBeforeCredentialDecryption(t *testing.T) {
	owner := uuid.New()
	for _, enabled := range []bool{true, false} {
		for _, tc := range []struct {
			name   string
			mutate func(*store.DBProvider)
		}{
			{"api_key_ref.key", func(p *store.DBProvider) { p.APIKeyRefKey = "host-key" }},
			{"api_key_ref.path", func(p *store.DBProvider) { p.APIKeyRefPath = "/private/keys.json" }},
			{"api_key_ref.whitespace", func(p *store.DBProvider) { p.APIKeyRefPath, p.APIKeyRefKey = " ", " " }},
			{"sidecar.name", func(p *store.DBProvider) { p.Type, p.CopilotCredentialName = "github-copilot", "host-login" }},
			{"sidecar.path", func(p *store.DBProvider) { p.Type, p.CopilotCredentialPath = "github-copilot", "/private/keys.json" }},
			{"sidecar.whitespace", func(p *store.DBProvider) {
				p.Type, p.CopilotCredentialPath, p.CopilotCredentialName = "github-copilot", " ", " "
			}},
			{"forward_authorization", func(p *store.DBProvider) { p.ForwardAuthorization = true }},
			{"allow_insecure_base_url", func(p *store.DBProvider) { p.AllowInsecureBaseURL = true }},
			{"private", func(p *store.DBProvider) { p.BaseURL = "https://10.0.0.1/v1" }},
			{"metadata", func(p *store.DBProvider) { p.BaseURL = "https://169.254.169.254" }},
			{"cleartext", func(p *store.DBProvider) { p.BaseURL = "http://127.0.0.1/v1" }},
			{"unapproved", func(p *store.DBProvider) { p.BaseURL = "https://unapproved.example.com/v1" }},
			{"subdomain", func(p *store.DBProvider) { p.BaseURL = "https://sub.api.example.com/v1" }},
			{"port", func(p *store.DBProvider) { p.BaseURL = "https://api.example.com:8443/v1" }},
			{"health-private", func(p *store.DBProvider) { p.Healthcheck = json.RawMessage(`{"path":"https://127.0.0.1/ready"}`) }},
			{"health-unapproved", func(p *store.DBProvider) {
				p.Healthcheck = json.RawMessage(`{"path":"https://probe.example.com/ready"}`)
			}},
			{"health-userinfo", func(p *store.DBProvider) {
				p.Healthcheck = json.RawMessage(`{"path":"https://user@api.example.com/ready"}`)
			}},
		} {
			t.Run(tc.name+"/"+boolGraphName(enabled), func(t *testing.T) {
				row := store.DBProvider{Name: "tenant", Type: "openai", WorkspaceID: owner, Enabled: enabled,
					BaseURL: "https://api.example.com/v1", APIKeyEncrypted: []byte("corrupt-ciphertext-must-not-be-decrypted")}
				tc.mutate(&row)
				if row.Type == "github-copilot" {
					row.APIKeyEncrypted = nil
					row.CopilotCredentialEncrypted = []byte("corrupt-copilot-must-not-be-decrypted")
				}
				_, err := BuildProvider(row, []store.DBProviderModel{{Name: "m"}}, nil, "https://api.example.com", "https://api.githubcopilot.com")
				if err == nil || strings.Contains(err.Error(), "decrypt") || strings.Contains(err.Error(), "/private/") ||
					strings.Contains(err.Error(), "corrupt-") {
					t.Fatalf("forbidden tenant configuration touched or disclosed a credential: %v", err)
				}
			})
		}
	}
	control := store.DBProvider{Name: "approved-corrupt", Type: "openai", WorkspaceID: owner, Enabled: true,
		BaseURL: "https://api.example.com/v1", APIKeyEncrypted: []byte("corrupt-ciphertext-must-not-be-decrypted")}
	if _, err := BuildProvider(control, []store.DBProviderModel{{Name: "m"}}, nil, "https://api.example.com"); err == nil || !strings.Contains(err.Error(), "cannot decrypt") {
		t.Fatalf("approved control did not exercise the corrupt decryption fixture: %v", err)
	}
}

func TestTenantInheritedEffectiveDestinationsAndReservedSettings(t *testing.T) {
	owner := uuid.New()
	base := testBases()["base"]
	base.Workspace, base.DatabaseManaged, base.BaseURL = identity.InWorkspace(owner), true, "https://api.example.com/v1"
	for _, tc := range []struct {
		name   string
		mutate func(*config.Provider)
	}{
		{"base-private", func(p *config.Provider) { p.BaseURL = "https://[::1]/v1" }},
		{"base-unapproved", func(p *config.Provider) { p.BaseURL = "https://unapproved.example.com/v1" }},
		{"cleartext-exception", func(p *config.Provider) { p.AllowInsecureBaseURL = true }},
		{"health-private", func(p *config.Provider) {
			p.Healthcheck = &config.ProviderHealthcheck{Path: "http://169.254.169.254/latest"}
		}},
		{"health-unapproved", func(p *config.Provider) {
			p.Healthcheck = &config.ProviderHealthcheck{Path: "https://probe.example.com/ready"}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := base
			tc.mutate(&p)
			row := store.DBProvider{Name: "child", Type: "openai", WorkspaceID: owner, Extends: p.Name,
				APIKeyEncrypted: []byte("corrupt-must-not-decrypt")}
			_, err := BuildProvider(row, nil, map[string]config.Provider{p.Name: p}, "https://api.example.com")
			if err == nil || strings.Contains(err.Error(), "decrypt") {
				t.Fatalf("inherited effective policy bypassed pre-decryption checks: %v", err)
			}
		})
	}
	for _, target := range []string{"/ready", "https://probe.example.com/ready"} {
		p := base
		p.Healthcheck = &config.ProviderHealthcheck{Path: target}
		row := store.DBProvider{Name: "child", Type: "openai", WorkspaceID: owner, Extends: p.Name}
		built, err := BuildProvider(row, nil, map[string]config.Provider{p.Name: p}, "https://api.example.com", "https://probe.example.com")
		if err != nil || built.OutboundPolicy == nil || built.Healthcheck.Path != target || built.APIKey != "" {
			t.Fatalf("approved inherited probe or credential isolation failed: %v", err)
		}
	}
}

func TestMergeCatalogTenantPolicyRejectsOldRowsBeforeDecryption(t *testing.T) {
	st := openReasoningMergeStore(t)
	ctx := context.Background()
	workspace := store.Workspace{Name: "tenant-policy"}
	if err := st.CreateWorkspace(ctx, &workspace); err != nil {
		t.Fatal(err)
	}
	for _, enabled := range []bool{true, false} {
		t.Run(boolGraphName(enabled), func(t *testing.T) {
			row := store.DBProvider{Name: "legacy", Type: "openai", WorkspaceID: workspace.ID,
				Enabled: enabled, BaseURL: "https://api.example.com/v1", APIKeyEncrypted: []byte("corrupt-private-ciphertext")}
			if err := st.CreateProviderAggregate(ctx, &row, []store.DBProviderModel{{Name: "m"}}); err != nil {
				t.Fatal(err)
			}
			defer st.DeleteProvider(ctx, row.ID)
			rt := &config.Runtime{Catalog: config.NewCatalog(nil, nil, nil)}
			before := *rt
			out, err := MergeCatalog(ctx, st, rt)
			if err == nil || !strings.Contains(err.Error(), "operator-approved") || strings.Contains(err.Error(), "decrypt") ||
				!reflect.DeepEqual(out, Merged{}) || !reflect.DeepEqual(*rt, before) {
				t.Fatalf("old row/default deny preflight changed runtime or decrypted: %v", err)
			}
			rt.MultiTenancy.AllowedUpstreamOrigins = []string{"https://api.example.com"}
			row.APIKeyRefPath, row.APIKeyRefKey = "/private/keys.json", "host-key"
			if err := st.UpdateProvider(ctx, &row); err != nil {
				t.Fatal(err)
			}
			saved := row
			if _, err := MergeCatalog(ctx, st, rt); err == nil || !strings.Contains(err.Error(), "reserved") || strings.Contains(err.Error(), "decrypt") {
				t.Fatalf("operator approval granted host credential access: %v", err)
			}
			after, err := st.GetProvider(ctx, row.Name)
			if err != nil || !reflect.DeepEqual(after, saved) {
				t.Fatal("rejected activation modified the old row")
			}
		})
	}
}
