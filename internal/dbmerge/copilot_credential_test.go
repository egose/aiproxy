package dbmerge

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/copilotlogin"
	"github.com/egose/aiproxy/internal/store"
	"github.com/google/uuid"
)

func TestBuildCopilotCredentialSources(t *testing.T) {
	t.Setenv("AIPROXY_DB_ENCRYPTION_KEY", "copilot-merge-key")
	cred, err := copilotlogin.NewCredential("client-id", "child-private-token", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	enc, err := store.EncryptCopilotCredential(cred, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	models := []store.DBProviderModel{{Name: "m", Capabilities: []string{"chat"}}}
	base := config.Provider{Type: config.ProviderTypeGitHubCopilot, Name: "base", Enabled: true, CopilotToken: "base-private-token", Models: []config.Model{{Name: "m", UpstreamName: "m"}}, ModelByName: map[string]config.Model{"m": {Name: "m", UpstreamName: "m"}}}
	for _, derived := range []bool{false, true} {
		for _, enabled := range []bool{false, true} {
			row := store.DBProvider{Name: "child", Type: "github-copilot", Enabled: enabled, CopilotCredentialEncrypted: enc}
			if derived {
				row.Extends = "base"
			}
			p, err := BuildProvider(row, models, map[string]config.Provider{"base": base})
			if err != nil || config.ValidateDynamicProvider(p) != nil {
				t.Fatalf("build/validate: %v", err)
			}
			if p.CopilotToken != cred.AccessToken || p.APIKey != "" || p.APIKeyRef != nil || p.CopilotCredentialRef != nil || p.Enabled != enabled {
				t.Fatal("wrong runtime materialization")
			}
			row.CopilotCredentialEncrypted = nil
			p, err = BuildProvider(row, models, map[string]config.Provider{"base": base})
			if err != nil || p.CopilotToken != "" {
				t.Fatal("child inherited base token")
			}
			if (config.ValidateDynamicProvider(p) == nil) == enabled {
				t.Fatal("missing credential enabled/disabled semantics changed")
			}
		}
	}
	for name, mutate := range map[string]func(*store.DBProvider){
		"wrong type":   func(p *store.DBProvider) { p.Type = "openai" },
		"file name":    func(p *store.DBProvider) { p.CopilotCredentialName = "private-name" },
		"file path":    func(p *store.DBProvider) { p.CopilotCredentialPath = "private-path" },
		"api blob":     func(p *store.DBProvider) { p.APIKeyEncrypted = []byte("private-blob") },
		"api key ref":  func(p *store.DBProvider) { p.APIKeyRefKey = "private-key" },
		"api path ref": func(p *store.DBProvider) { p.APIKeyRefPath = "private-path" },
		"bad blob":     func(p *store.DBProvider) { p.CopilotCredentialEncrypted = []byte("private-blob") },
		"empty blob":   func(p *store.DBProvider) { p.CopilotCredentialEncrypted = []byte{} },
		"orphan file": func(p *store.DBProvider) {
			p.CopilotCredentialEncrypted = nil
			p.CopilotCredentialPath = "private-path"
		},
	} {
		t.Run(name, func(t *testing.T) {
			for _, enabled := range []bool{false, true} {
				row := store.DBProvider{Name: "child", Type: "github-copilot", Enabled: enabled, CopilotCredentialEncrypted: enc}
				mutate(&row)
				_, err := BuildProvider(row, models, nil)
				if err == nil || strings.Contains(err.Error(), "private") {
					t.Fatal("invalid source accepted or error leaked credential data")
				}
			}
		})
	}
}

func TestMergeCopilotDatabaseAndSidecar(t *testing.T) {
	st, err := store.Open(context.Background(), testDBURL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	ctx := context.Background()
	if _, err := st.MigrateUp(ctx); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AIPROXY_DB_ENCRYPTION_KEY", "copilot-merge-key")
	secretsPath := filepath.Join(t.TempDir(), "keys.json")
	cred, err := copilotlogin.NewCredential("client-id", "sidecar-private-token", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := copilotlogin.Save(secretsPath, "file", cred); err != nil {
		t.Fatal(err)
	}
	baseName := "base-" + uuid.NewString()
	models := []store.DBProviderModel{{Name: "m", Capabilities: []string{"chat"}}}
	baseRow := store.DBProvider{Name: baseName, Type: "github-copilot", Enabled: true, CopilotCredentialPath: secretsPath, CopilotCredentialName: "file"}
	if err := st.CreateProviderAggregate(ctx, &baseRow, models); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.DeleteProvider(ctx, baseRow.ID) })
	want := map[string]string{baseName: cred.AccessToken}
	var dbRow store.DBProvider
	for _, source := range []string{"database", "sidecar"} {
		for _, derived := range []bool{false, true} {
			for _, enabled := range []bool{false, true} {
				row := store.DBProvider{Name: "child-" + uuid.NewString(), Type: "github-copilot", Enabled: enabled, DisplayName: "Local display"}
				if derived {
					row.Extends = baseName
				}
				localModels := models
				if derived {
					localModels = nil
				}
				if source == "sidecar" {
					row.CopilotCredentialPath, row.CopilotCredentialName = secretsPath, "file"
					if enabled {
						want[row.Name] = "sidecar-private-token"
					} else {
						want[row.Name] = ""
					}
				} else {
					cred.AccessToken = row.Name + "-private-token"
					row.CopilotCredentialEncrypted, err = store.EncryptCopilotCredential(cred, time.Now())
					if err != nil {
						t.Fatal(err)
					}
					want[row.Name] = cred.AccessToken
				}
				if err := st.CreateProviderAggregate(ctx, &row, localModels); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = st.DeleteProvider(ctx, row.ID) })
				if source == "database" {
					dbRow = row
				}
			}
		}
	}
	rt := &config.Runtime{Catalog: config.NewCatalog(nil, nil, nil), UserAgent: "root/1", UpstreamHeaderTimeout: 37 * time.Second, ForwardUserAgent: true}
	merged, err := MergeCatalog(ctx, st, rt)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range append(merged.Providers, merged.DisabledProviders...) {
		token, ok := want[p.Name]
		if !ok {
			continue
		}
		if p.CopilotToken != token || p.APIKey != "" || p.UserAgent != "root/1" || p.UpstreamHeaderTimeout != 37*time.Second || !p.ForwardUserAgent || len(p.Models) != 1 {
			t.Fatal("merge lost credential isolation, inherited models, or root defaults")
		}
		if p.Name != baseName && p.DisplayName != "Local display" {
			t.Fatal("local display name lost")
		}
		if err := config.ValidateDynamicProvider(p); err != nil {
			t.Fatal(err)
		}
		delete(want, p.Name)
	}
	if len(want) != 0 {
		t.Fatal("missing merged providers")
	}
	validBlob := append([]byte(nil), dbRow.CopilotCredentialEncrypted...)
	badBlobs := map[string][]byte{"corrupt": []byte("corrupt-private-blob"), "wrong-key": validBlob}
	for _, kind := range []string{"domain", "expiry", "version"} {
		bad := cred
		version := 1
		switch kind {
		case "domain":
			bad.Domain = "private.example"
		case "expiry":
			bad.ExpiresAt = time.Now().Add(-time.Minute).Unix()
		case "version":
			version = 2
		}
		plain, err := json.Marshal(map[string]any{"version": version, "kind": "github-copilot", "credential": bad})
		if err != nil {
			t.Fatal(err)
		}
		badBlobs[kind], err = store.EncryptSecret(plain)
		if err != nil {
			t.Fatal(err)
		}
	}
	for name, blob := range badBlobs {
		t.Run(name, func(t *testing.T) {
			if name == "wrong-key" {
				t.Setenv("AIPROXY_DB_ENCRYPTION_KEY", "wrong-private-key")
			}
			for _, enabled := range []bool{true, false} {
				dbRow.CopilotCredentialEncrypted = blob
				dbRow.Enabled = enabled
				if err := st.UpdateProviderAggregate(ctx, &dbRow, nil); err != nil {
					t.Fatal(err)
				}
				if _, err := MergeCatalog(ctx, st, rt); err == nil || strings.Contains(err.Error(), "private") {
					t.Fatal("invalid stored credential must fail safely, including disabled providers")
				}
			}
		})
	}
}
