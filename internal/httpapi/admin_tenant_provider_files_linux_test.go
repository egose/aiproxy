package httpapi

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/copilotlogin"
	"github.com/egose/aiproxy/internal/dbmerge"
	"github.com/egose/aiproxy/internal/store"
	"golang.org/x/sys/unix"
)

func watchTenantPolicySecretFiles(t *testing.T, paths ...string) func() int {
	t.Helper()
	fd, err := unix.InotifyInit1(unix.IN_CLOEXEC | unix.IN_NONBLOCK)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = unix.Close(fd) })
	for _, path := range paths {
		if _, err := unix.InotifyAddWatch(fd, path, unix.IN_OPEN|unix.IN_ACCESS); err != nil {
			t.Fatal(err)
		}
	}
	return func() int {
		t.Helper()
		buf := make([]byte, 8192)
		n, err := unix.Read(fd, buf)
		if errors.Is(err, unix.EAGAIN) {
			return 0
		}
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
}

func TestTenantProviderPolicyNeverOpensHostSecretFiles(t *testing.T) {
	fixture, workspace, tenant := tenantProviderPolicyFixture(t)
	ctx, st := context.Background(), fixture.store
	dir := t.TempDir()
	keysPath := filepath.Join(dir, "keys.json")
	if err := os.WriteFile(keysPath, []byte(`{"host-key":"host-file-private-fixture"}`), 0600); err != nil {
		t.Fatal(err)
	}
	cred, err := copilotlogin.NewCredential("public-client-id", "host-sidecar-private-fixture", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := copilotlogin.Save(keysPath, "host-login", cred); err != nil {
		t.Fatal(err)
	}
	sidecarPath, err := copilotlogin.SidecarPath(keysPath, "host-login")
	if err != nil {
		t.Fatal(err)
	}
	fileEvents := watchTenantPolicySecretFiles(t, keysPath, sidecarPath)
	for _, actor := range []string{tenant, fixture.access} {
		for _, body := range []map[string]any{
			{"name": "rejected", "type": "openai", "base_url": "https://api.example.com/v1", "api_key_ref": map[string]string{"path": keysPath, "key": "host-key"}, "models": []map[string]string{{"name": "m"}}},
			{"name": "rejected", "type": "github-copilot", "credential_ref": map[string]string{"path": keysPath, "name": "host-login"}, "models": []map[string]string{{"name": "m"}}},
		} {
			count := fixture.activations
			w := tenantProviderPolicyRequest(t, fixture, actor, "POST", "providers", workspace.ID, body)
			if w.Code != 400 || w.Header().Get(catalogSavedHeader) != "" || fixture.activations != count ||
				strings.Contains(w.Body.String(), keysPath) || fileEvents() != 0 {
				t.Fatalf("forbidden API create opened a host file or persisted: %d %s", w.Code, w.Body.String())
			}
		}
	}
	system := store.DBProvider{Name: "a-system-file", Type: "openai", WorkspaceID: store.SystemWorkspaceID,
		APIKeyRefPath: keysPath, APIKeyRefKey: "host-key", Enabled: true}
	if err := st.CreateProviderAggregate(ctx, &system, []store.DBProviderModel{{Name: "m"}}); err != nil {
		t.Fatal(err)
	}
	legacy := store.DBProvider{Name: "z-legacy-tenant", Type: "openai", WorkspaceID: workspace.ID,
		BaseURL: "https://api.example.com/v1", APIKeyRefPath: keysPath, APIKeyRefKey: "host-key",
		APIKeyEncrypted: []byte("corrupt-must-not-decrypt"), Enabled: true}
	if err := st.CreateProviderAggregate(ctx, &legacy, []store.DBProviderModel{{Name: "m"}}); err != nil {
		t.Fatal(err)
	}
	for _, enabled := range []bool{true, false} {
		for _, copilot := range []bool{false, true} {
			row := legacy
			row.Enabled = enabled
			if copilot {
				row.Type, row.BaseURL, row.APIKeyRefPath, row.APIKeyRefKey, row.APIKeyEncrypted = "github-copilot", "", "", "", nil
				row.CopilotCredentialPath, row.CopilotCredentialName = keysPath, "host-login"
				row.CopilotCredentialEncrypted = nil
			}
			if err := st.UpdateProvider(ctx, &row); err != nil {
				t.Fatal(err)
			}
			legacy.UpdatedAt = row.UpdatedAt
			before := row
			out, err := dbmerge.MergeCatalog(ctx, st, fixture.runtime)
			if err == nil || !strings.Contains(err.Error(), "reserved") || strings.Contains(err.Error(), "decrypt") ||
				!reflect.DeepEqual(out, dbmerge.Merged{}) || fileEvents() != 0 {
				t.Fatalf("old-row activation opened any host file (including the earlier System row) or decrypted: %v", err)
			}
			for _, actor := range []string{tenant, fixture.access} {
				for _, path := range []string{"providers/z-legacy-tenant", "providers/z-legacy-tenant/credential"} {
					w := tenantProviderPolicyRequest(t, fixture, actor, "PUT", path, workspace.ID, map[string]string{"api_key": "replacement-never-saved"})
					if w.Code != 400 || w.Header().Get(catalogSavedHeader) != "" || fileEvents() != 0 {
						t.Fatalf("old-row update opened a host secret file: %d %s", w.Code, w.Body.String())
					}
				}
			}
			after, err := st.GetProvider(ctx, row.Name)
			if err != nil || !reflect.DeepEqual(after, before) {
				t.Fatal("old-row rejection modified stored configuration")
			}
		}
	}
	if err := st.DeleteProvider(ctx, legacy.ID); err != nil {
		t.Fatal(err)
	}
	merged, err := dbmerge.MergeCatalog(ctx, st, fixture.runtime)
	if err != nil || fileEvents() == 0 {
		t.Fatalf("trusted System key reference did not retain actual file resolution: %v", err)
	}
	found := false
	for _, p := range merged.Providers {
		if p.Name == system.Name {
			found = p.APIKey == "host-file-private-fixture" && p.OutboundPolicy == nil
		}
	}
	if !found {
		t.Fatal("System reference did not resolve to the trusted credential")
	}
	hcl := fmt.Sprintf(`
listener "http" "public" { address = "127.0.0.1:8080" }
auth "main" { mode = "none" }
provider "openai" "hcl-key" {
  base_url = "http://10.0.0.1/v1"
  allow_insecure_base_url = true
  api_key_ref {
    path = %q
    key = "host-key"
  }
  model "m" {}
}
provider "github-copilot" "hcl-copilot" {
  credential_ref {
    path = %q
    name = "host-login"
  }
  model "m" {}
}
`, keysPath, keysPath)
	rt, err := config.Load([]byte(hcl), "trusted-files.hcl")
	if err != nil || fileEvents() == 0 {
		t.Fatalf("file-only HCL secret resolution changed: %v", err)
	}
	p, _ := rt.Catalog.Provider("hcl-copilot")
	if p.CopilotToken != cred.AccessToken || p.OutboundPolicy != nil {
		t.Fatal("trusted file-only Copilot reference stopped working")
	}
}
