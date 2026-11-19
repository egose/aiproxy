package httpapi

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/egose/aiproxy/internal/store"
)

func TestTenantCopilotPolicyRejectsBeforeFlowPersistenceDecryptionOrIssuerIO(t *testing.T) {
	f := openCopilotFlowHarness(t)
	workspace := f.workspace(t)
	actor, tenant := f.workspaceAdmin(t, workspace)
	global, operator := f.user(t, true)
	ctx := context.Background()
	deps := f.h.current()
	deps.MultiTenancy.AllowedUpstreamOrigins = nil
	f.h.UpdateDependencies(deps)
	legacy := store.DBProvider{Name: "blocked-copilot", Type: "github-copilot", WorkspaceID: workspace.ID,
		BaseURL: "https://127.0.0.1", Enabled: false}
	if err := f.st.CreateProviderAggregate(ctx, &legacy, []store.DBProviderModel{{Name: "m"}}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.st.DeleteProvider(ctx, legacy.ID) })
	for _, access := range []string{tenant, operator} {
		r := f.call(t, access, http.MethodPost, "/_internal/admin/copilot-device-flows", map[string]string{
			"client_id": "public-client-id", "provider_name": legacy.Name,
		})
		if r.code != 400 || !strings.Contains(r.raw, "operator-approved") {
			t.Fatalf("forbidden bound provider did not precede device start: %d %s", r.code, r.raw)
		}
	}
	for _, id := range []string{actor.ID.String(), global.ID.String()} {
		var count int
		if err := f.st.DB.NewRaw("SELECT count(*) FROM copilot_device_flows WHERE actor_id = ?", id).Scan(ctx, &count); err != nil || count != 0 {
			t.Fatal("forbidden device start persisted a flow")
		}
	}
	if code, token := f.issuer.counts(); code != 0 || token != 0 {
		t.Fatal("forbidden device start contacted the issuer")
	}
	deps.MultiTenancy.AllowedUpstreamOrigins = []string{"https://api.githubcopilot.com"}
	f.h.UpdateDependencies(deps)
	legacy.BaseURL = "https://api.githubcopilot.com"
	if err := f.st.UpdateProvider(ctx, &legacy); err != nil {
		t.Fatal(err)
	}
	start := f.call(t, tenant, http.MethodPost, "/_internal/admin/copilot-device-flows", map[string]string{
		"client_id": "public-client-id", "provider_name": legacy.Name,
	})
	if start.code != 201 {
		t.Fatalf("approved own device start failed: %d %s", start.code, start.raw)
	}
	id := flowIDOf(t, start)
	if _, err := f.st.DB.ExecContext(ctx, "UPDATE copilot_device_flows SET challenge_encrypted = ? WHERE id = ?", []byte(strings.Repeat("corrupt-", 8)), id); err != nil {
		t.Fatal(err)
	}
	snapshot := func(id string) string {
		t.Helper()
		var raw string
		if err := f.st.DB.NewRaw("SELECT to_jsonb(f)::text FROM copilot_device_flows AS f WHERE id = ?", id).Scan(ctx, &raw); err != nil {
			t.Fatal(err)
		}
		return raw
	}
	before := snapshot(id)
	codeBefore, tokenBefore := f.issuer.counts()
	deps.MultiTenancy.AllowedUpstreamOrigins = nil
	f.h.UpdateDependencies(deps)
	poll := f.call(t, tenant, http.MethodPost, "/_internal/admin/copilot-device-flows/"+id+"/poll", nil)
	if poll.code != 400 || !strings.Contains(poll.raw, "operator-approved") || strings.Contains(poll.raw, "decrypt") ||
		snapshot(id) != before {
		t.Fatalf("forbidden poll decrypted or changed a flow: %d %s", poll.code, poll.raw)
	}
	if code, token := f.issuer.counts(); code != codeBefore || token != tokenBefore {
		t.Fatal("forbidden poll contacted the issuer")
	}
	legacy.BaseURL = "https://127.0.0.1"
	if err := f.st.UpdateProvider(ctx, &legacy); err != nil {
		t.Fatal(err)
	}
	deps.MultiTenancy.AllowedUpstreamOrigins = []string{"https://api.githubcopilot.com"}
	f.h.UpdateDependencies(deps)
	for _, access := range []string{tenant, operator} {
		r := f.call(t, access, http.MethodPost, "/_internal/admin/copilot-device-flows", map[string]string{
			"client_id": "public-client-id", "provider_name": legacy.Name,
		})
		if r.code != 400 || !strings.Contains(r.raw, "operator-approved") {
			t.Fatalf("forbidden old provider authorized a credential flow: %d %s", r.code, r.raw)
		}
	}
	if code, token := f.issuer.counts(); code != codeBefore || token != tokenBefore {
		t.Fatal("forbidden old-row device start contacted the issuer")
	}
}

func TestTenantCopilotProviderPolicyPrecedesReadyCredentialDecryption(t *testing.T) {
	f := openCopilotFlowHarness(t)
	workspace := f.workspace(t)
	_, access := f.workspaceAdmin(t, workspace)
	ctx := context.Background()
	deps := f.h.current()
	deps.MultiTenancy.AllowedUpstreamOrigins = nil
	f.h.UpdateDependencies(deps)
	start := f.call(t, access, http.MethodPost, "/_internal/admin/copilot-device-flows", map[string]string{
		"client_id": "public-client-id", "workspace_id": workspace.ID.String(),
	})
	id := flowIDOf(t, start)
	f.expeditePoll(t, id)
	f.issuer.enqueue("success")
	if ready := f.call(t, access, http.MethodPost, "/_internal/admin/copilot-device-flows/"+id+"/poll", nil); ready.code != 200 || ready.parsed["status"] != "ready" {
		t.Fatalf("own mock flow did not become ready: %d %s", ready.code, ready.raw)
	}
	deps.MultiTenancy.AllowedUpstreamOrigins = []string{"https://api.githubcopilot.com"}
	f.h.UpdateDependencies(deps)
	if _, err := f.st.DB.ExecContext(ctx, "UPDATE copilot_device_flows SET credential_encrypted = ? WHERE id = ?", []byte(strings.Repeat("corrupt-", 8)), id); err != nil {
		t.Fatal(err)
	}
	var before string
	if err := f.st.DB.NewRaw("SELECT to_jsonb(f)::text FROM copilot_device_flows AS f WHERE id = ?", id).Scan(ctx, &before); err != nil {
		t.Fatal(err)
	}
	save := f.call(t, access, http.MethodPost, "/_internal/admin/providers", map[string]any{
		"name": "rejected-copilot", "type": "github-copilot", "workspace_id": workspace.ID.String(),
		"base_url": "https://127.0.0.1", "copilot_device_flow_id": id, "models": []map[string]string{{"name": "m"}},
	})
	if save.code != 400 || !strings.Contains(save.raw, "operator-approved") || save.header.Get(catalogSavedHeader) != "" {
		t.Fatalf("forbidden save resolved the corrupt ready credential before policy: %d %s", save.code, save.raw)
	}
	var after string
	if err := f.st.DB.NewRaw("SELECT to_jsonb(f)::text FROM copilot_device_flows AS f WHERE id = ?", id).Scan(ctx, &after); err != nil || before != after {
		t.Fatal("forbidden provider save consumed or changed the ready flow")
	}
	if _, err := f.st.GetProvider(ctx, "rejected-copilot"); err == nil {
		t.Fatal("forbidden ready-flow save persisted a provider")
	}
	control := f.call(t, access, http.MethodPost, "/_internal/admin/providers", map[string]any{
		"name": "rejected-copilot", "type": "github-copilot", "workspace_id": workspace.ID.String(),
		"copilot_device_flow_id": id, "models": []map[string]string{{"name": "m"}},
	})
	if control.code != http.StatusConflict {
		t.Fatalf("approved configuration did not detect the deliberately corrupt credential: %d %s", control.code, control.raw)
	}
}

func TestTenantApprovalCannotConsumeSystemOrForeignCopilotCredentials(t *testing.T) {
	for _, system := range []bool{false, true} {
		t.Run(map[bool]string{false: "foreign", true: "System"}[system], func(t *testing.T) {
			f := openCopilotFlowHarness(t)
			destination := f.workspace(t)
			source := f.workspace(t)
			if system {
				source.ID = store.SystemWorkspaceID
			}
			_, access := f.user(t, true)
			start := f.call(t, access, http.MethodPost, "/_internal/admin/copilot-device-flows", map[string]string{
				"client_id": "public-client-id", "workspace_id": source.ID.String(),
			})
			id := flowIDOf(t, start)
			f.expeditePoll(t, id)
			f.issuer.enqueue("success")
			if ready := f.call(t, access, http.MethodPost, "/_internal/admin/copilot-device-flows/"+id+"/poll", nil); ready.code != 200 || ready.parsed["status"] != "ready" {
				t.Fatalf("owned source flow did not become ready: %d %s", ready.code, ready.raw)
			}
			ctx := context.Background()
			if _, err := f.st.DB.ExecContext(ctx, "UPDATE copilot_device_flows SET credential_encrypted = ? WHERE id = ?", []byte(strings.Repeat("corrupt-", 8)), id); err != nil {
				t.Fatal(err)
			}
			var before string
			if err := f.st.DB.NewRaw("SELECT to_jsonb(f)::text FROM copilot_device_flows AS f WHERE id = ?", id).Scan(ctx, &before); err != nil {
				t.Fatal(err)
			}
			save := f.call(t, access, http.MethodPost, "/_internal/admin/providers", map[string]any{
				"name": "rejected-copilot", "type": "github-copilot", "workspace_id": destination.ID.String(),
				"copilot_device_flow_id": id, "models": []map[string]string{{"name": "m"}},
			})
			if save.code != 400 || !strings.Contains(save.raw, "does not authorize") || save.header.Get(catalogSavedHeader) != "" {
				t.Fatalf("approved destination inspected/consumed a foreign ready credential: %d %s", save.code, save.raw)
			}
			var after string
			if err := f.st.DB.NewRaw("SELECT to_jsonb(f)::text FROM copilot_device_flows AS f WHERE id = ?", id).Scan(ctx, &after); err != nil || before != after {
				t.Fatal("foreign credential rejection changed or consumed the source flow")
			}
			if _, err := f.st.GetProvider(ctx, "rejected-copilot"); err == nil {
				t.Fatal("foreign credential rejection persisted a provider")
			}
		})
	}
}
