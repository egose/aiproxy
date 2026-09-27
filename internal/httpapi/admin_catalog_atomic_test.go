package httpapi

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/egose/aiproxy/internal/store"
	"github.com/uptrace/bun"
)

func TestCatalogProviderModelFailureHTTP(t *testing.T) {
	st := openValidationStore(t)
	h, access := validationTestHandler(t, st)
	ctx := context.Background()
	activations := 0
	deps := h.current()
	deps.RequestReload = func() error { activations++; return nil }
	h.UpdateDependencies(deps)
	name := uniqName(t, "flow02")
	body := validProviderBody(name)
	body["models"] = []map[string]string{{"name": "first"}, {"name": "flow02-fault"}}
	path := "/_internal/admin/providers"
	exec := func(query string) {
		t.Helper()
		if _, err := st.DB.ExecContext(ctx, query); err != nil {
			t.Fatal(err)
		}
	}
	exec("ALTER TABLE db_provider_models ADD CONSTRAINT flow02_fault CHECK (name <> 'flow02-fault') NOT VALID")
	t.Cleanup(func() {
		_, _ = st.DB.ExecContext(ctx, "ALTER TABLE db_provider_models DROP CONSTRAINT IF EXISTS flow02_fault")
	})
	code, _, raw := doAdmin(t, h, access, "POST", path, body)
	if code != 500 || raw != "could not save catalog edit\n" || activations != 0 {
		t.Fatalf("POST failure = %d %s count=%d", code, raw, activations)
	}
	if _, err := st.GetProvider(ctx, name); err == nil {
		t.Fatal("orphan provider")
	}
	exec("ALTER TABLE db_provider_models DROP CONSTRAINT flow02_fault")
	code, _, raw = doAdmin(t, h, access, "POST", path, body)
	if code != 201 || activations != 1 {
		t.Fatalf("POST retry = %d %s", code, raw)
	}
	p, err := st.GetProvider(ctx, name)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.DeleteProvider(ctx, p.ID) })
	models, err := st.ListProviderModels(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	exec("ALTER TABLE db_provider_models ADD CONSTRAINT flow02_fault CHECK (name <> 'flow02-fault') NOT VALID")
	body["display_name"] = "rejected metadata"
	body["api_key"] = "rejected-secret"
	code, _, raw = doAdmin(t, h, access, "PUT", path+"/"+name, body)
	if code != 500 || raw != "could not save catalog edit\n" || activations != 1 {
		t.Fatalf("PUT failure = %d %s count=%d", code, raw, activations)
	}
	after, err := st.GetProvider(ctx, name)
	if err != nil {
		t.Fatal(err)
	}
	afterModels, err := st.ListProviderModels(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(p, after) || !reflect.DeepEqual(models, afterModels) {
		t.Fatal("PUT partially committed")
	}
	exec("ALTER TABLE db_provider_models DROP CONSTRAINT flow02_fault")
	code, _, raw = doAdmin(t, h, access, "PUT", path+"/"+name, body)
	if code != 200 || activations != 2 || strings.Contains(raw, "rejected-secret") {
		t.Fatalf("PUT retry = %d %s", code, raw)
	}
}

func TestCatalogPostcommitOutcomes(t *testing.T) {
	for _, resource := range []string{"providers", "aliases"} {
		for _, method := range []string{"POST", "PUT"} {
			for _, failure := range []string{"view", "activation"} {
				t.Run(resource+"/"+method+"/"+failure, func(t *testing.T) {
					st := openValidationStore(t)
					h, access := validationTestHandler(t, st)
					ctx := context.Background()
					name := uniqName(t, "flow02")
					body := validProviderBody(name)
					if resource == "aliases" {
						body = map[string]interface{}{"name": name, "targets": []map[string]string{{"provider": "openai", "model": "gpt-4o-mini"}}}
					}
					path := "/_internal/admin/" + resource
					if method == "PUT" {
						if code, _, raw := doAdmin(t, h, access, "POST", path, body); code != 201 {
							t.Fatalf("setup = %d %s", code, raw)
						}
						path += "/" + name
					}
					t.Cleanup(func() {
						if resource == "providers" {
							p, err := st.GetProvider(ctx, name)
							if err == nil {
								_ = st.DeleteProvider(ctx, p.ID)
							}
						} else {
							a, err := st.GetAlias(ctx, name)
							if err == nil {
								_ = st.DeleteAlias(ctx, a.ID)
							}
						}
					})
					count := 0
					table := "db_" + resource
					deps := h.current()
					deps.RequestReload = func() error {
						count++
						if resource == "providers" {
							if _, err := st.GetProvider(ctx, name); err != nil {
								return err
							}
						} else {
							if _, err := st.GetAlias(ctx, name); err != nil {
								return err
							}
						}
						if failure == "activation" {
							return errors.New("private-activation-secret")
						}
						_, err := st.DB.ExecContext(ctx, "ALTER TABLE "+table+" RENAME TO flow02_saved")
						return err
					}
					h.UpdateDependencies(deps)
					code, _, raw := doAdmin(t, h, access, method, path, body)
					if failure == "view" {
						if _, err := st.DB.ExecContext(ctx, "ALTER TABLE flow02_saved RENAME TO "+table); err != nil {
							t.Fatal(err)
						}
					}
					want := "saved but response view unavailable; read current state before retrying\n"
					if failure == "activation" {
						want = "saved but activation failed\n"
					}
					if code != 500 || raw != want || count != 1 {
						t.Fatalf("outcome = %d %q activations=%d", code, raw, count)
					}
					if code, _, raw := doAdmin(t, h, access, "GET", "/_internal/admin/"+resource+"/"+name, nil); code != 200 {
						t.Fatalf("saved view recovery = %d %s", code, raw)
					}
				})
			}
		}
	}
}

type catalogPauseKey struct{}

func TestCatalogPrecommitReadFailures(t *testing.T) {
	for _, resource := range []string{"providers", "aliases"} {
		for _, method := range []string{"POST", "PUT"} {
			t.Run(resource+"/"+method, func(t *testing.T) {
				st := openValidationStore(t)
				h, access := validationTestHandler(t, st)
				ctx := context.Background()
				count := 0
				deps := h.current()
				deps.RequestReload = func() error { count++; return nil }
				h.UpdateDependencies(deps)
				table := "db_" + resource
				if _, err := st.DB.ExecContext(ctx, "ALTER TABLE "+table+" RENAME TO flow02_saved"); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					if _, err := st.DB.ExecContext(ctx, "ALTER TABLE flow02_saved RENAME TO "+table); err != nil {
						t.Error(err)
					}
				})
				path := "/_internal/admin/" + resource
				if method == "PUT" {
					path += "/missing"
				}
				code, _, raw := doAdmin(t, h, access, method, path, validProviderBody("missing"))
				if code != 500 || raw != "could not save catalog edit\n" || count != 0 {
					t.Fatalf("read failure = %d %s activations=%d", code, raw, count)
				}
			})
		}
	}
}

type catalogPauseHook struct {
	read   chan struct{}
	resume chan struct{}
}

func (h catalogPauseHook) BeforeQuery(ctx context.Context, _ *bun.QueryEvent) context.Context {
	return ctx
}
func (h catalogPauseHook) AfterQuery(ctx context.Context, event *bun.QueryEvent) {
	if ctx.Value(catalogPauseKey{}) == true && strings.HasPrefix(event.Query, "SELECT") && strings.Contains(event.Query, `FROM "db_providers"`) {
		select {
		case h.read <- struct{}{}:
			<-h.resume
		default:
		}
	}
}

func TestCatalogCredentialAggregateHTTPConflict(t *testing.T) {
	for _, delayed := range []string{"credential", "aggregate"} {
		t.Run(delayed, func(t *testing.T) {
			st := openValidationStore(t)
			h, access := validationTestHandler(t, st)
			name := uniqName(t, "flow02")
			path := "/_internal/admin/providers/" + name
			if code, _, raw := doAdmin(t, h, access, "POST", "/_internal/admin/providers", validProviderBody(name)); code != 201 {
				t.Fatalf("setup = %d %s", code, raw)
			}
			p, err := st.GetProvider(context.Background(), name)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = st.DeleteProvider(context.Background(), p.ID) })
			pause := catalogPauseHook{read: make(chan struct{}, 1), resume: make(chan struct{})}
			st.DB.AddQueryHook(pause)
			count := 0
			deps := h.current()
			deps.RequestReload = func() error { count++; return nil }
			h.UpdateDependencies(deps)
			slowPath, fastPath := path+"/credential", path
			slowBody, fastBody := `{"api_key":"rotated"}`, `{"display_name":"new","models":[{"name":"new"}]}`
			if delayed == "aggregate" {
				slowPath, fastPath = fastPath, slowPath
				slowBody, fastBody = fastBody, slowBody
			}
			done := make(chan int, 1)
			ctx := context.WithValue(context.Background(), catalogPauseKey{}, true)
			go func() { done <- membershipRequest(ctx, h, access, "PUT", slowPath, slowBody).Code }()
			<-pause.read
			w := membershipRequest(context.Background(), h, access, "PUT", fastPath, fastBody)
			close(pause.resume)
			if code := <-done; code != http.StatusConflict {
				t.Fatalf("delayed update = %d", code)
			}
			if w.Code != 200 || count != 1 {
				t.Fatalf("winner = %d %s count=%d", w.Code, w.Body.String(), count)
			}
			w = membershipRequest(context.Background(), h, access, "PUT", slowPath, slowBody)
			if w.Code != 200 || count != 2 {
				t.Fatalf("retry = %d %s", w.Code, w.Body.String())
			}
			p, err = st.GetProvider(context.Background(), name)
			if err != nil {
				t.Fatal(err)
			}
			secret, err := store.DecryptSecret(p.APIKeyEncrypted)
			if err != nil {
				t.Fatal(err)
			}
			models, err := st.ListProviderModels(context.Background(), p.ID)
			if err != nil {
				t.Fatal(err)
			}
			if p.DisplayName != "new" || string(secret) != "rotated" || len(models) != 1 || models[0].Name != "new" {
				t.Fatal("retry lost unrelated edit")
			}
		})
	}
}
