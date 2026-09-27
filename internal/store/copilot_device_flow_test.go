package store

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/copilotlogin"
	"github.com/google/uuid"
)

func flowOK(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func flowStores(t *testing.T) (*Store, *Store, uuid.UUID, []User) {
	t.Helper()
	t.Setenv("AIPROXY_DB_ENCRYPTION_KEY", "flow-test-encryption")
	a := openLedgerTestStore(t)
	var schema string
	flowOK(t, a.DB.NewRaw("SELECT current_schema()").Scan(context.Background(), &schema))
	u, err := url.Parse(os.Getenv("AIPROXY_TEST_DATABASE_URL"))
	flowOK(t, err)
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	b, err := Open(context.Background(), u.String())
	flowOK(t, err)
	t.Cleanup(func() { _ = b.Close() })
	workspace, _, users := membershipFixture(t, a, "workspace")
	return a, b, workspace, users
}

func flowStart(t *testing.T, s *Store, user, workspace uuid.UUID, provider *uuid.UUID) (CopilotFlowStatus, *CopilotFlowLease) {
	t.Helper()
	status, lease, err := s.StartCopilotFlow(context.Background(), CopilotFlowStart{ActorID: user, WorkspaceID: workspace, ProviderID: provider, ClientID: "client-id"})
	flowOK(t, err)
	if status.Status != "starting" || lease == nil {
		t.Fatal("missing reservation")
	}
	return status, lease
}

func flowPending(t *testing.T, s *Store, user, workspace uuid.UUID, provider *uuid.UUID) CopilotFlowStatus {
	t.Helper()
	_, lease := flowStart(t, s, user, workspace, provider)
	code := copilotlogin.DeviceCode{DeviceCode: "private-device-code", UserCode: "ABCD-EFGH", VerificationURI: copilotlogin.VerificationURL, ExpiresIn: 30 * time.Minute, Interval: 5 * time.Second}
	out, err := s.FinalizeCopilotFlowStart(context.Background(), *lease, &code)
	flowOK(t, err)
	if out.Status != "pending" {
		t.Fatal(out)
	}
	return out
}

func flowDue(t *testing.T, s *Store, id uuid.UUID) {
	t.Helper()
	_, err := s.DB.ExecContext(context.Background(), "UPDATE copilot_device_flows SET poll_at = clock_timestamp() - interval '1 second' WHERE id = ?", id)
	flowOK(t, err)
}

func flowReady(t *testing.T, s *Store, user, workspace uuid.UUID, provider *uuid.UUID) CopilotFlowStatus {
	t.Helper()
	out := flowPending(t, s, user, workspace, provider)
	flowDue(t, s, out.ID)
	_, lease, err := s.ClaimCopilotFlowPoll(context.Background(), user, out.ID)
	flowOK(t, err)
	if lease == nil {
		t.Fatal("no poll claim")
	}
	cred, err := copilotlogin.NewCredential("client-id", "private-access-token", time.Now())
	flowOK(t, err)
	out, err = s.FinalizeCopilotFlowPoll(context.Background(), *lease, CopilotFlowPollResult{State: "ready", Credential: cred})
	flowOK(t, err)
	if out.Status != "ready" {
		t.Fatal(out)
	}
	return out
}

func flowRow(t *testing.T, s *Store, id uuid.UUID) copilotDeviceFlow {
	t.Helper()
	var f copilotDeviceFlow
	flowOK(t, s.DB.NewSelect().Model(&f).Where("id = ?", id).Scan(context.Background()))
	return f
}

func flowErased(t *testing.T, s *Store, id uuid.UUID) {
	t.Helper()
	f := flowRow(t, s, id)
	if f.ChallengeEncrypted != nil || f.CredentialEncrypted != nil || f.LeaseID != nil || f.LeaseUntil != nil || f.RetainUntil == nil {
		t.Fatal("terminal retained payload/lease")
	}
}

func TestCopilotFlowTwoStoreLifecycle(t *testing.T) {
	a, b, workspace, users := flowStores(t)
	ctx := context.Background()
	actor := users[0].ID
	type startResult struct {
		status CopilotFlowStatus
		lease  *CopilotFlowLease
		err    error
	}
	starts := make(chan startResult, 2)
	gate := make(chan struct{})
	for _, s := range []*Store{a, b} {
		go func(s *Store) {
			<-gate
			status, lease, err := s.StartCopilotFlow(ctx, CopilotFlowStart{ActorID: actor, WorkspaceID: workspace, ClientID: "client-id"})
			starts <- startResult{status, lease, err}
		}(s)
	}
	close(gate)
	var win startResult
	wins, limits := 0, 0
	for range 2 {
		r := <-starts
		if r.err == nil {
			wins++
			win = r
		} else if errors.Is(r.err, ErrCopilotFlowLimit) {
			limits++
		} else {
			t.Fatal(r.err)
		}
	}
	if wins != 1 || limits != 1 {
		t.Fatalf("wins %d limits %d", wins, limits)
	}
	code := copilotlogin.DeviceCode{DeviceCode: "private-device-code", UserCode: "ABCD-EFGH", VerificationURI: copilotlogin.VerificationURL, ExpiresIn: time.Hour, Interval: 5 * time.Second}
	status, err := b.FinalizeCopilotFlowStart(ctx, *win.lease, &code)
	flowOK(t, err)
	f := flowRow(t, a, status.ID)
	if f.ExpiresAt.Sub(f.CreatedAt) != 15*time.Minute || bytes.Contains(f.ChallengeEncrypted, []byte(code.DeviceCode)) || len(f.ChallengeEncrypted) == 0 {
		t.Fatal("pending bounds/encryption")
	}
	public, err := json.Marshal(status)
	flowOK(t, err)
	if bytes.Contains(public, []byte(code.DeviceCode)) || !bytes.Contains(public, []byte(code.UserCode)) {
		t.Fatal("status secret projection")
	}
	if _, err := b.FinalizeCopilotFlowStart(ctx, *win.lease, &code); !errors.Is(err, ErrCopilotFlowUnavailable) {
		t.Fatal("replayed start", err)
	}
	_, lease, err := a.ClaimCopilotFlowPoll(ctx, actor, status.ID)
	flowOK(t, err)
	if lease != nil {
		t.Fatal("early poll claimed")
	}
	flowDue(t, a, status.ID)
	type claimResult struct {
		lease *CopilotFlowLease
		err   error
	}
	claims := make(chan claimResult, 2)
	for _, s := range []*Store{a, b} {
		go func(s *Store) { _, l, e := s.ClaimCopilotFlowPoll(ctx, actor, status.ID); claims <- claimResult{l, e} }(s)
	}
	for range 2 {
		r := <-claims
		flowOK(t, r.err)
		if r.lease != nil {
			if lease != nil {
				t.Fatal("two claims")
			}
			lease = r.lease
		}
	}
	if lease == nil || lease.DeviceCode() != code.DeviceCode || lease.ClientID() != "client-id" || lease.Interval() != 5*time.Second {
		t.Fatal("poll lease missing private input")
	}
	encoded, err := json.Marshal(lease)
	flowOK(t, err)
	if string(encoded) != "{}" {
		t.Fatal("lease serializes secrets")
	}
	status, err = b.FinalizeCopilotFlowPoll(ctx, *lease, CopilotFlowPollResult{State: "pending", Interval: 13 * time.Second})
	flowOK(t, err)
	if status.PollAfterMs < 12900 {
		t.Fatal("slowdown not completion-relative", status)
	}
	flowDue(t, a, status.ID)
	_, lease, err = a.ClaimCopilotFlowPoll(ctx, actor, status.ID)
	flowOK(t, err)
	_, err = b.FinalizeCopilotFlowPoll(ctx, *lease, CopilotFlowPollResult{State: "pending", Interval: time.Second})
	flowOK(t, err)
	if flowRow(t, a, status.ID).IntervalNs != int64(13*time.Second) {
		t.Fatal("lowered upstream advice")
	}
	flowDue(t, a, status.ID)
	_, lease, err = a.ClaimCopilotFlowPoll(ctx, actor, status.ID)
	flowOK(t, err)
	cred, err := copilotlogin.NewCredential("client-id", "private-access-token", time.Now())
	flowOK(t, err)
	status, err = b.FinalizeCopilotFlowPoll(ctx, *lease, CopilotFlowPollResult{State: "ready", Credential: cred})
	flowOK(t, err)
	if status.ReadyExpiresAt == nil || time.Until(*status.ReadyExpiresAt) > 10*time.Minute {
		t.Fatal("ready expiry")
	}
	f = flowRow(t, a, status.ID)
	if f.ChallengeEncrypted != nil || bytes.Contains(f.CredentialEncrypted, []byte(cred.AccessToken)) {
		t.Fatal("ready payload")
	}
	snap, err := a.ReadyCopilotFlow(ctx, actor, status.ID)
	flowOK(t, err)
	encoded, err = json.Marshal(snap)
	flowOK(t, err)
	if string(encoded) != "{}" {
		t.Fatal("snapshot serializes")
	}
	var results [2]DBProvider
	done := make(chan error, 2)
	models := []DBProviderModel{{Name: "chat", Capabilities: []string{"chat"}}}
	for i, s := range []*Store{a, b} {
		results[i] = DBProvider{Name: "connected", Type: "github-copilot", WorkspaceID: workspace, Enabled: true}
		flowOK(t, snap.AttachCredential(&results[i]))
		go func(i int, s *Store) { done <- s.ConsumeCopilotFlow(ctx, snap, &results[i], &models) }(i, s)
	}
	wins = 0
	for range 2 {
		err := <-done
		if err == nil {
			wins++
		} else if !errors.Is(err, ErrCopilotFlowUnavailable) {
			t.Fatal(err)
		}
	}
	if wins != 1 {
		t.Fatal("consume wins", wins)
	}
	p, err := a.GetProvider(ctx, "connected")
	flowOK(t, err)
	saved, err := DecryptCopilotCredential(p.CopilotCredentialEncrypted, time.Now())
	flowOK(t, err)
	if saved != cred {
		t.Fatal("saved credential mismatch")
	}
	status, err = a.GetCopilotFlow(ctx, actor, status.ID)
	flowOK(t, err)
	if status.Status != "consumed" || status.ConsumedProviderID == nil || *status.ConsumedProviderID != p.ID || status.ConsumedUpdatedAt == nil || !status.ConsumedUpdatedAt.Equal(p.UpdatedAt) {
		t.Fatal("lost-response recovery", status)
	}
	flowErased(t, a, status.ID)
	flowOK(t, b.DeleteMembership(ctx, actor, workspace))
	users[0].Disabled = true
	flowOK(t, b.UpdateUser(ctx, &users[0]))
	flowOK(t, b.DeleteUser(ctx, actor))
	after, err := a.GetProvider(ctx, p.Name)
	flowOK(t, err)
	if !bytes.Equal(after.CopilotCredentialEncrypted, p.CopilotCredentialEncrypted) {
		t.Fatal("saved provider tied to actor lifecycle")
	}
}

func TestCopilotFlowAtomicRollbackAndBindings(t *testing.T) {
	a, b, workspace, users := flowStores(t)
	ctx := context.Background()
	actor := users[0].ID
	p := DBProvider{Name: "edit", Type: "github-copilot", WorkspaceID: workspace, CopilotCredentialName: "old-sidecar", Enabled: true}
	oldModels := []DBProviderModel{{Name: "old"}}
	flowOK(t, a.CreateProviderAggregate(ctx, &p, oldModels))
	old := p
	status := flowReady(t, a, actor, workspace, &p.ID)
	snap, err := a.ReadyCopilotFlow(ctx, actor, status.ID)
	flowOK(t, err)
	for _, mutate := range []func(*DBProvider){func(p *DBProvider) { p.WorkspaceID = uuid.New() }, func(p *DBProvider) { p.ID = uuid.New() }, func(p *DBProvider) { p.Type = "openai" }, func(p *DBProvider) { p.UpdatedAt = time.Time{} }} {
		bad := p
		mutate(&bad)
		if err := snap.AttachCredential(&bad); !errors.Is(err, ErrCopilotFlowInput) {
			t.Fatal("binding accepted", err)
		}
	}
	if _, err := b.ReadyCopilotFlow(ctx, users[1].ID, status.ID); !errors.Is(err, ErrCopilotFlowNotFound) {
		t.Fatal("other admin adopted", err)
	}
	flowOK(t, snap.AttachCredential(&p))
	p.DisplayName = "updated"
	bad := p
	bad.CopilotCredentialName = "ambiguous"
	if err := b.ConsumeCopilotFlow(ctx, snap, &bad, nil); !errors.Is(err, ErrCopilotFlowInput) {
		t.Fatal("ambiguous sources", err)
	}
	before := p
	_, err = a.DB.ExecContext(ctx, "ALTER TABLE db_provider_models ADD CONSTRAINT copilot_model_fault CHECK (name <> 'fault')")
	flowOK(t, err)
	models := []DBProviderModel{{Name: "new"}, {Name: "fault"}}
	if err := b.ConsumeCopilotFlow(ctx, snap, &p, &models); err == nil {
		t.Fatal("model fault accepted")
	}
	if !reflect.DeepEqual(p, before) {
		t.Fatal("failed consume mutated caller")
	}
	current, err := a.GetProvider(ctx, p.Name)
	flowOK(t, err)
	if !reflect.DeepEqual(current, old) {
		t.Fatal("model fault changed provider")
	}
	gotModels, err := a.ListProviderModels(ctx, p.ID)
	flowOK(t, err)
	if len(gotModels) != 1 || gotModels[0].Name != "old" {
		t.Fatal("partial models")
	}
	if flowRow(t, a, status.ID).State != "ready" {
		t.Fatal("model failure consumed")
	}
	current.DisplayName = "concurrent"
	flowOK(t, a.UpdateProvider(ctx, &current))
	if err := b.ConsumeCopilotFlow(ctx, snap, &p, nil); !errors.Is(err, ErrCatalogConflict) {
		t.Fatal("stale provider revision", err)
	}
	if flowRow(t, a, status.ID).State != "ready" {
		t.Fatal("conflict consumed")
	}
	p = current
	flowOK(t, snap.AttachCredential(&p))
	flowOK(t, b.ConsumeCopilotFlow(ctx, snap, &p, nil))
	if p.CopilotCredentialName != "" || p.CopilotCredentialPath != "" {
		t.Fatal("source not switched")
	}
	flowErased(t, a, status.ID)
	create := flowReady(t, a, users[1].ID, workspace, nil)
	createSnap, err := a.ReadyCopilotFlow(ctx, users[1].ID, create.ID)
	flowOK(t, err)
	candidate := DBProvider{Name: p.Name, Type: "github-copilot", WorkspaceID: workspace, Enabled: true}
	flowOK(t, createSnap.AttachCredential(&candidate))
	if err := b.ConsumeCopilotFlow(ctx, createSnap, &candidate, nil); err == nil {
		t.Fatal("name conflict accepted")
	}
	if flowRow(t, a, create.ID).State != "ready" || candidate.ID != uuid.Nil {
		t.Fatal("conflicting create consumed")
	}
	candidate.Name = "new-provider"
	if err := b.ConsumeCopilotFlow(ctx, createSnap, &candidate, &models); err == nil {
		t.Fatal("create model fault accepted")
	}
	if _, err := a.GetProvider(ctx, candidate.Name); err == nil {
		t.Fatal("orphan provider")
	}
	if flowRow(t, a, create.ID).State != "ready" {
		t.Fatal("create model fault consumed")
	}
	flowOK(t, b.ConsumeCopilotFlow(ctx, createSnap, &candidate, nil))
}

func TestCopilotFlowExpiryLeasesCancel(t *testing.T) {
	for _, scenario := range []string{"start-expired", "start-failed", "bad-url", "pending-expired", "lease-abandoned", "cancel-inflight", "cancel-ready", "ready-expired", "denied", "issuer-expired", "failed", "wrong-client", "wrong-key"} {
		t.Run(scenario, func(t *testing.T) {
			a, b, workspace, users := flowStores(t)
			ctx := context.Background()
			actor := users[0].ID
			status, lease := flowStart(t, a, actor, workspace, nil)
			code := copilotlogin.DeviceCode{DeviceCode: "secret-device", UserCode: "USER-CODE", VerificationURI: copilotlogin.VerificationURL, ExpiresIn: 15 * time.Minute, Interval: time.Second}
			if scenario == "start-expired" {
				_, err := a.DB.ExecContext(ctx, "UPDATE copilot_device_flows SET expires_at = clock_timestamp() - interval '1 second' WHERE id = ?", status.ID)
				flowOK(t, err)
				_, err = b.FinalizeCopilotFlowStart(ctx, *lease, &code)
				if !errors.Is(err, ErrCopilotFlowUnavailable) {
					t.Fatal(err)
				}
				flowErased(t, a, status.ID)
				return
			}
			if scenario == "start-failed" {
				status, err := b.FinalizeCopilotFlowStart(ctx, *lease, nil)
				flowOK(t, err)
				if status.Status != "failed" {
					t.Fatal(status)
				}
				flowErased(t, a, status.ID)
				return
			}
			if scenario == "bad-url" {
				code.VerificationURI = "https://evil.example/device"
				status, err := b.FinalizeCopilotFlowStart(ctx, *lease, &code)
				flowOK(t, err)
				if status.Status != "failed" {
					t.Fatal(status)
				}
				flowErased(t, a, status.ID)
				return
			}
			status, err := a.FinalizeCopilotFlowStart(ctx, *lease, &code)
			flowOK(t, err)
			if scenario == "pending-expired" {
				_, err := a.DB.ExecContext(ctx, "UPDATE copilot_device_flows SET expires_at = clock_timestamp() - interval '1 second' WHERE id = ?", status.ID)
				flowOK(t, err)
				status, lease, err = b.ClaimCopilotFlowPoll(ctx, actor, status.ID)
				flowOK(t, err)
				if lease != nil || status.Status != "expired" {
					t.Fatal(status)
				}
				flowErased(t, a, status.ID)
				return
			}
			flowDue(t, a, status.ID)
			_, lease, err = a.ClaimCopilotFlowPoll(ctx, actor, status.ID)
			flowOK(t, err)
			if scenario == "lease-abandoned" {
				_, err := a.DB.ExecContext(ctx, "UPDATE copilot_device_flows SET lease_until = clock_timestamp() - interval '1 second' WHERE id = ?", status.ID)
				flowOK(t, err)
			}
			if scenario == "cancel-inflight" {
				_, err := b.CancelCopilotFlow(ctx, actor, status.ID)
				flowOK(t, err)
			}
			cred, err := copilotlogin.NewCredential("client-id", "private-access", time.Now())
			flowOK(t, err)
			result := CopilotFlowPollResult{State: "ready", Credential: cred}
			switch scenario {
			case "denied":
				result.State = "denied"
			case "issuer-expired":
				result.State = "expired"
			case "failed":
				result.State = "raw-secret-issuer-error"
			case "wrong-client":
				result.Credential.ClientID = "another-client"
			case "wrong-key":
				t.Setenv("AIPROXY_DB_ENCRYPTION_KEY", "")
				t.Setenv("AIPROXY_JWT_SECRET", "")
			}
			status, err = b.FinalizeCopilotFlowPoll(ctx, *lease, result)
			if scenario == "lease-abandoned" || scenario == "cancel-inflight" {
				if !errors.Is(err, ErrCopilotFlowUnavailable) {
					t.Fatal("late success revived", err)
				}
				flowErased(t, a, status.ID)
				return
			}
			flowOK(t, err)
			if scenario == "cancel-ready" {
				_, err = b.CancelCopilotFlow(ctx, actor, status.ID)
				flowOK(t, err)
			}
			if scenario == "ready-expired" {
				_, err := a.DB.ExecContext(ctx, "UPDATE copilot_device_flows SET expires_at = clock_timestamp() - interval '1 second' WHERE id = ?", status.ID)
				flowOK(t, err)
				_, err = b.ReadyCopilotFlow(ctx, actor, status.ID)
				if !errors.Is(err, ErrCopilotFlowUnavailable) {
					t.Fatal(err)
				}
			}
			flowErased(t, a, status.ID)
			before := flowRow(t, a, status.ID)
			_, err = a.CancelCopilotFlow(ctx, actor, status.ID)
			flowOK(t, err)
			after := flowRow(t, a, status.ID)
			if after.Revision != before.Revision || !after.RetainUntil.Equal(*before.RetainUntil) {
				t.Fatal("terminal cancel not idempotent")
			}
		})
	}
}

func TestCopilotFlowCurrentAuthorityInvalidation(t *testing.T) {
	for _, scenario := range []string{"remove-readd", "demote-promote", "disable-enable", "system-demote-promote", "provider-delete-recreate", "user-delete", "workspace-delete"} {
		t.Run(scenario, func(t *testing.T) {
			a, b, workspace, users := flowStores(t)
			ctx := context.Background()
			actor := users[0].ID
			if scenario == "system-demote-promote" {
				users[0].IsAdmin = true
				flowOK(t, a.UpdateUser(ctx, &users[0]))
			}
			var p *DBProvider
			var pid *uuid.UUID
			if scenario == "provider-delete-recreate" {
				p = &DBProvider{Name: "bound", Type: "github-copilot", WorkspaceID: workspace}
				flowOK(t, a.CreateProvider(ctx, p))
				pid = &p.ID
			}
			status := flowReady(t, a, actor, workspace, pid)
			snap, err := a.ReadyCopilotFlow(ctx, actor, status.ID)
			flowOK(t, err)
			switch scenario {
			case "remove-readd":
				flowOK(t, b.DeleteMembership(ctx, actor, workspace))
				flowOK(t, b.AddMembership(ctx, &WorkspaceMember{UserID: actor, WorkspaceID: workspace, Role: "admin"}))
			case "demote-promote":
				flowOK(t, b.SetMembershipRole(ctx, users[1].ID, actor, workspace, "member"))
				flowOK(t, b.SetMembershipRole(ctx, users[1].ID, actor, workspace, "admin"))
			case "disable-enable":
				users[0].Disabled = true
				flowOK(t, b.UpdateUser(ctx, &users[0]))
				users[0].Disabled = false
				flowOK(t, b.UpdateUser(ctx, &users[0]))
			case "system-demote-promote":
				users[0].IsAdmin = false
				flowOK(t, b.UpdateUser(ctx, &users[0]))
				users[0].IsAdmin = true
				flowOK(t, b.UpdateUser(ctx, &users[0]))
			case "provider-delete-recreate":
				flowOK(t, b.DeleteProvider(ctx, p.ID))
				p.ID = uuid.Nil
				flowOK(t, b.CreateProvider(ctx, p))
			case "user-delete":
				flowOK(t, b.DeleteUser(ctx, actor))
			case "workspace-delete":
				flowOK(t, b.DeleteWorkspace(ctx, workspace))
			}
			candidate := DBProvider{Name: "new", Type: "github-copilot", WorkspaceID: workspace}
			if p != nil {
				candidate = *p
			}
			if p == nil {
				flowOK(t, snap.AttachCredential(&candidate))
			}
			err = b.ConsumeCopilotFlow(ctx, snap, &candidate, nil)
			if err == nil {
				t.Fatal("revived invalidated flow")
			}
			_, err = a.GetCopilotFlow(ctx, actor, status.ID)
			if strings.Contains(scenario, "delete") {
				if !errors.Is(err, ErrCopilotFlowNotFound) {
					t.Fatal("deleted flow", err)
				}
			} else {
				flowOK(t, err)
				f := flowRow(t, a, status.ID)
				if f.State != "cancelled" || f.ErrorCode != "authority_lost" {
					t.Fatal("authority loss not persistent", f.State)
				}
				flowErased(t, a, status.ID)
			}
		})
	}
	t.Run("current-rights", func(t *testing.T) {
		a, b, workspace, users := flowStores(t)
		ctx := context.Background()
		status := flowPending(t, a, users[0].ID, workspace, nil)
		_, err := b.GetCopilotFlow(ctx, users[1].ID, status.ID)
		if !errors.Is(err, ErrCopilotFlowNotFound) {
			t.Fatal(err)
		}
		_, err = a.DB.ExecContext(ctx, "UPDATE users SET disabled = true WHERE id = ?", users[0].ID)
		flowOK(t, err)
		_, err = b.GetCopilotFlow(ctx, users[0].ID, status.ID)
		if !errors.Is(err, ErrCopilotFlowUnauthorized) {
			t.Fatal("cached user authority", err)
		}
		_, err = a.DB.ExecContext(ctx, "UPDATE users SET disabled = false WHERE id = ?", users[0].ID)
		flowOK(t, err)
		_, err = a.DB.ExecContext(ctx, "UPDATE workspace_members SET role = 'member' WHERE user_id = ?", users[0].ID)
		flowOK(t, err)
		_, err = b.GetCopilotFlow(ctx, users[0].ID, status.ID)
		if !errors.Is(err, ErrCopilotFlowForbidden) {
			t.Fatal("cached membership", err)
		}
		other := Workspace{Name: "other"}
		flowOK(t, a.CreateWorkspace(ctx, &other))
		_, _, err = b.StartCopilotFlow(ctx, CopilotFlowStart{ActorID: users[1].ID, WorkspaceID: other.ID, ClientID: "client-id"})
		if !errors.Is(err, ErrCopilotFlowForbidden) {
			t.Fatal("cross workspace", err)
		}
	})
}

func TestCopilotFlowLimitsAndCleanup(t *testing.T) {
	a, b, workspace, users := flowStores(t)
	ctx := context.Background()
	status, lease := flowStart(t, a, users[0].ID, workspace, nil)
	_, err := a.FinalizeCopilotFlowStart(ctx, *lease, nil)
	flowOK(t, err)
	_, _, err = b.StartCopilotFlow(ctx, CopilotFlowStart{ActorID: users[0].ID, WorkspaceID: workspace, ClientID: "client-id"})
	if !errors.Is(err, ErrCopilotFlowLimit) {
		t.Fatal("failed start bypass", err)
	}
	_, err = a.DB.ExecContext(ctx, "UPDATE copilot_device_flows SET created_at=clock_timestamp()-interval '31 seconds' WHERE id=?", status.ID)
	flowOK(t, err)
	_, err = a.DB.ExecContext(ctx, "UPDATE users SET copilot_flow_started_at=clock_timestamp()-interval '31 seconds' WHERE id=?", users[0].ID)
	flowOK(t, err)
	status, _ = flowStart(t, a, users[0].ID, workspace, nil)
	_, err = b.CancelCopilotFlow(ctx, users[0].ID, status.ID)
	flowOK(t, err)
	_, _, err = b.StartCopilotFlow(ctx, CopilotFlowStart{ActorID: users[0].ID, WorkspaceID: workspace, ClientID: "client-id"})
	if !errors.Is(err, ErrCopilotFlowLimit) {
		t.Fatal("cancel bypass", err)
	}
	flowOK(t, b.DeleteWorkspace(ctx, workspace))
	other := Workspace{Name: "new-workspace"}
	flowOK(t, a.CreateWorkspace(ctx, &other))
	flowOK(t, a.AddMembership(ctx, &WorkspaceMember{UserID: users[0].ID, WorkspaceID: other.ID, Role: "admin"}))
	_, _, err = b.StartCopilotFlow(ctx, CopilotFlowStart{ActorID: users[0].ID, WorkspaceID: other.ID, ClientID: "client-id"})
	if !errors.Is(err, ErrCopilotFlowLimit) {
		t.Fatal("workspace deletion bypassed throttle", err)
	}
	workspace = other.ID
	admins := make([]User, 22)
	for i := range admins {
		admins[i] = User{Email: uuid.NewString() + "@example.com", PasswordHash: "unused", IsAdmin: true}
		flowOK(t, a.CreateUser(ctx, &admins[i]))
	}
	results := make(chan error, len(admins))
	var wg sync.WaitGroup
	for i := range admins {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			s := a
			if i%2 == 1 {
				s = b
			}
			_, _, err := s.StartCopilotFlow(ctx, CopilotFlowStart{ActorID: admins[i].ID, WorkspaceID: workspace, ClientID: "client-id"})
			results <- err
		}(i)
	}
	wg.Wait()
	close(results)
	wins, limited := 0, 0
	for err := range results {
		if err == nil {
			wins++
		} else if errors.Is(err, ErrCopilotFlowLimit) {
			limited++
		} else {
			t.Fatal(err)
		}
	}
	if wins != 20 || limited != 2 {
		t.Fatalf("workspace bounds %d/%d", wins, limited)
	}
	_, err = a.DB.ExecContext(ctx, "UPDATE copilot_device_flows SET expires_at=clock_timestamp()-interval '1 second'")
	flowOK(t, err)
	n, err := b.CleanupCopilotFlows(ctx, 7)
	flowOK(t, err)
	if n != 7 {
		t.Fatal("cleanup batch", n)
	}
	var expired int
	flowOK(t, a.DB.NewRaw("SELECT count(*) FROM copilot_device_flows WHERE state='expired'").Scan(ctx, &expired))
	if expired != 7 {
		t.Fatal("unbounded cleanup", expired)
	}
	_, err = a.CleanupCopilotFlows(ctx, 100)
	flowOK(t, err)
	_, err = a.DB.ExecContext(ctx, `INSERT INTO copilot_device_flows (id,actor_id,workspace_id,purpose,client_id,state,created_at,expires_at,poll_at,retain_until) SELECT gen_random_uuid(), ?, ?, 'create','client-id','failed',clock_timestamp()-interval '1 minute',clock_timestamp(),clock_timestamp(),clock_timestamp()+interval '1 hour' FROM generate_series(1,980)`, users[0].ID, workspace)
	flowOK(t, err)
	_, _, err = b.StartCopilotFlow(ctx, CopilotFlowStart{ActorID: users[0].ID, WorkspaceID: workspace, ClientID: "client-id"})
	if !errors.Is(err, ErrCopilotFlowLimit) {
		t.Fatal("global retained bound", err)
	}
	_, err = a.DB.ExecContext(ctx, "UPDATE copilot_device_flows SET retain_until=clock_timestamp()-interval '1 second'")
	flowOK(t, err)
	counts := make(chan int, 2)
	errs := make(chan error, 2)
	for _, s := range []*Store{a, b} {
		go func(s *Store) { n, err := s.CleanupCopilotFlows(ctx, 100); counts <- n; errs <- err }(s)
	}
	for range 2 {
		flowOK(t, <-errs)
		if n := <-counts; n != 100 {
			t.Fatal(n)
		}
	}
	var count int
	flowOK(t, a.DB.NewRaw("SELECT count(*) FROM copilot_device_flows").Scan(ctx, &count))
	if count != 800 {
		t.Fatal("cleanup overlap", count)
	}
	if _, err := a.CleanupCopilotFlows(ctx, 101); !errors.Is(err, ErrCopilotFlowInput) {
		t.Fatal("unbounded cleanup accepted")
	}
}
