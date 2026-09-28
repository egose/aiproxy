package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/driver/pgdriver"
)

func (s *Store) ListProviders(ctx context.Context) ([]DBProvider, error) {
	var out []DBProvider
	err := s.DB.NewSelect().Model(&out).Order("name ASC").Scan(ctx)
	return out, err
}

func (s *Store) GetProvider(ctx context.Context, name string) (DBProvider, error) {
	var p DBProvider
	err := s.DB.NewSelect().Model(&p).Where("name = ?", name).Scan(ctx)
	return p, err
}

func (s *Store) CreateProvider(ctx context.Context, p *DBProvider) error {
	return s.CreateProviderAggregate(ctx, p, nil)
}

func (s *Store) UpdateProvider(ctx context.Context, p *DBProvider) error {
	return s.UpdateProviderAggregate(ctx, p, nil)
}

func (s *Store) DeleteProvider(ctx context.Context, id uuid.UUID) error {
	return s.DB.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		var p DBProvider
		if err := tx.NewSelect().Model(&p).Where("id = ?", id).Scan(ctx); err != nil {
			return err
		}
		if err := lockMembershipWorkspace(ctx, tx, p.WorkspaceID); err != nil {
			return err
		}
		if _, err := tx.NewDelete().Model((*copilotDeviceFlow)(nil)).Where("provider_id = ?", id).Exec(ctx); err != nil {
			return err
		}
		_, err := tx.NewDelete().Model((*DBProvider)(nil)).Where("id = ?", id).Exec(ctx)
		return err
	})
}

func (s *Store) ListProviderModels(ctx context.Context, providerID uuid.UUID) ([]DBProviderModel, error) {
	var out []DBProviderModel
	err := s.DB.NewSelect().Model(&out).Where("provider_id = ?", providerID).Order("name ASC").Scan(ctx)
	return out, err
}

func (s *Store) ReplaceProviderModels(ctx context.Context, providerID uuid.UUID, models []DBProviderModel) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.NewUpdate().Model((*DBProvider)(nil)).Set("updated_at = GREATEST(clock_timestamp(), updated_at + interval '1 microsecond')").Where("id = ?", providerID).Exec(ctx)
	if err != nil {
		return err
	}
	if n, err := result.RowsAffected(); err != nil {
		return err
	} else if n != 1 {
		return ErrCatalogConflict
	}
	if err := replaceProviderModels(ctx, tx, providerID, models); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ListAliases(ctx context.Context) ([]DBAlias, error) {
	var out []DBAlias
	err := s.DB.NewSelect().Model(&out).Order("name ASC").Scan(ctx)
	return out, err
}

func (s *Store) GetAlias(ctx context.Context, name string) (DBAlias, error) {
	var a DBAlias
	err := s.DB.NewSelect().Model(&a).Where("name = ?", name).Scan(ctx)
	return a, err
}

func (s *Store) CreateAlias(ctx context.Context, a *DBAlias, targets []DBAliasTarget) error {
	output := a
	copy := *a
	a = &copy
	targets = append([]DBAliasTarget(nil), targets...)
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	a.ID = uuid.New()
	a.CreatedAt = time.Now().UTC().Truncate(time.Microsecond)
	a.UpdatedAt = a.CreatedAt
	if _, err := tx.NewInsert().Model(a).Exec(ctx); err != nil {
		_ = tx.Rollback()
		return err
	}
	for i := range targets {
		targets[i].ID = uuid.New()
		targets[i].AliasID = a.ID
		if _, err := tx.NewInsert().Model(&targets[i]).Exec(ctx); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	*output = *a
	return nil
}

func (s *Store) UpdateAlias(ctx context.Context, a *DBAlias, targets []DBAliasTarget) error {
	output := a
	copy := *a
	a = &copy
	targets = append([]DBAliasTarget(nil), targets...)
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	previous := a.UpdatedAt
	a.UpdatedAt = nextCatalogRevision(previous)
	result, err := tx.NewUpdate().Model(a).Where("id = ? AND updated_at = ?", a.ID, previous).Exec(ctx)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	if n, err := result.RowsAffected(); err != nil {
		return err
	} else if n != 1 {
		return ErrCatalogConflict
	}
	if _, err := tx.NewDelete().Model((*DBAliasTarget)(nil)).Where("alias_id = ?", a.ID).Exec(ctx); err != nil {
		_ = tx.Rollback()
		return err
	}
	for i := range targets {
		targets[i].ID = uuid.New()
		targets[i].AliasID = a.ID
		if _, err := tx.NewInsert().Model(&targets[i]).Exec(ctx); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	*output = *a
	return nil
}

func (s *Store) DeleteAlias(ctx context.Context, id uuid.UUID) error {
	_, err := s.DB.NewDelete().Model((*DBAlias)(nil)).Where("id = ?", id).Exec(ctx)
	return err
}

func (s *Store) ListAliasTargets(ctx context.Context, aliasID uuid.UUID) ([]DBAliasTarget, error) {
	var out []DBAliasTarget
	err := s.DB.NewSelect().Model(&out).Where("alias_id = ?", aliasID).Order("provider ASC").Order("model ASC").Scan(ctx)
	return out, err
}

func (s *Store) ListInboundKeys(ctx context.Context) ([]InboundKey, error) {
	var out []InboundKey
	err := s.DB.NewSelect().Model(&out).Order("name ASC").Scan(ctx)
	return out, err
}

func (s *Store) CreateInboundKey(ctx context.Context, k *InboundKey) error {
	k.ID = uuid.New()
	k.CreatedAt = time.Now()
	k.UpdatedAt = k.CreatedAt
	_, err := s.DB.NewInsert().Model(k).Exec(ctx)
	return err
}

func (s *Store) UpdateInboundKey(ctx context.Context, k *InboundKey) error {
	k.UpdatedAt = time.Now()
	_, err := s.DB.NewUpdate().Model(k).Where("id = ?", k.ID).Exec(ctx)
	return err
}

func (s *Store) RotateInboundKey(ctx context.Context, id uuid.UUID, hash, prefix string) error {
	_, err := s.DB.NewUpdate().Model((*InboundKey)(nil)).Set("token_hash = ?", hash).
		Set("token_prefix = ?", prefix).Set("enabled = TRUE").Set("updated_at = ?", time.Now()).Where("id = ?", id).Exec(ctx)
	return err
}

func (s *Store) SetInboundKeyEnabled(ctx context.Context, id uuid.UUID, enabled bool) error {
	_, err := s.DB.NewUpdate().Model((*InboundKey)(nil)).Set("enabled = ?", enabled).
		Set("updated_at = ?", time.Now()).Where("id = ?", id).Exec(ctx)
	return err
}

func (s *Store) DeleteInboundKey(ctx context.Context, id uuid.UUID) error {
	_, err := s.DB.NewDelete().Model((*InboundKey)(nil)).Where("id = ?", id).Exec(ctx)
	return err
}

func (s *Store) ListKeyUserIDs(ctx context.Context, keyID uuid.UUID) ([]uuid.UUID, error) {
	var rows []KeyUser
	err := s.DB.NewSelect().Model(&rows).Where("key_id = ?", keyID).Scan(ctx)
	out := make([]uuid.UUID, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.UserID)
	}
	return out, err
}

func (s *Store) ListKeyTeamIDs(ctx context.Context, keyID uuid.UUID) ([]uuid.UUID, error) {
	var rows []KeyTeam
	err := s.DB.NewSelect().Model(&rows).Where("key_id = ?", keyID).Scan(ctx)
	out := make([]uuid.UUID, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.TeamID)
	}
	return out, err
}

func (s *Store) SetKeyBindings(ctx context.Context, keyID uuid.UUID, userIDs, teamIDs []uuid.UUID) error {
	return s.updateKeyPolicy(ctx, keyID, nil, InboundKeyPolicyPatch{ReplaceBindings: true, UserIDs: userIDs, TeamIDs: teamIDs})
}

func (s *Store) ListInboundKeysVisibleToMember(ctx context.Context, userID uuid.UUID) ([]InboundKey, error) {
	var out []InboundKey
	err := s.DB.NewSelect().Model(&out).Distinct().
		Join("LEFT JOIN key_users ku ON ku.key_id = k.id AND ku.user_id = ?", userID).
		Join("LEFT JOIN key_teams kt ON kt.key_id = k.id").
		Join("LEFT JOIN team_members tm ON tm.team_id = kt.team_id AND tm.user_id = ?", userID).
		Where("ku.user_id IS NOT NULL OR tm.user_id IS NOT NULL").
		Order("name ASC").
		Scan(ctx)
	return out, err
}

func (s *Store) GetUserByEmail(ctx context.Context, email string) (User, error) {
	var u User
	err := s.DB.NewSelect().Model(&u).Where("email = ?", email).Scan(ctx)
	return u, err
}

func (s *Store) GetUserByID(ctx context.Context, id uuid.UUID) (User, error) {
	var u User
	err := s.DB.NewSelect().Model(&u).Where("id = ?", id).Scan(ctx)
	return u, err
}

func (s *Store) ListUsers(ctx context.Context) ([]User, error) {
	var out []User
	err := s.DB.NewSelect().Model(&out).Order("email ASC").Scan(ctx)
	return out, err
}

func (s *Store) CreateUser(ctx context.Context, u *User) error {
	u.ID = uuid.New()
	u.CreatedAt = time.Now()
	u.UpdatedAt = u.CreatedAt
	_, err := s.DB.NewInsert().Model(u).Exec(ctx)
	return err
}

func (s *Store) UpdateUser(ctx context.Context, u *User) error {
	next := *u
	next.UpdatedAt = time.Now()
	err := s.DB.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		var current User
		if err := tx.NewSelect().Model(&current).Where("id = ?", u.ID).For("UPDATE").Scan(ctx); err != nil {
			return err
		}
		if (!current.Disabled && next.Disabled) || (current.IsAdmin && !next.IsAdmin) {
			var workspaceIDs []uuid.UUID
			if err := tx.NewSelect().Model((*copilotDeviceFlow)(nil)).Column("workspace_id").Distinct().Where("actor_id = ? AND state IN ('starting','pending','ready')", u.ID).Order("workspace_id").Scan(ctx, &workspaceIDs); err != nil {
				return err
			}
			for _, workspaceID := range workspaceIDs {
				if err := lockMembershipWorkspace(ctx, tx, workspaceID); err != nil {
					return err
				}
			}
			if err := invalidateCopilotFlows(ctx, tx, u.ID, nil); err != nil {
				return err
			}
		}
		_, err := tx.NewUpdate().Model(&next).Where("id = ?", u.ID).Exec(ctx)
		return err
	})
	if err == nil {
		*u = next
	}
	return err
}

func (s *Store) CreateRefreshToken(ctx context.Context, rt *RefreshToken) error {
	rt.ID = uuid.New()
	rt.CreatedAt = time.Now()
	_, err := s.DB.NewInsert().Model(rt).Exec(ctx)
	return err
}

func (s *Store) GetRefreshToken(ctx context.Context, hash string) (RefreshToken, error) {
	var rt RefreshToken
	err := s.DB.NewSelect().Model(&rt).Where("token_hash = ?", hash).Scan(ctx)
	return rt, err
}

func (s *Store) RevokeRefreshToken(ctx context.Context, id uuid.UUID, at time.Time) error {
	_, err := s.DB.NewUpdate().Model((*RefreshToken)(nil)).Set("revoked_at = ?", at).Where("id = ?", id).Exec(ctx)
	return err
}

func (s *Store) GetOIDCConfig(ctx context.Context) (OIDCConfig, error) {
	var cfg OIDCConfig
	err := s.DB.NewSelect().Model(&cfg).Where("id = 'default'").Scan(ctx)
	return cfg, err
}

func (s *Store) UpsertOIDCConfig(ctx context.Context, cfg *OIDCConfig) error {
	cfg.ID = "default"
	cfg.UpdatedAt = time.Now()
	_, err := s.DB.NewInsert().Model(cfg).
		On("CONFLICT (id) DO UPDATE").
		Set("enabled = EXCLUDED.enabled").
		Set("issuer_url = EXCLUDED.issuer_url").
		Set("client_id = EXCLUDED.client_id").
		Set("client_secret_encrypted = EXCLUDED.client_secret_encrypted"). // pragma: allowlist secret
		Set("scopes = EXCLUDED.scopes").
		Set("username_claim = EXCLUDED.username_claim").
		Set("admin_claim = EXCLUDED.admin_claim").
		Set("admin_value = EXCLUDED.admin_value").
		Set("display_name = EXCLUDED.display_name").
		Set("updated_at = EXCLUDED.updated_at").
		Exec(ctx)
	return err
}

func (s *Store) CreateInvite(ctx context.Context, inv *Invite) error {
	inv.ID = uuid.New()
	inv.CreatedAt = time.Now()
	_, err := s.DB.NewInsert().Model(inv).Exec(ctx)
	return err
}

func (s *Store) ListInvites(ctx context.Context) ([]Invite, error) {
	var out []Invite
	err := s.DB.NewSelect().Model(&out).Order("created_at ASC").Scan(ctx)
	return out, err
}

func (s *Store) GetInviteByHash(ctx context.Context, hash string) (Invite, error) {
	var inv Invite
	err := s.DB.NewSelect().Model(&inv).Where("token_hash = ?", hash).Scan(ctx)
	return inv, err
}

func (s *Store) DeleteInvite(ctx context.Context, id uuid.UUID) error {
	_, err := s.DB.NewDelete().Model((*Invite)(nil)).Where("id = ?", id).Exec(ctx)
	return err
}

var ErrInviteUnavailable = errors.New("invite is expired or already accepted")
var ErrInviteEmailRegistered = errors.New("email is already registered")
var ErrInviteChanged = errors.New("invite changed during acceptance; retry with the current invitation")

func (s *Store) AcceptInvite(ctx context.Context, inv *Invite, u *User) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var scope Invite
	if err := tx.NewSelect().Model(&scope).
		Where("id = ? AND token_hash = ?", inv.ID, inv.TokenHash).Scan(ctx); err != nil {
		return err
	}
	if scope.WorkspaceID != nil {
		var workspace Workspace
		if err := tx.NewSelect().Model(&workspace).Where("id = ?", *scope.WorkspaceID).For("KEY SHARE").Scan(ctx); err != nil {
			return err
		}
	}
	var current Invite
	if err := tx.NewSelect().Model(&current).
		Where("id = ? AND token_hash = ?", inv.ID, inv.TokenHash).
		For("UPDATE").Scan(ctx); err != nil {
		return err
	}
	if (scope.WorkspaceID == nil) != (current.WorkspaceID == nil) || (scope.WorkspaceID != nil && current.WorkspaceID != nil && *scope.WorkspaceID != *current.WorkspaceID) {
		return ErrInviteChanged
	}
	result, err := tx.NewUpdate().Model(&current).
		Set("accepted_at = clock_timestamp()").
		Where("id = ? AND accepted_at IS NULL AND expires_at > clock_timestamp()", current.ID).
		Returning("accepted_at").Exec(ctx)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return ErrInviteUnavailable
	}
	now := time.Now()
	created := User{ID: uuid.New(), Email: current.Email, PasswordHash: u.PasswordHash, IsAdmin: current.IsAdmin, CreatedAt: now, UpdatedAt: now}
	if _, err := tx.NewInsert().Model(&created).Exec(ctx); err != nil {
		var pgErr pgdriver.Error
		if errors.As(err, &pgErr) && pgErr.Field('C') == "23505" && pgErr.Field('n') == "users_email_key" {
			return ErrInviteEmailRegistered
		}
		return err
	}
	if current.WorkspaceID != nil {
		role := current.WorkspaceRole
		if role != "admin" && role != "member" {
			role = "member"
		}
		membership := WorkspaceMember{UserID: created.ID, WorkspaceID: *current.WorkspaceID, Role: role, CreatedAt: now}
		if err := insertMembership(ctx, tx, &membership); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	*inv = current
	*u = created
	return nil
}

func (s *Store) CountEnabledAdmins(ctx context.Context) (int, error) {
	return s.DB.NewSelect().Model((*User)(nil)).Where("is_admin = true AND disabled = false").Count(ctx)
}

var SystemWorkspaceID = uuid.MustParse("00000000-0000-0000-0000-000000000000")

func (s *Store) GetWorkspace(ctx context.Context, id uuid.UUID) (Workspace, error) {
	var out Workspace
	err := s.DB.NewSelect().Model(&out).Where("id = ?", id).Scan(ctx)
	return out, err
}

func (s *Store) GetWorkspaceByName(ctx context.Context, name string) (Workspace, error) {
	var out Workspace
	err := s.DB.NewSelect().Model(&out).Where("name = ?", name).Scan(ctx)
	return out, err
}

func (s *Store) ListWorkspaces(ctx context.Context) ([]Workspace, error) {
	var out []Workspace
	err := s.DB.NewSelect().Model(&out).Order("name ASC").Scan(ctx)
	return out, err
}

func (s *Store) CreateWorkspace(ctx context.Context, workspace *Workspace) error {
	kind, err := normalizeWorkspaceKind(workspace.Kind)
	if err != nil {
		return err
	}
	workspace.Kind = kind
	workspace.ID = uuid.New()
	workspace.CreatedAt = time.Now()
	workspace.UpdatedAt = workspace.CreatedAt
	_, err = s.DB.NewInsert().Model(workspace).Exec(ctx)
	return err
}

func (s *Store) CreatePersonalWorkspace(ctx context.Context, userID uuid.UUID, workspace *Workspace) error {
	return s.DB.RunInTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted}, func(ctx context.Context, tx bun.Tx) error {
		if err := lockMembershipUser(ctx, tx, userID); err != nil {
			return err
		}
		workspace.Kind = WorkspaceKindPersonal
		existing, err := personalWorkspaceForUserTx(ctx, tx, userID)
		if err != nil {
			return err
		}
		if existing != uuid.Nil {
			return ErrPersonalWorkspaceExists
		}
		now := time.Now()
		workspace.ID = uuid.New()
		workspace.CreatedAt = now
		workspace.UpdatedAt = now
		if _, err := tx.NewInsert().Model(workspace).Exec(ctx); err != nil {
			return err
		}
		return insertMembership(ctx, tx, &WorkspaceMember{UserID: userID, WorkspaceID: workspace.ID, Role: "admin", CreatedAt: now})
	})
}

func (s *Store) UpdateWorkspace(ctx context.Context, workspace *Workspace) error {
	kind, err := normalizeWorkspaceKind(workspace.Kind)
	if err != nil {
		return err
	}
	workspace.Kind = kind
	workspace.UpdatedAt = time.Now()
	_, err = s.DB.NewUpdate().Model(workspace).Where("id = ?", workspace.ID).Exec(ctx)
	return err
}

func (s *Store) DeleteWorkspace(ctx context.Context, id uuid.UUID) error {
	_, err := s.DB.NewDelete().Model((*Workspace)(nil)).Where("id = ?", id).Exec(ctx)
	return err
}

func (s *Store) GetMembership(ctx context.Context, userID, workspaceID uuid.UUID) (WorkspaceMember, error) {
	var out WorkspaceMember
	err := s.DB.NewSelect().Model(&out).Where("user_id = ? AND workspace_id = ?", userID, workspaceID).Scan(ctx)
	return out, err
}

func (s *Store) ListMembershipsByUser(ctx context.Context, userID uuid.UUID) ([]WorkspaceMember, error) {
	var out []WorkspaceMember
	err := s.DB.NewSelect().Model(&out).Where("user_id = ?", userID).Scan(ctx)
	return out, err
}

func (s *Store) ListMembershipsByWorkspace(ctx context.Context, workspaceID uuid.UUID) ([]WorkspaceMember, error) {
	var out []WorkspaceMember
	err := s.DB.NewSelect().Model(&out).Where("workspace_id = ?", workspaceID).Scan(ctx)
	return out, err
}

func (s *Store) CountWorkspaceAdmins(ctx context.Context, workspaceID uuid.UUID) (int, error) {
	return s.DB.NewSelect().Model((*WorkspaceMember)(nil)).Where("workspace_id = ? AND role = 'admin'", workspaceID).Count(ctx)
}

func (s *Store) CreateTeam(ctx context.Context, t *WorkspaceTeam) error {
	var kind string
	if err := s.DB.NewSelect().Model((*Workspace)(nil)).Column("kind").Where("id = ?", t.WorkspaceID).Scan(ctx, &kind); err != nil {
		return err
	}
	if kind == WorkspaceKindPersonal {
		return ErrPersonalWorkspaceTeams
	}
	t.ID = uuid.New()
	t.CreatedAt = time.Now()
	t.UpdatedAt = t.CreatedAt
	_, err := s.DB.NewInsert().Model(t).Exec(ctx)
	return err
}

func (s *Store) GetTeam(ctx context.Context, id uuid.UUID) (WorkspaceTeam, error) {
	var out WorkspaceTeam
	err := s.DB.NewSelect().Model(&out).Where("id = ?", id).Scan(ctx)
	return out, err
}

func (s *Store) ListTeamsByWorkspace(ctx context.Context, workspaceID uuid.UUID) ([]WorkspaceTeam, error) {
	var out []WorkspaceTeam
	err := s.DB.NewSelect().Model(&out).Where("workspace_id = ?", workspaceID).Order("name ASC").Scan(ctx)
	return out, err
}

func (s *Store) UpdateTeam(ctx context.Context, t *WorkspaceTeam) error {
	t.UpdatedAt = time.Now()
	_, err := s.DB.NewUpdate().Model(t).Where("id = ?", t.ID).Exec(ctx)
	return err
}

func (s *Store) DeleteTeam(ctx context.Context, id uuid.UUID) error {
	_, err := s.DB.NewDelete().Model((*WorkspaceTeam)(nil)).Where("id = ?", id).Exec(ctx)
	return err
}

func (s *Store) GetTeamMember(ctx context.Context, userID, teamID uuid.UUID) (TeamMember, error) {
	var out TeamMember
	err := s.DB.NewSelect().Model(&out).Where("user_id = ? AND team_id = ?", userID, teamID).Scan(ctx)
	return out, err
}

func (s *Store) CountTeamAdmins(ctx context.Context, teamID uuid.UUID) (int, error) {
	return s.DB.NewSelect().Model((*TeamMember)(nil)).Where("team_id = ? AND role = 'admin'", teamID).Count(ctx)
}

func (s *Store) ListTeamAdminTeamIDs(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error) {
	var rows []TeamMember
	err := s.DB.NewSelect().Model(&rows).Where("user_id = ? AND role = 'admin'", userID).Scan(ctx)
	out := make([]uuid.UUID, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.TeamID)
	}
	return out, err
}

func (s *Store) CountKeysByTeam(ctx context.Context, teamID uuid.UUID) (int, error) {
	return s.DB.NewSelect().Model((*InboundKey)(nil)).Where("owner_team_id = ?", teamID).Count(ctx)
}

func (s *Store) ListTeamMembers(ctx context.Context, teamID uuid.UUID) ([]TeamMember, error) {
	var out []TeamMember
	err := s.DB.NewSelect().Model(&out).Where("team_id = ?", teamID).Scan(ctx)
	return out, err
}

func (s *Store) ListTeamMembershipsByUser(ctx context.Context, userID uuid.UUID) ([]TeamMember, error) {
	var out []TeamMember
	err := s.DB.NewSelect().Model(&out).Where("user_id = ?", userID).Scan(ctx)
	return out, err
}

func (s *Store) ListTeamsByUser(ctx context.Context, userID uuid.UUID) ([]WorkspaceTeam, error) {
	var out []WorkspaceTeam
	err := s.DB.NewSelect().Model(&out).
		Join("JOIN team_members tm ON tm.team_id = wt.id").
		Where("tm.user_id = ?", userID).
		Order("wt.name ASC").
		Scan(ctx)
	return out, err
}

func (s *Store) CountProvidersByWorkspace(ctx context.Context, workspaceID uuid.UUID) (int, error) {
	return s.DB.NewSelect().Model((*DBProvider)(nil)).Where("workspace_id = ?", workspaceID).Count(ctx)
}

func (s *Store) CountAliasesByWorkspace(ctx context.Context, workspaceID uuid.UUID) (int, error) {
	return s.DB.NewSelect().Model((*DBAlias)(nil)).Where("workspace_id = ?", workspaceID).Count(ctx)
}

func (s *Store) CountKeysByWorkspace(ctx context.Context, workspaceID uuid.UUID) (int, error) {
	return s.DB.NewSelect().Model((*InboundKey)(nil)).Where("workspace_id = ?", workspaceID).Count(ctx)
}

func (s *Store) CreateUserWithWorkspace(ctx context.Context, u *User, workspace *Workspace, role string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	now := time.Now()
	u.ID = uuid.New()
	u.CreatedAt = now
	u.UpdatedAt = now
	if _, err := tx.NewInsert().Model(u).Exec(ctx); err != nil {
		_ = tx.Rollback()
		return err
	}
	workspace.Kind = WorkspaceKindPersonal
	workspace.ID = uuid.New()
	workspace.CreatedAt = now
	workspace.UpdatedAt = now
	if _, err := tx.NewInsert().Model(workspace).Exec(ctx); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := insertMembership(ctx, tx, &WorkspaceMember{UserID: u.ID, WorkspaceID: workspace.ID, Role: role, CreatedAt: now}); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func (s *Store) MembershipsWithWorkspace(ctx context.Context, userID uuid.UUID) ([]Workspace, error) {
	var out []Workspace
	err := s.DB.NewSelect().Model((*Workspace)(nil)).
		Join("JOIN workspace_members wm ON wm.workspace_id = workspace.id").
		Where("wm.user_id = ?", userID).
		Order("workspace.name ASC").
		Scan(ctx, &out)
	return out, err
}

func (s *Store) UpsertScopeQuota(ctx context.Context, q *ScopeQuota) error {
	q.UpdatedAt = time.Now()
	if q.CreatedAt.IsZero() {
		q.CreatedAt = q.UpdatedAt
	}
	_, err := s.DB.NewInsert().Model(q).
		On("CONFLICT (workspace_id, scope_type, scope_id, model) DO UPDATE SET budget_micros = EXCLUDED.budget_micros, tpm_ceiling = EXCLUDED.tpm_ceiling, tpm_effective = EXCLUDED.tpm_effective, spent_offset_micros = EXCLUDED.spent_offset_micros, updated_at = EXCLUDED.updated_at").
		Exec(ctx)
	return err
}

func (s *Store) GetScopeQuota(ctx context.Context, workspaceID uuid.UUID, scopeType string, scopeID uuid.UUID, model string) (ScopeQuota, error) {
	var out ScopeQuota
	err := s.DB.NewSelect().Model(&out).
		Where("workspace_id = ? AND scope_type = ? AND scope_id = ? AND model = ?", workspaceID, scopeType, scopeID, model).
		Scan(ctx)
	return out, err
}

func (s *Store) ListScopeQuotasByScope(ctx context.Context, workspaceID uuid.UUID, scopeType string, scopeID uuid.UUID) ([]ScopeQuota, error) {
	var out []ScopeQuota
	err := s.DB.NewSelect().Model(&out).
		Where("workspace_id = ? AND scope_type = ? AND scope_id = ?", workspaceID, scopeType, scopeID).
		Order("model ASC").Scan(ctx)
	return out, err
}

func (s *Store) InsertSpendEntry(ctx context.Context, e *SpendEntry) error {
	e.ID = uuid.New()
	e.CreatedAt = time.Now()
	_, err := s.DB.NewInsert().Model(e).Exec(ctx)
	return err
}

func (s *Store) SumScopeSpend(ctx context.Context, workspaceID uuid.UUID, scopeType string, scopeID uuid.UUID) (int64, error) {
	var sum int64
	col := "user_id"
	if scopeType == "team" {
		col = "team_id"
	}
	err := s.DB.NewSelect().Model((*SpendEntry)(nil)).
		ColumnExpr("COALESCE(SUM(cost_micros), 0)").
		Where("workspace_id = ? AND "+col+" = ?", workspaceID, scopeID).
		Scan(ctx, &sum)
	return sum, err
}

func (s *Store) KeyUsageByWorkspace(ctx context.Context, workspaceID uuid.UUID) ([]KeyUsage, error) {
	var out []KeyUsage
	err := s.DB.NewSelect().Model((*SpendEntry)(nil)).
		Column("key_id").
		ColumnExpr("COUNT(*) AS requests").
		ColumnExpr("COALESCE(SUM(tokens), 0) AS tokens").
		ColumnExpr("COALESCE(SUM(cost_micros), 0) AS spend").
		Where("workspace_id = ?", workspaceID).
		Group("key_id").
		Scan(ctx, &out)
	return out, err
}
