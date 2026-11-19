package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/identity"
	"github.com/egose/aiproxy/internal/store"
	"github.com/egose/aiproxy/internal/upstreamhttp"
	"github.com/egose/aiproxy/internal/upstreampolicy"
	"github.com/google/uuid"
)

func TestModelDiscoveryTenantPolicyUsesRestrictedTransport(t *testing.T) {
	policy, _ := upstreampolicy.New([]string{"https://api.example.com"})
	for _, selector := range []string{"native", "chrome", "safari"} {
		p := config.Provider{Name: "tenant", Type: "openai", BaseURL: "https://api.example.com/v1", APIKey: "own-discovery-fixture",
			DatabaseManaged: true, Workspace: identity.InWorkspace(uuid.New()), OutboundPolicy: policy, TLSClientHello: selector}
		factories, calls := 0, 0
		models, err := listUpstreamModelsWithTransportFactory(context.Background(), p, func(options upstreamhttp.Options) (upstreamhttp.Transport, error) {
			factories++
			if options.OutboundPolicy != policy || options.Profile.CanonicalName() == "" {
				t.Fatal("discovery lost the provider's restriction or TLS profile")
			}
			return discoveryTransport(func(req *http.Request) (*http.Response, error) {
				calls++
				if options.OutboundPolicy.CheckURL(req.URL) != nil || req.URL.Path != "/v1/models" ||
					req.Header.Get("Authorization") != "Bearer own-discovery-fixture" {
					t.Fatal("discovery changed the approved origin or own credential")
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"data":[{"id":"m"}]}`))}, nil
			}), nil
		})
		if err != nil || len(models) != 1 || models[0].ID != "m" || calls != 1 || factories != 1 {
			t.Fatalf("approved discovery %s = %v %v calls=%d factory=%d", selector, models, err, calls, factories)
		}
		for _, mutate := range []func(*config.Provider){
			func(p *config.Provider) { p.BaseURL = "https://127.0.0.1/v1" },
			func(p *config.Provider) { p.BaseURL = "https://unapproved.example.com/v1" },
			func(p *config.Provider) { p.BaseURL = "https://api.example.com:8443/v1" },
			func(p *config.Provider) { p.OutboundPolicy = nil },
			func(p *config.Provider) { p.APIKeyRef = &config.APIKeyRef{Path: "/private/keys.json", Key: "host-key"} },
			func(p *config.Provider) {
				p.CopilotCredentialRef = &config.CopilotCredentialRef{Path: "/private/keys.json", Name: "host-login"}
			},
			func(p *config.Provider) { p.ForwardAuthorization = true },
			func(p *config.Provider) { p.AllowInsecureBaseURL = true },
		} {
			forbidden := p
			mutate(&forbidden)
			beforeFactories, beforeCalls := factories, calls
			models, err := listUpstreamModelsWithTransportFactory(context.Background(), forbidden, func(upstreamhttp.Options) (upstreamhttp.Transport, error) {
				factories++
				return discoveryTransport(func(*http.Request) (*http.Response, error) {
					calls++
					return nil, fmt.Errorf("forbidden discovery reached outbound I/O")
				}), nil
			})
			if err == nil || models != nil || factories != beforeFactories || calls != beforeCalls ||
				strings.Contains(err.Error(), "/private/") {
				t.Fatalf("forbidden discovery prepared or made an outbound call: %v", err)
			}
		}
	}
}

func TestValidateCheckDBTenantDestinationPreflightIsReadOnly(t *testing.T) {
	dsn := os.Getenv("AIPROXY_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("AIPROXY_TEST_DATABASE_URL not set")
	}
	t.Setenv("AIPROXY_DB_ENCRYPTION_KEY", "tenant-preflight-fixture")
	ctx := context.Background()
	admin, err := store.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	schema := "tenant_preflight_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := admin.DB.ExecContext(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer admin.DB.ExecContext(ctx, "DROP SCHEMA "+schema+" CASCADE")
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	query := u.Query()
	query.Set("search_path", schema)
	u.RawQuery = query.Encode()
	st, err := store.Open(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err := st.MigrateUp(ctx); err != nil {
		t.Fatal(err)
	}
	if err := st.EnsureSystemWorkspace(ctx); err != nil {
		t.Fatal(err)
	}
	workspace := store.Workspace{Name: "tenant"}
	if err := st.CreateWorkspace(ctx, &workspace); err != nil {
		t.Fatal(err)
	}
	row := store.DBProvider{Name: "legacy", Type: "openai", WorkspaceID: workspace.ID, BaseURL: "https://api.example.com/v1",
		Enabled: true, APIKeyEncrypted: []byte("corrupt-must-not-decrypt")}
	if err := st.CreateProviderAggregate(ctx, &row, []store.DBProviderModel{{Name: "m"}}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "config.hcl")
	write := func(origins string) {
		t.Helper()
		source := fmt.Sprintf("listener \"http\" \"public\" { address = \"127.0.0.1:8080\" }\nauth \"main\" { mode = \"none\" }\ndatabase { url = %q }\nmulti_tenancy {\n%s\n}\n", u.String(), origins)
		if err := os.WriteFile(path, []byte(source), 0600); err != nil {
			t.Fatal(err)
		}
	}
	preflight := func(want string) {
		t.Helper()
		before, err := st.GetProvider(ctx, row.Name)
		if err != nil {
			t.Fatal(err)
		}
		stdout, _, err := executeRootCommand("", "validate", "--config", path, "--check-db")
		if !strings.Contains(stdout, "config is valid") {
			t.Fatal("preflight no longer distinguishes file validation from DB validation")
		}
		if want == "" {
			if err != nil || !strings.Contains(stdout, "database catalog: valid (1 providers, 0 aliases, 0 keys)") {
				t.Fatalf("repaired DB preflight = %s %v", stdout, err)
			}
		} else if err == nil || !strings.Contains(err.Error(), want) || strings.Contains(err.Error(), "decrypt") ||
			strings.Contains(stdout, "database catalog: valid") || strings.Contains(err.Error(), "/private/") {
			t.Fatalf("invalid DB preflight did not fail before credential resolution: %s %v", stdout, err)
		}
		after, err := st.GetProvider(ctx, row.Name)
		if err != nil || !reflect.DeepEqual(before, after) {
			t.Fatal("validate --check-db mutated provider state")
		}
	}
	write("")
	preflight("operator-approved")
	write(`allowed_upstream_origins = ["https://api.example.com"]`)
	row.APIKeyRefPath, row.APIKeyRefKey = "/private/keys.json", "host-key"
	if err := st.UpdateProvider(ctx, &row); err != nil {
		t.Fatal(err)
	}
	preflight("reserved")
	row.APIKeyRefPath, row.APIKeyRefKey = "", ""
	row.APIKeyEncrypted, err = store.EncryptSecret([]byte("own-preflight-fixture"))
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateProvider(ctx, &row); err != nil {
		t.Fatal(err)
	}
	preflight("")
}
