package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/copilotlogin"
	"github.com/google/uuid"
)

func TestCopilotFlowAuthorityLockWaits(t *testing.T) {
	t.Run("share-before-workspace-prevents-disable-overtaking", func(t *testing.T) {
		a, b, workspace, users := flowStores(t)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		status := flowPending(t, a, users[0].ID, workspace, nil)
		flowDue(t, a, status.ID)
		_, lease, err := a.ClaimCopilotFlowPoll(ctx, users[0].ID, status.ID)
		flowOK(t, err)
		holder, err := a.DB.BeginTx(ctx, nil)
		flowOK(t, err)
		defer holder.Rollback()
		flowOK(t, lockMembershipWorkspace(ctx, holder, workspace))
		cred, err := copilotlogin.NewCredential("client-id", "private-held-success", time.Now())
		flowOK(t, err)
		finalized := make(chan error, 1)
		go func() {
			_, err := b.FinalizeCopilotFlowPoll(ctx, *lease, CopilotFlowPollResult{State: "ready", Credential: cred})
			finalized <- err
		}()
		waitMembershipLocks(t, ctx, a, workspace, 1)
		updated := make(chan error, 1)
		users[0].Disabled = true
		go func() { updated <- a.UpdateUser(ctx, &users[0]) }()
		waitMembershipLocks(t, ctx, a, users[0].ID, 1)
		flowOK(t, holder.Commit())
		flowOK(t, <-finalized)
		flowOK(t, <-updated)
		if flowRow(t, a, status.ID).State != "cancelled" {
			t.Fatal("disable did not erase finalized-but-unconsumed result")
		}
		flowErased(t, a, status.ID)
	})
	for _, boundary := range []string{"start", "status", "claim", "finalize", "snapshot", "consume", "cancel"} {
		t.Run("current-user-after-wait/"+boundary, func(t *testing.T) {
			a, b, workspace, users := flowStores(t)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			var status CopilotFlowStatus
			var lease *CopilotFlowLease
			var snap CopilotFlowSnapshot
			var err error
			p := DBProvider{Name: "save", Type: "github-copilot", WorkspaceID: workspace}
			if boundary == "snapshot" || boundary == "consume" {
				status = flowReady(t, a, users[0].ID, workspace, nil)
				snap, err = a.ReadyCopilotFlow(ctx, users[0].ID, status.ID)
				flowOK(t, err)
				flowOK(t, snap.AttachCredential(&p))
			} else if boundary != "start" {
				status = flowPending(t, a, users[0].ID, workspace, nil)
				flowDue(t, a, status.ID)
				if boundary == "finalize" {
					_, lease, err = a.ClaimCopilotFlowPoll(ctx, users[0].ID, status.ID)
					flowOK(t, err)
				}
			}
			holder, err := a.DB.BeginTx(ctx, nil)
			flowOK(t, err)
			defer holder.Rollback()
			var user User
			flowOK(t, holder.NewSelect().Model(&user).Where("id = ?", users[0].ID).For("UPDATE").Scan(ctx))
			done := make(chan error, 1)
			go func() {
				var err error
				switch boundary {
				case "start":
					_, _, err = b.StartCopilotFlow(ctx, CopilotFlowStart{ActorID: user.ID, WorkspaceID: workspace, ClientID: "client-id"})
				case "status":
					_, err = b.GetCopilotFlow(ctx, user.ID, status.ID)
				case "claim":
					_, _, err = b.ClaimCopilotFlowPoll(ctx, user.ID, status.ID)
				case "finalize":
					_, err = b.FinalizeCopilotFlowPoll(ctx, *lease, CopilotFlowPollResult{State: "pending"})
				case "snapshot":
					_, err = b.ReadyCopilotFlow(ctx, user.ID, status.ID)
				case "consume":
					err = b.ConsumeCopilotFlow(ctx, snap, &p, nil)
				case "cancel":
					_, err = b.CancelCopilotFlow(ctx, user.ID, status.ID)
				}
				done <- err
			}()
			waitMembershipLocks(t, ctx, a, user.ID, 1)
			_, err = holder.ExecContext(ctx, "UPDATE users SET disabled = true WHERE id = ?", user.ID)
			flowOK(t, err)
			flowOK(t, holder.Commit())
			if err := <-done; !errors.Is(err, ErrCopilotFlowUnauthorized) {
				t.Fatal("stale actor passed boundary", err)
			}
			if p.ID != uuid.Nil {
				t.Fatal("unauthorized aggregate committed")
			}
		})
	}
	t.Run("membership-loss-during-workspace-wait", func(t *testing.T) {
		a, b, workspace, users := flowStores(t)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		status := flowPending(t, a, users[0].ID, workspace, nil)
		flowDue(t, a, status.ID)
		_, lease, err := a.ClaimCopilotFlowPoll(ctx, users[0].ID, status.ID)
		flowOK(t, err)
		holder, err := a.DB.BeginTx(ctx, nil)
		flowOK(t, err)
		defer holder.Rollback()
		flowOK(t, lockMembershipUser(ctx, holder, users[0].ID))
		flowOK(t, lockMembershipWorkspace(ctx, holder, workspace))
		done := make(chan error, 1)
		go func() {
			_, err := b.FinalizeCopilotFlowPoll(ctx, *lease, CopilotFlowPollResult{State: "pending"})
			done <- err
		}()
		waitMembershipLocks(t, ctx, a, workspace, 1)
		_, err = holder.ExecContext(ctx, "UPDATE workspace_members SET role='member' WHERE user_id=? AND workspace_id=?", users[0].ID, workspace)
		flowOK(t, err)
		flowOK(t, invalidateCopilotFlows(ctx, holder, users[0].ID, &workspace))
		flowOK(t, holder.Commit())
		if err := <-done; !errors.Is(err, ErrCopilotFlowForbidden) {
			t.Fatal("stale membership after wait", err)
		}
		flowErased(t, a, status.ID)
	})
}

func TestCopilotFlowConsumeLockOrder(t *testing.T) {
	for _, op := range []string{"disable", "demote", "offboard", "delete-user", "delete-provider", "cancel"} {
		t.Run(op, func(t *testing.T) {
			a, b, workspace, users := flowStores(t)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			p := DBProvider{Name: "edit", Type: "github-copilot", WorkspaceID: workspace, CopilotCredentialName: "old", Enabled: true}
			flowOK(t, a.CreateProvider(ctx, &p))
			status := flowReady(t, a, users[0].ID, workspace, &p.ID)
			snap, err := a.ReadyCopilotFlow(ctx, users[0].ID, status.ID)
			flowOK(t, err)
			flowOK(t, snap.AttachCredential(&p))
			holder, err := a.DB.BeginTx(ctx, nil)
			flowOK(t, err)
			defer holder.Rollback()
			var f copilotDeviceFlow
			flowOK(t, holder.NewSelect().Model(&f).Where("id=?", status.ID).For("UPDATE").Scan(ctx))
			consumed := make(chan error, 1)
			go func() { consumed <- b.ConsumeCopilotFlow(ctx, snap, &p, nil) }()
			waitMembershipLocks(t, ctx, a, status.ID, 1)
			changed := make(chan error, 1)
			go func() {
				var err error
				switch op {
				case "disable":
					u := users[0]
					u.Disabled = true
					err = a.UpdateUser(ctx, &u)
				case "demote":
					err = a.SetMembershipRole(ctx, users[1].ID, users[0].ID, workspace, "member")
				case "offboard":
					err = a.DeleteMembership(ctx, users[0].ID, workspace)
				case "delete-user":
					err = a.DeleteUser(ctx, users[0].ID)
				case "delete-provider":
					err = a.DeleteProvider(ctx, *f.ProviderID)
				case "cancel":
					_, err = a.CancelCopilotFlow(ctx, users[0].ID, status.ID)
				}
				changed <- err
			}()
			waitID := workspace
			if op == "disable" || op == "delete-user" {
				waitID = users[0].ID
			}
			waitMembershipLocks(t, ctx, a, waitID, 1)
			flowOK(t, holder.Commit())
			flowOK(t, <-consumed)
			flowOK(t, <-changed)
			if op != "delete-provider" && op != "delete-user" {
				flowErased(t, a, status.ID)
				if flowRow(t, a, status.ID).State != "consumed" {
					t.Fatal("authority change reverted consumed tombstone")
				}
			}
			if op != "delete-provider" {
				saved, err := a.GetProvider(ctx, p.Name)
				flowOK(t, err)
				if len(saved.CopilotCredentialEncrypted) == 0 {
					t.Fatal("saved credential removed by actor change")
				}
			}
		})
	}
}

func TestCopilotFlowExpiryDuringProviderWait(t *testing.T) {
	a, b, workspace, users := flowStores(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	p := DBProvider{Name: "edit", Type: "github-copilot", WorkspaceID: workspace, CopilotCredentialName: "old", Enabled: true}
	flowOK(t, a.CreateProvider(ctx, &p))
	old := p
	status := flowReady(t, a, users[0].ID, workspace, &p.ID)
	snap, err := a.ReadyCopilotFlow(ctx, users[0].ID, status.ID)
	flowOK(t, err)
	flowOK(t, snap.AttachCredential(&p))
	_, err = a.DB.ExecContext(ctx, "UPDATE copilot_device_flows SET expires_at=clock_timestamp()+interval '300 milliseconds' WHERE id=?", status.ID)
	flowOK(t, err)
	holder, err := a.DB.BeginTx(ctx, nil)
	flowOK(t, err)
	defer holder.Rollback()
	var locked DBProvider
	flowOK(t, holder.NewSelect().Model(&locked).Where("id=?", p.ID).For("UPDATE").Scan(ctx))
	done := make(chan error, 1)
	go func() { done <- b.ConsumeCopilotFlow(ctx, snap, &p, nil) }()
	waitMembershipLocks(t, ctx, a, p.ID, 1)
	_, err = holder.ExecContext(ctx, "SELECT pg_sleep(0.35)")
	flowOK(t, err)
	flowOK(t, holder.Commit())
	if err := <-done; !errors.Is(err, ErrCatalogConflict) {
		t.Fatal("expired during wait committed", err)
	}
	saved, err := a.GetProvider(ctx, p.Name)
	flowOK(t, err)
	if saved.UpdatedAt != old.UpdatedAt || saved.CopilotCredentialName != "old" || saved.CopilotCredentialEncrypted != nil {
		t.Fatal("expiry rollback failed")
	}
	status, err = a.GetCopilotFlow(ctx, users[0].ID, status.ID)
	flowOK(t, err)
	if status.Status != "expired" {
		t.Fatal(status)
	}
	flowErased(t, a, status.ID)
}
