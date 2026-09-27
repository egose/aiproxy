package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/egose/aiproxy/internal/adminauth"
	"github.com/egose/aiproxy/internal/store"
	"github.com/google/uuid"
)

func catalogRuntimeFixture(t *testing.T) (*App, string, string, func(string, string, string) *httptest.ResponseRecorder) {
	t.Helper()
	dsn := os.Getenv("AIPROXY_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("AIPROXY_TEST_DATABASE_URL not set")
	}
	t.Setenv("AIPROXY_JWT_SECRET", "flow02-runtime-fixture-secret")
	t.Setenv("AIPROXY_DB_ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef")
	ctx := context.Background()
	admin, err := store.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close() })
	schema := "flow02_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := admin.DB.ExecContext(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.DB.ExecContext(ctx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Error(err)
		}
	})
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	fixture, err := store.Open(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.MigrateUp(ctx); err != nil {
		_ = fixture.Close()
		t.Fatal(err)
	}
	if err := fixture.Close(); err != nil {
		t.Fatal(err)
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"model": body.Model, "choices": []interface{}{}})
	}))
	t.Cleanup(upstream.Close)
	cfg := fmt.Sprintf(`
listener "http" "public" { address = "127.0.0.1:0" }
auth "main" {
  mode = "bearer_static"
  client "static" { token = "static-fixture" }
}
provider "openai-compatible" "local" {
  base_url = %q
  api_key = "fixture"
  model "old" {}
  model "new" {}
}
database { url = %q }
multi_tenancy { enabled = true }
`, upstream.URL, u.String())
	path := writeConfigFile(t, cfg)
	a, err := Build(ctx, BuildOptions{ConfigPath: path, LogOutput: io.Discard})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Close() })
	owner := store.User{Email: "admin@example.com", IsAdmin: true}
	if err := a.adminStore.CreateUser(ctx, &owner); err != nil {
		t.Fatal(err)
	}
	access, _, err := adminauth.IssueAccess(owner.ID.String(), owner.Email, true)
	if err != nil {
		t.Fatal(err)
	}
	request := func(method, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		token := access
		if strings.HasPrefix(path, "/v1/") {
			token = "static-fixture"
		}
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		a.Server.Handler.ServeHTTP(w, r)
		return w
	}
	return a, path, cfg, request
}

func TestCatalogRuntimeRollbackAndRecovery(t *testing.T) {
	a, _, _, request := catalogRuntimeFixture(t)
	ctx := context.Background()
	st := a.adminStore
	local, _ := a.Config.Catalog.Provider("local")
	body := func(model string) string {
		return fmt.Sprintf(`{"name":"dynamic","type":"openai-compatible","base_url":%q,"api_key":"fixture-secret","display_name":%q,"models":[{"name":"m","upstream_name":%q},{"name":"fault"}]}`, local.BaseURL, model, model)
	}
	exec := func(sql string) {
		t.Helper()
		if _, err := st.DB.ExecContext(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	check := func(want string) {
		t.Helper()
		w := request("POST", "/v1/chat/completions", `{"model":"dynamic/m","messages":[]}`)
		if want == "absent" {
			if w.Code == 200 {
				t.Fatal("rejected provider became routable")
			}
			return
		}
		if w.Code != 200 || !strings.Contains(w.Body.String(), `"model":"`+want+`"`) {
			t.Fatalf("route = %d %s want %s", w.Code, w.Body.String(), want)
		}
	}
	exec("ALTER TABLE db_provider_models ADD CONSTRAINT flow02_fault CHECK (name <> 'fault') NOT VALID")
	w := request("POST", "/_internal/admin/providers", body("old"))
	if w.Code != 500 || w.Body.String() != "could not save catalog edit\n" {
		t.Fatalf("POST failure = %d %s", w.Code, w.Body.String())
	}
	if _, err := st.GetProvider(ctx, "dynamic"); err == nil {
		t.Fatal("orphan")
	}
	if err := a.Reload(); err != nil {
		t.Fatal(err)
	}
	check("absent")
	exec("ALTER TABLE db_provider_models DROP CONSTRAINT flow02_fault")
	w = request("POST", "/_internal/admin/providers", body("old"))
	if w.Code != 201 {
		t.Fatalf("POST retry = %d %s", w.Code, w.Body.String())
	}
	check("old")
	p, err := st.GetProvider(ctx, "dynamic")
	if err != nil {
		t.Fatal(err)
	}
	models, err := st.ListProviderModels(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	exec("ALTER TABLE db_provider_models ADD CONSTRAINT flow02_fault CHECK (name <> 'fault') NOT VALID")
	w = request("PUT", "/_internal/admin/providers/dynamic", body("new"))
	if w.Code != 500 {
		t.Fatalf("PUT failure = %d %s", w.Code, w.Body.String())
	}
	after, err := st.GetProvider(ctx, "dynamic")
	if err != nil {
		t.Fatal(err)
	}
	afterModels, err := st.ListProviderModels(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(p, after) || !reflect.DeepEqual(models, afterModels) {
		t.Fatal("rejected aggregate saved")
	}
	check("old")
	if err := a.Reload(); err != nil {
		t.Fatal(err)
	}
	check("old")
	exec("ALTER TABLE db_provider_models DROP CONSTRAINT flow02_fault")
	w = request("PUT", "/_internal/admin/providers/dynamic", body("new"))
	if w.Code != 200 {
		t.Fatalf("PUT retry = %d %s", w.Code, w.Body.String())
	}
	check("new")
	if err := a.Reload(); err != nil {
		t.Fatal(err)
	}
	check("new")
}

func TestCatalogRuntimeActivationRecovery(t *testing.T) {
	for _, resource := range []string{"providers", "aliases"} {
		for _, method := range []string{"POST", "PUT"} {
			t.Run(resource+"/"+method, func(t *testing.T) {
				a, path, cfg, request := catalogRuntimeFixture(t)
				local, _ := a.Config.Catalog.Provider("local")
				body := func(model string) string {
					if resource == "providers" {
						return fmt.Sprintf(`{"name":"dynamic","type":"openai-compatible","base_url":%q,"api_key":"fixture-secret","display_name":%q,"models":[{"name":"m","upstream_name":%q}]}`, local.BaseURL, model, model)
					}
					return fmt.Sprintf(`{"name":"dynamic","algorithm":"least_connections","targets":[{"provider":"local","model":%q}]}`, model)
				}
				endpoint := "/_internal/admin/" + resource
				public := "dynamic/m"
				if resource == "aliases" {
					public = "alias/dynamic"
				}
				check := func(want string) {
					t.Helper()
					w := request("POST", "/v1/chat/completions", `{"model":"`+public+`","messages":[]}`)
					if want == "absent" {
						if w.Code == 200 {
							t.Fatal("unactivated create routed")
						}
						return
					}
					if w.Code != 200 || !strings.Contains(w.Body.String(), `"model":"`+want+`"`) {
						t.Fatalf("route = %d %s want %s", w.Code, w.Body.String(), want)
					}
				}
				old := "absent"
				if method == "PUT" {
					w := request("POST", endpoint, body("old"))
					if w.Code != 201 {
						t.Fatalf("setup = %d %s", w.Code, w.Body.String())
					}
					endpoint += "/dynamic"
					old = "old"
				}
				check(old)
				rewriteConfigFile(t, path, "invalid hcl { private-config-secret")
				w := request(method, endpoint, body("new"))
				if w.Code != 500 || w.Body.String() != "saved but activation failed\n" {
					t.Fatalf("activation failure = %d %s", w.Code, w.Body.String())
				}
				check(old)
				ctx := context.Background()
				if resource == "providers" {
					p, err := a.adminStore.GetProvider(ctx, "dynamic")
					if err != nil {
						t.Fatal(err)
					}
					models, err := a.adminStore.ListProviderModels(ctx, p.ID)
					if err != nil {
						t.Fatal(err)
					}
					if p.DisplayName != "new" || len(models) != 1 || models[0].UpstreamName != "new" {
						t.Fatal("incomplete saved provider")
					}
				} else {
					alias, err := a.adminStore.GetAlias(ctx, "dynamic")
					if err != nil {
						t.Fatal(err)
					}
					targets, err := a.adminStore.ListAliasTargets(ctx, alias.ID)
					if err != nil {
						t.Fatal(err)
					}
					if alias.Algorithm != "least_connections" || len(targets) != 1 || targets[0].Model != "new" {
						t.Fatal("incomplete saved alias")
					}
				}
				rewriteConfigFile(t, path, cfg)
				if err := a.Reload(); err != nil {
					t.Fatal(err)
				}
				check("new")
				if method == "POST" {
					endpoint += "/dynamic"
				}
				w = request("PUT", endpoint, body("old"))
				if w.Code != 200 {
					t.Fatalf("successful change = %d %s", w.Code, w.Body.String())
				}
				check("old")
			})
		}
	}
}
