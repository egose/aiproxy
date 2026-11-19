package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"testing"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/store"
)

func TestAdminNamedCopilotStartVisibility(t *testing.T) {
	f := newCopilotFlowHarness(t, openAdminReasoningStore(t))
	ctx := context.Background()
	a, b := f.workspace(t), f.workspace(t)
	system := store.Workspace{ID: store.SystemWorkspaceID}
	aActor, aAdmin := f.workspaceAdmin(t, a)
	dualActor, dualAdmin := f.workspaceAdmin(t, a)
	if err := f.st.AddMembership(ctx, &store.WorkspaceMember{UserID: dualActor.ID, WorkspaceID: b.ID, Role: roleAdmin}); err != nil {
		t.Fatal(err)
	}
	globalActor, global := f.user(t, true)
	_, aMember := f.workspaceMember(t, a)
	_, systemMember := f.workspaceMember(t, system)
	_, noMembership := f.user(t, false)

	rt, err := config.Load([]byte(`
listener "http" "public" { address = "127.0.0.1:8080" }
auth "main" { mode = "none" }
provider "openai" "system-hcl" {
  api_key = "hcl-fixture"
  model "m" {}
}
provider "github-copilot" "system-disabled" {
  enabled = false
  model "m" {}
}
`), "copilot-start-visibility.hcl")
	if err != nil {
		t.Fatal(err)
	}
	activations := 0
	deps := f.h.current()
	deps.Catalog = rt.Catalog
	deps.RequestReload = func() error { activations++; return nil }
	f.h.UpdateDependencies(deps)

	seed := func(name, typ string, ws store.Workspace) store.DBProvider {
		t.Helper()
		p := store.DBProvider{Name: name, Type: typ, WorkspaceID: ws.ID}
		if err := f.st.CreateProviderAggregate(ctx, &p, []store.DBProviderModel{{Name: "m"}}); err != nil {
			t.Fatal(err)
		}
		return p
	}
	aProvider := seed("a-copilot", "github-copilot", a)
	bProvider := seed("b-copilot", "github-copilot", b)
	systemProvider := seed("system-db", "github-copilot", system)
	seed("b-non-copilot", "openai", b)
	beforeGraph, beforeRevisions := readAdminSavedGraph(t, f.st), readDirectWriteRevisions(t, f.st)
	flowCount := func(t *testing.T) int {
		t.Helper()
		var count int
		if err := f.st.DB.NewRaw("SELECT count(*) FROM copilot_device_flows").Scan(ctx, &count); err != nil {
			t.Fatal(err)
		}
		return count
	}

	const path = adminPathPrefix + "copilot-device-flows"
	startBody := func(name, workspaceJSON string) string {
		extra := ""
		if workspaceJSON != "" {
			extra = `,"workspace_id":` + workspaceJSON
		}
		return fmt.Sprintf(`{"client_id":"public-client-id","provider_name":%q%s}`, name, extra)
	}
	const notFound = "provider not found\n"
	const invalidID = "invalid workspace id\n"
	memberships := &workspaceMembershipReadHook{}
	f.st.DB.AddQueryHook(catalogDenialQueryHook{})
	f.st.DB.AddQueryHook(memberships)
	reject := func(t *testing.T, access, target, workspaceJSON string, selection directWriteSelection, code int, body string, maxReferences int) {
		t.Helper()
		audit := &catalogDenialAudit{}
		w := callDirectWrite(f.h, context.WithValue(ctx, catalogDenialAuditKey{}, audit), access, http.MethodPost, path, startBody(target, workspaceJSON), selection)
		if w.Code != code || w.Body.String() != body {
			t.Errorf("start %s: %d %q, want %d %q", target, w.Code, w.Body.String(), code, body)
		}
		if len(audit.unexpected) != 0 || audit.references > maxReferences {
			t.Errorf("denial read beyond authorized metadata: references=%d (max %d), unexpected=%q", audit.references, maxReferences, audit.unexpected)
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Error("device response missing no-store")
		}
		assertNoDirectWriteMarkers(t, w.Header())
	}
	noSelection := directWriteSelection{}
	for _, name := range []string{bProvider.Name, "b-non-copilot", systemProvider.Name, "system-hcl", "system-disabled", "missing"} {
		reject(t, aAdmin, name, "", noSelection, http.StatusNotFound, notFound, 1)
		reject(t, noMembership, name, "", noSelection, http.StatusNotFound, notFound, 0)
	}
	reject(t, systemMember, aProvider.Name, "", noSelection, http.StatusNotFound, notFound, 1)
	reject(t, systemMember, systemProvider.Name, "", noSelection, http.StatusNotFound, "workspace not found\n", 1)
	reject(t, aMember, aProvider.Name, "", noSelection, http.StatusForbidden, "workspace admin required\n", 1)
	for _, name := range []string{"system-hcl", "system-disabled"} {
		for _, access := range []string{systemMember, global} {
			reject(t, access, name, "", noSelection, http.StatusBadRequest, "config-managed: edit the HCL file\n", 1)
		}
		reject(t, global, name, "", directWriteSelection{headers: []string{a.ID.String()}}, http.StatusNotFound, notFound, 1)
	}

	aID, bID := a.ID.String(), b.ID.String()
	for _, actor := range []struct{ name, access string }{{"a-only", aAdmin}, {"dual", dualAdmin}, {"global", global}} {
		t.Run(actor.name, func(t *testing.T) {
			for _, tc := range deniedDirectWriteSelections(aID, bID) {
				t.Run(tc.name, func(t *testing.T) {
					for _, name := range []string{aProvider.Name, "missing"} {
						if tc.malformed {
							reject(t, actor.access, name, "", tc.selection, http.StatusBadRequest, invalidID, 0)
						} else {
							reject(t, actor.access, name, "", tc.selection, http.StatusNotFound, notFound, 1)
						}
					}
				})
			}
			matching := directWriteSelection{query: "?workspace_id=" + aID, headers: []string{aID}}
			for _, name := range []string{aProvider.Name, "missing"} {
				reject(t, actor.access, name, fmt.Sprintf("%q", bID), noSelection, http.StatusNotFound, notFound, 1)
				reject(t, actor.access, name, fmt.Sprintf("%q", bID), matching, http.StatusNotFound, notFound, 0)
				reject(t, actor.access, name, fmt.Sprintf("%q", aID), directWriteSelection{query: "?workspace_id=" + bID}, http.StatusNotFound, notFound, 0)
				for _, invalid := range []string{`"not-a-uuid"`, `""`, `null`, `42`} {
					reject(t, actor.access, name, invalid, matching, http.StatusBadRequest, invalidID, 0)
				}
			}
		})
	}
	memberships.fail = true
	for _, name := range []string{aProvider.Name, "b-non-copilot", "system-hcl", "missing"} {
		reads := memberships.reads
		reject(t, aAdmin, name, "", noSelection, http.StatusInternalServerError, "could not resolve workspace visibility\n", 0)
		if memberships.reads != reads+1 {
			t.Error("named start did not resolve workspace visibility exactly once")
		}
	}
	memberships.fail = false

	w := callDirectWrite(f.h, ctx, dualAdmin, http.MethodPost, path, startBody("b-non-copilot", ""), noSelection)
	if w.Code != http.StatusBadRequest || w.Body.String() != "device authorization is only supported by github-copilot\n" {
		t.Errorf("visible non-Copilot start: %d %q", w.Code, w.Body.String())
	}
	assertNoDirectWriteMarkers(t, w.Header())
	var throttled int
	if err := f.st.DB.NewRaw("SELECT count(*) FROM users WHERE copilot_flow_started_at IS NOT NULL").Scan(ctx, &throttled); err != nil {
		t.Fatal(err)
	}
	if code, token := f.issuer.counts(); code != 0 || token != 0 || flowCount(t) != 0 || throttled != 0 {
		t.Error("rejected starts persisted or throttled a flow, or contacted the mock issuer")
	}
	if t.Failed() {
		t.FailNow()
	}

	for i, tc := range []struct {
		actor     store.User
		access    string
		provider  store.DBProvider
		selection directWriteSelection
		workspace string
	}{
		{aActor, aAdmin, aProvider, noSelection, ""},
		{dualActor, dualAdmin, bProvider, directWriteSelection{query: "?workspace_id=" + bID + "&workspace_id=" + bID, headers: []string{bID, bID}}, fmt.Sprintf("%q", bID)},
		{globalActor, global, systemProvider, directWriteSelection{query: "?workspace_id=" + system.ID.String()}, ""},
	} {
		w := callDirectWrite(f.h, ctx, tc.access, http.MethodPost, path, startBody(tc.provider.Name, tc.workspace), tc.selection)
		var status store.CopilotFlowStatus
		if err := json.Unmarshal(w.Body.Bytes(), &status); w.Code != http.StatusCreated || err != nil || status.Status != "pending" || status.WorkspaceID != tc.provider.WorkspaceID {
			t.Fatalf("authorized %s: %d %s (decode: %v)", tc.provider.Name, w.Code, w.Body.String(), err)
		}
		assertNoFlowSecrets(t, w.Body.String())
		assertNoDirectWriteMarkers(t, w.Header())
		binding, err := f.st.ReadCopilotFlowBinding(ctx, tc.actor.ID, status.ID)
		if err != nil || binding.ProviderID == nil || *binding.ProviderID != tc.provider.ID || binding.ProviderName != tc.provider.Name || binding.WorkspaceID != tc.provider.WorkspaceID {
			t.Fatalf("start did not persist the authorized provider binding: %+v, %v", binding, err)
		}
		if code, token := f.issuer.counts(); code != i+1 || token != 0 || flowCount(t) != i+1 {
			t.Fatalf("authorized start %s did not persist one flow and issue one challenge", tc.provider.Name)
		}
	}
	if activations != 0 || readDirectWriteRevisions(t, f.st) != beforeRevisions || !reflect.DeepEqual(readAdminSavedGraph(t, f.st), beforeGraph) {
		t.Fatal("starting a flow changed the saved catalog, revisions or reload count")
	}
}
