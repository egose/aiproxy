package store

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/egose/aiproxy/internal/copilotlogin"
	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

var (
	ErrCopilotFlowNotFound     = errors.New("device flow not found")
	ErrCopilotFlowUnauthorized = errors.New("device flow actor is not active")
	ErrCopilotFlowForbidden    = errors.New("device flow requires current workspace authority")
	ErrCopilotFlowLimit        = errors.New("device flow admission limit reached")
	ErrCopilotFlowUnavailable  = errors.New("device flow is no longer available")
	ErrCopilotFlowInput        = errors.New("invalid device flow input")
)

type copilotDeviceFlow struct {
	bun.BaseModel       `bun:"table:copilot_device_flows,alias:f"`
	ID                  uuid.UUID `bun:"id,pk"`
	ActorID             uuid.UUID
	WorkspaceID         uuid.UUID
	Purpose             string
	ProviderID          *uuid.UUID
	ProviderName        string
	ClientID            string
	State               string
	ChallengeEncrypted  []byte
	CredentialEncrypted []byte
	CreatedAt           time.Time
	ExpiresAt           time.Time
	PollAt              time.Time
	IntervalNs          int64
	LeaseID             *uuid.UUID
	LeaseUntil          *time.Time
	Revision            int64
	RetainUntil         *time.Time
	ErrorCode           string
	ConsumedProviderID  *uuid.UUID
	ConsumedUpdatedAt   *time.Time
}

type CopilotFlowStatus struct {
	ID                 uuid.UUID  `json:"id"`
	WorkspaceID        uuid.UUID  `json:"workspace_id"`
	Status             string     `json:"status"`
	UserCode           string     `json:"user_code,omitempty"`
	VerificationURI    string     `json:"verification_uri,omitempty"`
	ExpiresAt          *time.Time `json:"expires_at,omitempty"`
	PollAfterMs        int64      `json:"poll_after_ms,omitempty"`
	ReadyExpiresAt     *time.Time `json:"ready_expires_at,omitempty"`
	ProviderName       string     `json:"provider_name,omitempty"`
	ConsumedProviderID *uuid.UUID `json:"consumed_provider_id,omitempty"`
	ConsumedUpdatedAt  *time.Time `json:"consumed_updated_at,omitempty"`
	ErrorCode          string     `json:"error_code,omitempty"`
}

type CopilotFlowStart struct {
	ActorID     uuid.UUID
	WorkspaceID uuid.UUID
	ProviderID  *uuid.UUID
	ClientID    string
}

type CopilotFlowLease struct {
	flowID     uuid.UUID
	actorID    uuid.UUID
	leaseID    uuid.UUID
	revision   int64
	clientID   string
	deviceCode string
	interval   time.Duration
}

func (l CopilotFlowLease) ClientID() string        { return l.clientID }
func (l CopilotFlowLease) DeviceCode() string      { return l.deviceCode }
func (l CopilotFlowLease) Interval() time.Duration { return l.interval }

type CopilotFlowPollResult struct {
	State      string
	Interval   time.Duration
	Credential copilotlogin.Credential `json:"-"`
}

type copilotChallenge struct {
	Version         int
	DeviceCode      string
	UserCode        string
	VerificationURI string
}

func flowText(s string, max int) bool {
	if len(s) == 0 || len(s) > max {
		return false
	}
	for _, c := range []byte(s) {
		if c < 33 || c > 126 {
			return false
		}
	}
	return true
}

func flowNow(ctx context.Context, tx bun.Tx) (time.Time, error) {
	var now time.Time
	err := tx.NewRaw("SELECT clock_timestamp()").Scan(ctx, &now)
	return now, err
}

func flowAuthority(ctx context.Context, tx bun.Tx, actorID, workspaceID uuid.UUID) error {
	return flowAuthorityLock(ctx, tx, actorID, workspaceID, "SHARE")
}

func flowAuthorityLock(ctx context.Context, tx bun.Tx, actorID, workspaceID uuid.UUID, lock string) error {
	var user User
	if err := tx.NewSelect().Model(&user).Where("id = ?", actorID).For(lock).Scan(ctx); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrCopilotFlowUnauthorized
		}
		return err
	}
	if user.Disabled {
		return ErrCopilotFlowUnauthorized
	}
	if err := lockMembershipWorkspace(ctx, tx, workspaceID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrCopilotFlowNotFound
		}
		return err
	}
	if user.IsAdmin {
		return nil
	}
	allowed, err := tx.NewSelect().Model((*WorkspaceMember)(nil)).Where("user_id = ? AND workspace_id = ? AND role = 'admin'", actorID, workspaceID).Exists(ctx)
	if err != nil {
		return err
	}
	if !allowed || workspaceID == SystemWorkspaceID {
		return ErrCopilotFlowForbidden
	}
	return nil
}

func flowTerminal(f *copilotDeviceFlow, state, code string, now time.Time) {
	f.State, f.ErrorCode = state, code
	f.ChallengeEncrypted, f.CredentialEncrypted = nil, nil
	f.LeaseID, f.LeaseUntil = nil, nil
	t := now.Add(time.Hour)
	f.RetainUntil = &t
	f.Revision++
}

func flowLive(f *copilotDeviceFlow) bool {
	return f.State == "starting" || f.State == "pending" || f.State == "ready"
}

func expireFlow(f *copilotDeviceFlow, now time.Time) bool {
	if !flowLive(f) {
		return false
	}
	if !now.Before(f.ExpiresAt) {
		flowTerminal(f, "expired", "expired", now)
		return true
	}
	if f.LeaseUntil != nil && !now.Before(*f.LeaseUntil) {
		flowTerminal(f, "failed", "lease_abandoned", now)
		return true
	}
	return false
}

func saveFlow(ctx context.Context, tx bun.Tx, f *copilotDeviceFlow) error {
	_, err := tx.NewUpdate().Model(f).WherePK().Exec(ctx)
	return err
}

func flowStatus(f *copilotDeviceFlow, now time.Time) (CopilotFlowStatus, error) {
	out := CopilotFlowStatus{ID: f.ID, WorkspaceID: f.WorkspaceID, Status: f.State, ProviderName: f.ProviderName, ErrorCode: f.ErrorCode, ConsumedProviderID: f.ConsumedProviderID, ConsumedUpdatedAt: f.ConsumedUpdatedAt}
	if f.State == "ready" {
		out.ReadyExpiresAt = &f.ExpiresAt
	}
	if f.State == "starting" || f.State == "pending" {
		out.ExpiresAt = &f.ExpiresAt
		next := f.PollAt
		if f.LeaseUntil != nil && next.Before(*f.LeaseUntil) {
			next = *f.LeaseUntil
		}
		if next.After(now) {
			out.PollAfterMs = max(1, int64(next.Sub(now)/time.Millisecond)+1)
		}
	}
	if f.State == "pending" {
		c, err := decodeChallenge(f.ChallengeEncrypted)
		if err != nil {
			return CopilotFlowStatus{}, err
		}
		out.UserCode, out.VerificationURI = c.UserCode, c.VerificationURI
	}
	return out, nil
}

func decodeChallenge(blob []byte) (copilotChallenge, error) {
	var c copilotChallenge
	plain, err := DecryptSecret(blob)
	if err != nil || len(plain) > 16384 {
		return c, ErrCopilotFlowUnavailable
	}
	if json.Unmarshal(plain, &c) != nil || c.Version != 1 || !flowText(c.DeviceCode, 8192) || !flowText(c.UserCode, 256) || c.VerificationURI != copilotlogin.VerificationURL {
		return copilotChallenge{}, ErrCopilotFlowUnavailable
	}
	return c, nil
}

func (s *Store) withCopilotFlow(ctx context.Context, actorID, id uuid.UUID, fn func(context.Context, bun.Tx, *copilotDeviceFlow, time.Time) error) (CopilotFlowStatus, error) {
	var out CopilotFlowStatus
	var outcome error
	err := s.DB.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		var f copilotDeviceFlow
		if err := tx.NewSelect().Model(&f).Where("id = ? AND actor_id = ?", id, actorID).Scan(ctx); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrCopilotFlowNotFound
			}
			return err
		}
		if err := flowAuthority(ctx, tx, actorID, f.WorkspaceID); err != nil {
			return err
		}
		if err := tx.NewSelect().Model(&f).Where("id = ? AND actor_id = ?", id, actorID).For("UPDATE").Scan(ctx); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrCopilotFlowNotFound
			}
			return err
		}
		now, err := flowNow(ctx, tx)
		if err != nil {
			return err
		}
		if f.RetainUntil != nil && !now.Before(*f.RetainUntil) {
			if _, err := tx.NewDelete().Model(&f).WherePK().Exec(ctx); err != nil {
				return err
			}
			outcome = ErrCopilotFlowNotFound
			return nil
		}
		if expireFlow(&f, now) {
			if err := saveFlow(ctx, tx, &f); err != nil {
				return err
			}
		}
		if fn != nil {
			outcome = fn(ctx, tx, &f, now)
			if outcome != nil && !errors.Is(outcome, ErrCopilotFlowUnavailable) && !errors.Is(outcome, ErrCopilotFlowInput) {
				return outcome
			}
		}
		out, err = flowStatus(&f, now)
		if err != nil {
			flowTerminal(&f, "failed", "authorization_failed", now)
			if err := saveFlow(ctx, tx, &f); err != nil {
				return err
			}
			out, _ = flowStatus(&f, now)
			outcome = ErrCopilotFlowUnavailable
		}
		return nil
	})
	if err != nil {
		return CopilotFlowStatus{}, err
	}
	return out, outcome
}

func cleanupCopilotFlowsTx(ctx context.Context, tx bun.Tx, limit int) (int, error) {
	var rows []copilotDeviceFlow
	err := tx.NewSelect().Model(&rows).Where("(state IN ('starting','pending','ready') AND (expires_at <= clock_timestamp() OR lease_until <= clock_timestamp())) OR retain_until <= clock_timestamp()").Order("id").Limit(limit).For("UPDATE SKIP LOCKED").Scan(ctx)
	if err != nil {
		return 0, err
	}
	now, err := flowNow(ctx, tx)
	if err != nil {
		return 0, err
	}
	for i := range rows {
		f := &rows[i]
		if f.RetainUntil != nil && !now.Before(*f.RetainUntil) {
			if _, err := tx.NewDelete().Model(f).WherePK().Exec(ctx); err != nil {
				return 0, err
			}
		} else if expireFlow(f, now) {
			if err := saveFlow(ctx, tx, f); err != nil {
				return 0, err
			}
		}
	}
	return len(rows), nil
}

func (s *Store) CleanupCopilotFlows(ctx context.Context, limit int) (int, error) {
	if limit < 1 || limit > 100 {
		return 0, ErrCopilotFlowInput
	}
	var count int
	err := s.DB.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		var err error
		count, err = cleanupCopilotFlowsTx(ctx, tx, limit)
		return err
	})
	return count, err
}

func (s *Store) StartCopilotFlow(ctx context.Context, in CopilotFlowStart) (CopilotFlowStatus, *CopilotFlowLease, error) {
	if !flowText(in.ClientID, 256) {
		return CopilotFlowStatus{}, nil, ErrCopilotFlowInput
	}
	var out CopilotFlowStatus
	var lease *CopilotFlowLease
	var denied bool
	err := s.DB.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if err := flowAuthorityLock(ctx, tx, in.ActorID, in.WorkspaceID, "UPDATE"); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(728196402)"); err != nil {
			return err
		}
		if _, err := cleanupCopilotFlowsTx(ctx, tx, 100); err != nil {
			return err
		}
		now, err := flowNow(ctx, tx)
		if err != nil {
			return err
		}
		var total, workspace, actor int
		if err := tx.NewRaw(`SELECT count(*), count(*) FILTER (WHERE workspace_id = ? AND state IN ('starting','pending','ready')), count(*) FILTER (WHERE actor_id = ? AND (created_at > ? OR (workspace_id = ? AND state IN ('starting','pending','ready')))) FROM copilot_device_flows`, in.WorkspaceID, in.ActorID, now.Add(-30*time.Second), in.WorkspaceID).Scan(ctx, &total, &workspace, &actor); err != nil {
			return err
		}
		if total >= 1000 || workspace >= 20 || actor > 0 {
			denied = true
			return nil
		}
		var throttled bool
		if err := tx.NewRaw("SELECT COALESCE(copilot_flow_started_at > ?, false) FROM users WHERE id = ?", now.Add(-30*time.Second), in.ActorID).Scan(ctx, &throttled); err != nil {
			return err
		}
		if throttled {
			denied = true
			return nil
		}
		f := copilotDeviceFlow{ID: uuid.New(), ActorID: in.ActorID, WorkspaceID: in.WorkspaceID, Purpose: "create", ClientID: in.ClientID, State: "starting", IntervalNs: int64(5 * time.Second), Revision: 1}
		if in.ProviderID != nil {
			var p DBProvider
			if err := tx.NewSelect().Model(&p).Where("id = ? AND workspace_id = ? AND type = 'github-copilot'", *in.ProviderID, in.WorkspaceID).For("SHARE").Scan(ctx); err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					return ErrCopilotFlowNotFound
				}
				return err
			}
			f.Purpose, f.ProviderID, f.ProviderName = "edit", &p.ID, p.Name
		}
		now, err = flowNow(ctx, tx)
		if err != nil {
			return err
		}
		f.CreatedAt, f.ExpiresAt, f.PollAt = now, now.Add(30*time.Second), now.Add(30*time.Second)
		lid := uuid.New()
		until := f.ExpiresAt
		f.LeaseID, f.LeaseUntil = &lid, &until
		if _, err := tx.NewInsert().Model(&f).Exec(ctx); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "UPDATE users SET copilot_flow_started_at = ? WHERE id = ?", now, in.ActorID); err != nil {
			return err
		}
		out, err = flowStatus(&f, now)
		lease = &CopilotFlowLease{flowID: f.ID, actorID: f.ActorID, leaseID: lid, revision: f.Revision, clientID: f.ClientID}
		return err
	})
	if err != nil {
		return CopilotFlowStatus{}, nil, err
	}
	if denied {
		return CopilotFlowStatus{}, nil, ErrCopilotFlowLimit
	}
	return out, lease, nil
}

func leaseMatches(f *copilotDeviceFlow, l CopilotFlowLease) bool {
	return f.Revision == l.revision && f.LeaseID != nil && *f.LeaseID == l.leaseID
}

func (s *Store) FinalizeCopilotFlowStart(ctx context.Context, l CopilotFlowLease, code *copilotlogin.DeviceCode) (CopilotFlowStatus, error) {
	return s.withCopilotFlow(ctx, l.actorID, l.flowID, func(ctx context.Context, tx bun.Tx, f *copilotDeviceFlow, now time.Time) error {
		if f.State != "starting" || !leaseMatches(f, l) {
			return ErrCopilotFlowUnavailable
		}
		if code == nil || !flowText(code.DeviceCode, 8192) || !flowText(code.UserCode, 256) || code.VerificationURI != copilotlogin.VerificationURL || code.ExpiresIn <= 0 || code.Interval <= 0 {
			flowTerminal(f, "failed", "authorization_failed", now)
			return saveFlow(ctx, tx, f)
		}
		plain, _ := json.Marshal(copilotChallenge{Version: 1, DeviceCode: code.DeviceCode, UserCode: code.UserCode, VerificationURI: code.VerificationURI})
		blob, err := EncryptSecret(plain)
		if err != nil {
			flowTerminal(f, "failed", "authorization_failed", now)
			return saveFlow(ctx, tx, f)
		}
		f.State, f.ChallengeEncrypted = "pending", blob
		f.ExpiresAt = f.CreatedAt.Add(min(code.ExpiresIn, 15*time.Minute))
		f.IntervalNs = int64(code.Interval)
		f.PollAt = now.Add(code.Interval)
		f.LeaseID, f.LeaseUntil = nil, nil
		f.Revision++
		expireFlow(f, now)
		return saveFlow(ctx, tx, f)
	})
}

func (s *Store) GetCopilotFlow(ctx context.Context, actorID, id uuid.UUID) (CopilotFlowStatus, error) {
	return s.withCopilotFlow(ctx, actorID, id, nil)
}

func (s *Store) CancelCopilotFlow(ctx context.Context, actorID, id uuid.UUID) (CopilotFlowStatus, error) {
	return s.withCopilotFlow(ctx, actorID, id, func(ctx context.Context, tx bun.Tx, f *copilotDeviceFlow, now time.Time) error {
		if !flowLive(f) {
			return nil
		}
		flowTerminal(f, "cancelled", "cancelled", now)
		return saveFlow(ctx, tx, f)
	})
}

func (s *Store) ClaimCopilotFlowPoll(ctx context.Context, actorID, id uuid.UUID) (CopilotFlowStatus, *CopilotFlowLease, error) {
	var lease *CopilotFlowLease
	out, err := s.withCopilotFlow(ctx, actorID, id, func(ctx context.Context, tx bun.Tx, f *copilotDeviceFlow, now time.Time) error {
		if f.State != "pending" || f.LeaseID != nil || now.Before(f.PollAt) {
			return nil
		}
		c, err := decodeChallenge(f.ChallengeEncrypted)
		if err != nil {
			flowTerminal(f, "failed", "authorization_failed", now)
			return saveFlow(ctx, tx, f)
		}
		lid, until := uuid.New(), now.Add(30*time.Second)
		f.LeaseID, f.LeaseUntil = &lid, &until
		f.Revision++
		lease = &CopilotFlowLease{flowID: f.ID, actorID: f.ActorID, leaseID: lid, revision: f.Revision, clientID: f.ClientID, deviceCode: c.DeviceCode, interval: time.Duration(f.IntervalNs)}
		return saveFlow(ctx, tx, f)
	})
	if err != nil {
		return out, nil, err
	}
	return out, lease, nil
}

func (s *Store) FinalizeCopilotFlowPoll(ctx context.Context, l CopilotFlowLease, result CopilotFlowPollResult) (CopilotFlowStatus, error) {
	return s.withCopilotFlow(ctx, l.actorID, l.flowID, func(ctx context.Context, tx bun.Tx, f *copilotDeviceFlow, now time.Time) error {
		if f.State != "pending" || !leaseMatches(f, l) {
			return ErrCopilotFlowUnavailable
		}
		switch result.State {
		case "pending":
			interval := max(time.Duration(f.IntervalNs), result.Interval)
			f.IntervalNs = int64(interval)
			f.PollAt = now.Add(interval)
			f.LeaseID, f.LeaseUntil = nil, nil
			f.Revision++
		case "ready":
			blob, err := EncryptCopilotCredential(result.Credential, now)
			if err != nil || len(blob) > 32768 || result.Credential.ClientID != f.ClientID {
				flowTerminal(f, "failed", "authorization_failed", now)
			} else {
				f.State, f.CredentialEncrypted, f.ChallengeEncrypted = "ready", blob, nil
				f.ExpiresAt = now.Add(10 * time.Minute)
				f.LeaseID, f.LeaseUntil = nil, nil
				f.Revision++
			}
		case "denied":
			flowTerminal(f, "denied", "access_denied", now)
		case "expired":
			flowTerminal(f, "expired", "expired", now)
		default:
			flowTerminal(f, "failed", "authorization_failed", now)
		}
		return saveFlow(ctx, tx, f)
	})
}

type CopilotFlowSnapshot struct{ flow copilotDeviceFlow }

func (s *Store) ReadyCopilotFlow(ctx context.Context, actorID, id uuid.UUID) (CopilotFlowSnapshot, error) {
	var snapshot CopilotFlowSnapshot
	_, err := s.withCopilotFlow(ctx, actorID, id, func(ctx context.Context, tx bun.Tx, f *copilotDeviceFlow, now time.Time) error {
		if f.State != "ready" {
			return ErrCopilotFlowUnavailable
		}
		if _, err := DecryptCopilotCredential(f.CredentialEncrypted, now); err != nil {
			return ErrCopilotFlowUnavailable
		}
		snapshot.flow = *f
		return nil
	})
	return snapshot, err
}

func (snap CopilotFlowSnapshot) AttachCredential(p *DBProvider) error {
	f := &snap.flow
	if f.State != "ready" || p.Type != "github-copilot" || p.WorkspaceID != f.WorkspaceID || (f.Purpose == "edit" && (p.ID != *f.ProviderID || p.UpdatedAt.IsZero())) || (f.Purpose == "create" && p.ID != uuid.Nil) {
		return ErrCopilotFlowInput
	}
	p.CopilotCredentialEncrypted = bytes.Clone(f.CredentialEncrypted)
	p.CopilotCredentialName, p.CopilotCredentialPath, p.APIKeyRefKey, p.APIKeyRefPath = "", "", "", ""
	p.APIKeyEncrypted = nil // pragma: allowlist secret
	return nil
}

func (s *Store) ConsumeCopilotFlow(ctx context.Context, snap CopilotFlowSnapshot, p *DBProvider, models *[]DBProviderModel) error {
	var next DBProvider
	_, err := s.withCopilotFlow(ctx, snap.flow.ActorID, snap.flow.ID, func(ctx context.Context, tx bun.Tx, f *copilotDeviceFlow, now time.Time) error {
		if f.State != "ready" || f.Revision != snap.flow.Revision || !bytes.Equal(f.CredentialEncrypted, p.CopilotCredentialEncrypted) {
			return ErrCopilotFlowUnavailable
		}
		candidate := *p
		if err := (CopilotFlowSnapshot{flow: *f}).AttachCredential(&candidate); err != nil {
			return err
		}
		if p.CopilotCredentialName != "" || p.CopilotCredentialPath != "" || p.APIKeyRefKey != "" || p.APIKeyRefPath != "" || len(p.APIKeyEncrypted) != 0 {
			return ErrCopilotFlowInput
		}
		if _, err := DecryptCopilotCredential(f.CredentialEncrypted, now); err != nil {
			return ErrCopilotFlowUnavailable
		}
		var err error
		if f.Purpose == "edit" {
			var current DBProvider
			if err := tx.NewSelect().Model(&current).Where("id = ? AND workspace_id = ?", *f.ProviderID, f.WorkspaceID).For("UPDATE").Scan(ctx); err != nil {
				return err
			}
			if current.Type != "github-copilot" || current.Name != p.Name || !current.UpdatedAt.Equal(p.UpdatedAt) {
				return ErrCatalogConflict
			}
			next, err = updateProviderAggregateTx(ctx, tx, p, models)
		} else {
			var ms []DBProviderModel
			if models != nil {
				ms = *models
			}
			next, err = createProviderAggregateTx(ctx, tx, p, ms)
		}
		if err != nil {
			return err
		}
		completed, err := flowNow(ctx, tx)
		if err != nil {
			return err
		}
		if !completed.Before(f.ExpiresAt) {
			return ErrCatalogConflict
		}
		flowTerminal(f, "consumed", "", completed)
		f.ConsumedProviderID, f.ConsumedUpdatedAt, f.ProviderName = &next.ID, &next.UpdatedAt, next.Name
		return saveFlow(ctx, tx, f)
	})
	if err != nil {
		return err
	}
	*p = next
	return nil
}

func invalidateCopilotFlows(ctx context.Context, tx bun.Tx, actorID uuid.UUID, workspaceID *uuid.UUID) error {
	q := tx.NewUpdate().Model((*copilotDeviceFlow)(nil)).
		Set("state = 'cancelled'").Set("error_code = 'authority_lost'").
		Set("challenge_encrypted = NULL").Set("credential_encrypted = NULL").
		Set("lease_id = NULL").Set("lease_until = NULL").
		Set("revision = revision + 1").Set("retain_until = clock_timestamp() + interval '1 hour'").
		Where("actor_id = ? AND state IN ('starting','pending','ready')", actorID)
	if workspaceID != nil {
		q = q.Where("workspace_id = ?", *workspaceID)
	}
	_, err := q.Exec(ctx)
	return err
}
