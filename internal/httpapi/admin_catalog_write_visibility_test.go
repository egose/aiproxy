package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/egose/aiproxy/internal/adminauth"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/store"
	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type catalogDenialAuditKey struct{}

type catalogDenialAudit struct {
	references int
	unexpected []string
}

type catalogDenialQueryHook struct{}

func (catalogDenialQueryHook) BeforeQuery(ctx context.Context, event *bun.QueryEvent) context.Context {
	audit, _ := ctx.Value(catalogDenialAuditKey{}).(*catalogDenialAudit)
	if audit == nil {
		return ctx
	}
	columns, from, _ := strings.Cut(event.Query, ` FROM `)
	switch {
	case columns == `SELECT "p"."id", "p"."name", "p"."type", "p"."extends", "p"."enabled", "p"."workspace_id"` && strings.HasPrefix(from, `"db_providers" AS "p" WHERE (name = `),
		columns == `SELECT "a"."id", "a"."name", "a"."workspace_id"` && strings.HasPrefix(from, `"db_aliases" AS "a" WHERE (name = `):
		audit.references++
	case event.Operation() == "SELECT" && (strings.HasPrefix(from, `"users" AS `) || strings.HasPrefix(from, `"workspace_members" AS `) || strings.HasPrefix(from, `"workspaces" AS `)):
	default:
		audit.unexpected = append(audit.unexpected, event.Query)
	}
	return ctx
}

func (catalogDenialQueryHook) AfterQuery(context.Context, *bun.QueryEvent) {}

func TestAdminCatalogWriteVisibility(t *testing.T) {
	rt, err := config.Load([]byte(`
listener "http" "public" { address = "127.0.0.1:8080" }
auth "main" { mode = "none" }
provider "openai" "system-hcl" {
  api_key = "hcl-fixture"
  model "m" {}
}
provider "openai" "system-disabled" {
  enabled = false
  model "m" {}
}
alias "system-hcl" {
  algorithm = "round_robin"
  target {
    provider = "system-hcl"
    model    = "m"
  }
}
`), "write-visibility.hcl")
	if err != nil {
		t.Fatal(err)
	}
	fixture := newAdminReasoningFixture(t, rt)
	st, h, ctx := fixture.store, fixture.handler, context.Background()
	a, b := store.Workspace{Name: "a"}, store.Workspace{Name: "b"}
	for _, workspace := range []*store.Workspace{&a, &b} {
		if err := st.CreateWorkspace(ctx, workspace); err != nil {
			t.Fatal(err)
		}
	}
	actor := func(name string, workspace *uuid.UUID, role string) string {
		t.Helper()
		u := store.User{Email: name + "@example.test"}
		if err := st.CreateUser(ctx, &u); err != nil {
			t.Fatal(err)
		}
		if workspace != nil {
			if err := st.AddMembership(ctx, &store.WorkspaceMember{UserID: u.ID, WorkspaceID: *workspace, Role: role}); err != nil {
				t.Fatal(err)
			}
		}
		access, _, err := adminauth.IssueAccess(u.ID.String(), u.Email, false)
		if err != nil {
			t.Fatal(err)
		}
		return access
	}
	global := fixture.access
	system := store.SystemWorkspaceID
	aAdmin, aMember := actor("a-admin", &a.ID, roleAdmin), actor("a-member", &a.ID, roleMember)
	systemMember, noMembership := actor("system-member", &system, roleMember), actor("no-membership", nil, "")
	request := func(access, method, path, body string, headers ...string) *http.Request {
		r := httptest.NewRequest(method, adminPathPrefix+path, strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+access)
		r.Header.Set("Content-Type", "application/json")
		for _, value := range headers {
			r.Header.Add("X-Workspace-ID", value)
		}
		return r
	}
	send := func(r *http.Request) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	for _, seed := range []struct {
		name string
		id   uuid.UUID
	}{{"a-managed", a.ID}, {"b-managed", b.ID}, {"system-managed", system}} {
		body := fmt.Sprintf(`{"name":%q,"type":"openai","api_key":"setup-fixture","models":[{"name":"m"}]}`, seed.name)
		w := send(request(global, "POST", "providers", body, seed.id.String()))
		if w.Code != http.StatusCreated {
			t.Fatalf("seed provider %s: %d %s", seed.name, w.Code, w.Body.String())
		}
		body = fmt.Sprintf(`{"name":%q,"targets":[{"provider":%q,"model":"m"}]}`, seed.name, seed.name)
		w = send(request(global, "POST", "aliases", body, seed.id.String()))
		if w.Code != http.StatusCreated {
			t.Fatalf("seed alias %s: %d %s", seed.name, w.Code, w.Body.String())
		}
	}
	readRevisions := func(t *testing.T) store.ServingRevisions {
		t.Helper()
		revisions, err := st.ServingRevisions(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return revisions
	}
	beforeGraph := readAdminSavedGraph(t, st)
	beforeRevisions, beforeActivations := readRevisions(t), fixture.activations
	applied := beforeRevisions
	deps := h.current()
	deps.RequestReload = func() error {
		if err := fixture.reload(); err != nil {
			return err
		}
		var err error
		applied, err = st.ServingRevisions(ctx)
		return err
	}
	deps.AppliedServingRevisions = func() store.ServingRevisions { return applied }
	h.UpdateDependencies(deps)
	memberships := &workspaceMembershipReadHook{}
	st.DB.AddQueryHook(catalogDenialQueryHook{})
	st.DB.AddQueryHook(memberships)
	reject := func(t *testing.T, r *http.Request, status int, body string, maxReferences int) {
		t.Helper()
		audit := &catalogDenialAudit{}
		activations := fixture.activations
		w := send(r.WithContext(context.WithValue(r.Context(), catalogDenialAuditKey{}, audit)))
		if w.Code != status || w.Body.String() != body {
			t.Fatalf("%s %s = %d %q, want %d %q", r.Method, r.URL, w.Code, w.Body.String(), status, body)
		}
		for _, header := range []string{catalogSavedHeader, "X-Aiproxy-Local-Active", "X-Aiproxy-Cluster-Acknowledged", "X-Aiproxy-Applied-Catalog-Revision", "X-Aiproxy-Applied-Key-Revision"} {
			if len(w.Header().Values(header)) != 0 {
				t.Errorf("rejected write returned %s", header)
			}
		}
		if len(audit.unexpected) != 0 || audit.references > maxReferences {
			t.Errorf("denial read beyond authorized metadata: references=%d (max %d), unexpected=%q", audit.references, maxReferences, audit.unexpected)
		}
		if fixture.activations != activations {
			t.Error("rejected write requested reload")
		}
	}
	type route struct {
		name, method, resource, collection, suffix string
		fields                                     map[string]string
		bodySelector                               bool
	}
	routes := []route{
		{"provider-update", "PUT", "provider", "providers", "", map[string]string{"display_name": "Own edit"}, true},
		{"credential-update", "PUT", "provider", "providers", "/credential", map[string]string{"api_key": "rotated-fixture"}, false},
		{"alias-update", "PUT", "alias", "aliases", "", map[string]string{"algorithm": "least_connections"}, true},
		{"alias-delete", "DELETE", "alias", "aliases", "", nil, false},
		{"provider-delete", "DELETE", "provider", "providers", "", nil, false},
	}
	type selection struct {
		query   string
		headers []string
		body    *string
	}
	writeRequest := func(t *testing.T, op route, access, name string, sel selection) *http.Request {
		t.Helper()
		body := ""
		if op.fields != nil {
			fields := maps.Clone(op.fields)
			if sel.body != nil {
				fields["workspace_id"] = *sel.body
			}
			raw, err := json.Marshal(fields)
			if err != nil {
				t.Fatal(err)
			}
			body = string(raw)
		}
		return request(access, op.method, op.collection+"/"+name+op.suffix+sel.query, body, sel.headers...)
	}
	aID, bID := a.ID.String(), b.ID.String()
	unknownID, invalidID := uuid.NewString(), "invalid"
	for _, op := range routes {
		t.Run(op.name, func(t *testing.T) {
			notFound := op.resource + " not found\n"
			staticNames := []string{"system-hcl"}
			if op.resource == "provider" {
				staticNames = append(staticNames, "system-disabled")
			}
			for _, name := range append([]string{"b-managed", "system-managed", "missing"}, staticNames...) {
				reject(t, writeRequest(t, op, aAdmin, name, selection{}), http.StatusNotFound, notFound, 1)
				reject(t, writeRequest(t, op, noMembership, name, selection{}), http.StatusNotFound, notFound, 0)
			}
			reject(t, writeRequest(t, op, aAdmin, "b-managed", selection{headers: []string{bID}}), http.StatusNotFound, notFound, 0)

			mismatch := []selection{
				{query: "?workspace_id=" + bID}, {headers: []string{bID}}, {headers: []string{unknownID}},
				{query: "?workspace_id=" + aID, headers: []string{bID}},
				{query: "?workspace_id=" + bID, headers: []string{aID}},
				{query: "?workspace_id=" + aID + "&workspace_id=" + bID}, {headers: []string{aID, bID}},
			}
			malformed := []selection{
				{query: "?workspace_id=invalid", headers: []string{aID}},
				{query: "?workspace_id=" + aID, headers: []string{"invalid"}},
				{query: "?workspace_id=" + aID + "&workspace_id="}, {headers: []string{aID, ""}},
			}
			if op.bodySelector {
				mismatch = append(mismatch, selection{body: &bID}, selection{query: "?workspace_id=" + aID, headers: []string{aID}, body: &bID})
				malformed = append(malformed, selection{headers: []string{aID}, body: &invalidID})
				for _, name := range []string{"a-managed", "missing"} {
					path := op.collection + "/" + name
					for _, value := range []string{`null`, `42`, `""`} {
						reject(t, request(global, op.method, path, `{"workspace_id":`+value+`}`, aID), http.StatusBadRequest, "invalid workspace id\n", 0)
					}
					duplicate := fmt.Sprintf(`{"workspace_id":%q,"workspace_id":%q}`, bID, aID)
					reject(t, request(global, op.method, path, duplicate), http.StatusBadRequest, "duplicate JSON field \"workspace_id\"\n", 0)
				}
			}
			for _, sel := range mismatch {
				for _, name := range []string{"a-managed", "missing"} {
					reject(t, writeRequest(t, op, global, name, sel), http.StatusNotFound, notFound, 1)
				}
			}
			for _, sel := range malformed {
				for _, name := range []string{"a-managed", "missing"} {
					reject(t, writeRequest(t, op, global, name, sel), http.StatusBadRequest, "invalid workspace id\n", 0)
				}
			}
			if op.method == "PUT" {
				reject(t, request(aAdmin, op.method, op.collection+"/b-managed"+op.suffix, "{"), http.StatusBadRequest, "invalid body\n", 0)
			}
			if op.resource == "provider" && op.method == "PUT" {
				for _, name := range []string{"b-managed", "missing"} {
					body := fmt.Sprintf("provider %q: deferred_transport and deferred_receiver are static-config-only; edit the static config file\n", name)
					r := request(aAdmin, op.method, "providers/"+name+op.suffix, `{"models":[{"name":"m","deferred_transport":{}}]}`)
					reject(t, r, http.StatusBadRequest, body, 0)
				}
			}

			roleStatus, systemStatus, systemBody := http.StatusBadRequest, http.StatusBadRequest, "workspace admin required\n"
			if op.method == "DELETE" {
				roleStatus, systemStatus, systemBody = http.StatusForbidden, http.StatusNotFound, "workspace not found\n"
			}
			reject(t, writeRequest(t, op, aMember, "a-managed", selection{}), roleStatus, "workspace admin required\n", 1)
			reject(t, writeRequest(t, op, systemMember, "system-managed", selection{}), systemStatus, systemBody, 1)
			for _, name := range staticNames {
				for _, access := range []string{systemMember, global} {
					reject(t, writeRequest(t, op, access, name, selection{headers: []string{system.String()}}), roleStatus, "config-managed: edit the HCL file\n", 1)
				}
				reject(t, writeRequest(t, op, global, name, selection{headers: []string{aID}}), http.StatusNotFound, notFound, 1)
			}

			memberships.fail = true
			defer func() { memberships.fail = false }()
			for _, name := range []string{"a-managed", "b-managed", "system-hcl", "missing"} {
				before := memberships.reads
				reject(t, writeRequest(t, op, aAdmin, name, selection{headers: []string{aID}}), http.StatusInternalServerError, "could not resolve workspace visibility\n", 0)
				if memberships.reads != before+1 {
					t.Error("write did not resolve workspace visibility exactly once")
				}
			}
		})
	}
	if fixture.activations != beforeActivations || readRevisions(t) != beforeRevisions || applied != beforeRevisions || !reflect.DeepEqual(readAdminSavedGraph(t, st), beforeGraph) {
		t.Fatal("denied writes changed saved aggregates, durable/applied revisions or reload count")
	}
	if t.Failed() {
		return
	}

	for _, tc := range []struct {
		name, access string
		id           uuid.UUID
	}{{"a-managed", aAdmin, a.ID}, {"system-managed", global, system}} {
		t.Run("authorized/"+tc.name, func(t *testing.T) {
			id := tc.id.String()
			memberships.fail = tc.id == system
			defer func() { memberships.fail = false }()
			for _, op := range routes {
				sel := selection{query: "?workspace_id=" + id, headers: []string{id}}
				if op.bodySelector {
					sel.body = &id
				}
				before, activations, reads := readRevisions(t), fixture.activations, memberships.reads
				w := send(writeRequest(t, op, tc.access, tc.name, sel))
				if w.Code != http.StatusOK {
					t.Fatalf("%s: %d %s", op.name, w.Code, w.Body.String())
				}
				saved := ""
				if op.resource == "provider" {
					saved = "true"
				}
				after := readRevisions(t)
				if w.Header().Get(catalogSavedHeader) != saved || w.Header().Get("X-Aiproxy-Local-Active") != "true" ||
					w.Header().Get("X-Aiproxy-Cluster-Acknowledged") != "false" || w.Header().Get("X-Aiproxy-Applied-Catalog-Revision") != strconv.FormatInt(after.Catalog, 10) ||
					w.Header().Get("X-Aiproxy-Applied-Key-Revision") != strconv.FormatInt(after.Keys, 10) ||
					fixture.activations != activations+1 || after.Catalog <= before.Catalog || after.Keys != before.Keys || applied != after {
					t.Fatalf("%s did not retain save and local activation semantics: %v", op.name, w.Header())
				}
				if tc.id == system && memberships.reads != reads {
					t.Fatal("global administrator unexpectedly needed membership resolution")
				}
				switch op.name {
				case "provider-update", "credential-update":
					p, models := adminReasoningSavedProvider(t, st, tc.name)
					key := "setup-fixture"
					if op.name == "credential-update" {
						key = "rotated-fixture"
					}
					plain, err := store.DecryptSecret(p.APIKeyEncrypted)
					if p.WorkspaceID != tc.id || p.DisplayName != "Own edit" || len(models) != 1 || models[0].Name != "m" || err != nil || string(plain) != key {
						t.Fatalf("%s did not preserve ownership and save the intended provider aggregate", op.name)
					}
				case "alias-update":
					a, err := st.GetAlias(ctx, tc.name)
					if err != nil || a.WorkspaceID != tc.id || a.Algorithm != "least_connections" {
						t.Fatalf("alias update did not save the intended edit: %v", err)
					}
					targets, err := st.ListAliasTargets(ctx, a.ID)
					if err != nil || len(targets) != 1 || targets[0].Provider != tc.name || targets[0].Model != "m" {
						t.Fatalf("alias update did not retain its target: %v", err)
					}
				case "alias-delete":
					if _, err := st.GetAlias(ctx, tc.name); !errors.Is(err, sql.ErrNoRows) {
						t.Fatalf("alias delete did not remove the row: %v", err)
					}
				case "provider-delete":
					if _, err := st.GetProvider(ctx, tc.name); !errors.Is(err, sql.ErrNoRows) {
						t.Fatalf("provider delete did not remove the row: %v", err)
					}
				}
			}
		})
	}
}
