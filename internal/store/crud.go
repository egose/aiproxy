package store

import (
	"context"
	"time"

	"github.com/google/uuid"
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
	p.ID = uuid.New()
	p.CreatedAt = time.Now()
	p.UpdatedAt = p.CreatedAt
	_, err := s.DB.NewInsert().Model(p).Exec(ctx)
	return err
}

func (s *Store) UpdateProvider(ctx context.Context, p *DBProvider) error {
	p.UpdatedAt = time.Now()
	_, err := s.DB.NewUpdate().Model(p).Where("id = ?", p.ID).Exec(ctx)
	return err
}

func (s *Store) DeleteProvider(ctx context.Context, id uuid.UUID) error {
	_, err := s.DB.NewDelete().Model((*DBProvider)(nil)).Where("id = ?", id).Exec(ctx)
	return err
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
	if _, err := tx.NewDelete().Model((*DBProviderModel)(nil)).Where("provider_id = ?", providerID).Exec(ctx); err != nil {
		_ = tx.Rollback()
		return err
	}
	for i := range models {
		models[i].ID = uuid.New()
		models[i].ProviderID = providerID
		if _, err := tx.NewInsert().Model(&models[i]).Exec(ctx); err != nil {
			_ = tx.Rollback()
			return err
		}
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
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	a.ID = uuid.New()
	a.CreatedAt = time.Now()
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
	return tx.Commit()
}

func (s *Store) UpdateAlias(ctx context.Context, a *DBAlias, targets []DBAliasTarget) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	a.UpdatedAt = time.Now()
	if _, err := tx.NewUpdate().Model(a).Where("id = ?", a.ID).Exec(ctx); err != nil {
		_ = tx.Rollback()
		return err
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
	return tx.Commit()
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
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if _, err := tx.NewDelete().Model((*KeyUser)(nil)).Where("key_id = ?", keyID).Exec(ctx); err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err := tx.NewDelete().Model((*KeyTeam)(nil)).Where("key_id = ?", keyID).Exec(ctx); err != nil {
		_ = tx.Rollback()
		return err
	}
	now := time.Now()
	for _, uid := range userIDs {
		if _, err := tx.NewInsert().Model(&KeyUser{KeyID: keyID, UserID: uid, CreatedAt: now}).Exec(ctx); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	for _, tid := range teamIDs {
		if _, err := tx.NewInsert().Model(&KeyTeam{KeyID: keyID, TeamID: tid, CreatedAt: now}).Exec(ctx); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
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
	u.UpdatedAt = time.Now()
	_, err := s.DB.NewUpdate().Model(u).Where("id = ?", u.ID).Exec(ctx)
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

func (s *Store) AcceptInvite(ctx context.Context, inv *Invite, u *User) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	now := time.Now()
	inv.AcceptedAt = &now
	if _, err := tx.NewUpdate().Model(inv).Column("accepted_at").Where("id = ?", inv.ID).Exec(ctx); err != nil {
		_ = tx.Rollback()
		return err
	}
	u.ID = uuid.New()
	u.CreatedAt = now
	u.UpdatedAt = now
	if _, err := tx.NewInsert().Model(u).Exec(ctx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func (s *Store) CountEnabledAdmins(ctx context.Context) (int, error) {
	return s.DB.NewSelect().Model((*User)(nil)).Where("is_admin = true AND disabled = false").Count(ctx)
}

func (s *Store) DeleteUser(ctx context.Context, id uuid.UUID) error {
	_, err := s.DB.NewDelete().Model((*User)(nil)).Where("id = ?", id).Exec(ctx)
	return err
}

var SystemOrgID = uuid.MustParse("00000000-0000-0000-0000-000000000000")

func (s *Store) GetOrganization(ctx context.Context, id uuid.UUID) (Organization, error) {
	var out Organization
	err := s.DB.NewSelect().Model(&out).Where("id = ?", id).Scan(ctx)
	return out, err
}

func (s *Store) GetOrganizationByName(ctx context.Context, name string) (Organization, error) {
	var out Organization
	err := s.DB.NewSelect().Model(&out).Where("name = ?", name).Scan(ctx)
	return out, err
}

func (s *Store) ListOrganizations(ctx context.Context) ([]Organization, error) {
	var out []Organization
	err := s.DB.NewSelect().Model(&out).Order("name ASC").Scan(ctx)
	return out, err
}

func (s *Store) CreateOrganization(ctx context.Context, org *Organization) error {
	org.ID = uuid.New()
	org.CreatedAt = time.Now()
	org.UpdatedAt = org.CreatedAt
	_, err := s.DB.NewInsert().Model(org).Exec(ctx)
	return err
}

func (s *Store) UpdateOrganization(ctx context.Context, org *Organization) error {
	org.UpdatedAt = time.Now()
	_, err := s.DB.NewUpdate().Model(org).Where("id = ?", org.ID).Exec(ctx)
	return err
}

func (s *Store) DeleteOrganization(ctx context.Context, id uuid.UUID) error {
	_, err := s.DB.NewDelete().Model((*Organization)(nil)).Where("id = ?", id).Exec(ctx)
	return err
}

func (s *Store) GetMembership(ctx context.Context, userID, orgID uuid.UUID) (OrganizationMember, error) {
	var out OrganizationMember
	err := s.DB.NewSelect().Model(&out).Where("user_id = ? AND org_id = ?", userID, orgID).Scan(ctx)
	return out, err
}

func (s *Store) ListMembershipsByUser(ctx context.Context, userID uuid.UUID) ([]OrganizationMember, error) {
	var out []OrganizationMember
	err := s.DB.NewSelect().Model(&out).Where("user_id = ?", userID).Scan(ctx)
	return out, err
}

func (s *Store) ListMembershipsByOrg(ctx context.Context, orgID uuid.UUID) ([]OrganizationMember, error) {
	var out []OrganizationMember
	err := s.DB.NewSelect().Model(&out).Where("org_id = ?", orgID).Scan(ctx)
	return out, err
}

func (s *Store) UpsertMembership(ctx context.Context, m *OrganizationMember) error {
	m.CreatedAt = time.Now()
	_, err := s.DB.NewInsert().Model(m).
		On("CONFLICT (user_id, org_id) DO UPDATE").
		Set("role = EXCLUDED.role").
		Exec(ctx)
	return err
}

func (s *Store) DeleteMembership(ctx context.Context, userID, orgID uuid.UUID) error {
	_, err := s.DB.NewDelete().Model((*OrganizationMember)(nil)).Where("user_id = ? AND org_id = ?", userID, orgID).Exec(ctx)
	return err
}

func (s *Store) CountOrgAdmins(ctx context.Context, orgID uuid.UUID) (int, error) {
	return s.DB.NewSelect().Model((*OrganizationMember)(nil)).Where("org_id = ? AND role = 'admin'", orgID).Count(ctx)
}

func (s *Store) CreateTeam(ctx context.Context, t *OrganizationTeam) error {
	t.ID = uuid.New()
	t.CreatedAt = time.Now()
	t.UpdatedAt = t.CreatedAt
	_, err := s.DB.NewInsert().Model(t).Exec(ctx)
	return err
}

func (s *Store) GetTeam(ctx context.Context, id uuid.UUID) (OrganizationTeam, error) {
	var out OrganizationTeam
	err := s.DB.NewSelect().Model(&out).Where("id = ?", id).Scan(ctx)
	return out, err
}

func (s *Store) ListTeamsByOrg(ctx context.Context, orgID uuid.UUID) ([]OrganizationTeam, error) {
	var out []OrganizationTeam
	err := s.DB.NewSelect().Model(&out).Where("org_id = ?", orgID).Order("name ASC").Scan(ctx)
	return out, err
}

func (s *Store) UpdateTeam(ctx context.Context, t *OrganizationTeam) error {
	t.UpdatedAt = time.Now()
	_, err := s.DB.NewUpdate().Model(t).Where("id = ?", t.ID).Exec(ctx)
	return err
}

func (s *Store) DeleteTeam(ctx context.Context, id uuid.UUID) error {
	_, err := s.DB.NewDelete().Model((*OrganizationTeam)(nil)).Where("id = ?", id).Exec(ctx)
	return err
}

func (s *Store) AddTeamMember(ctx context.Context, userID, teamID uuid.UUID, role string) error {
	if role == "" {
		role = "member"
	}
	m := &TeamMember{UserID: userID, TeamID: teamID, Role: role, CreatedAt: time.Now()}
	_, err := s.DB.NewInsert().Model(m).
		On("CONFLICT (user_id, team_id) DO UPDATE SET role = EXCLUDED.role").
		Exec(ctx)
	return err
}

func (s *Store) RemoveTeamMember(ctx context.Context, userID, teamID uuid.UUID) error {
	_, err := s.DB.NewDelete().Model((*TeamMember)(nil)).Where("user_id = ? AND team_id = ?", userID, teamID).Exec(ctx)
	return err
}

func (s *Store) GetTeamMember(ctx context.Context, userID, teamID uuid.UUID) (TeamMember, error) {
	var out TeamMember
	err := s.DB.NewSelect().Model(&out).Where("user_id = ? AND team_id = ?", userID, teamID).Scan(ctx)
	return out, err
}

func (s *Store) SetTeamMemberRole(ctx context.Context, userID, teamID uuid.UUID, role string) error {
	_, err := s.DB.NewUpdate().Model((*TeamMember)(nil)).Set("role = ?", role).Where("user_id = ? AND team_id = ?", userID, teamID).Exec(ctx)
	return err
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

func (s *Store) ListTeamsByUser(ctx context.Context, userID uuid.UUID) ([]OrganizationTeam, error) {
	var out []OrganizationTeam
	err := s.DB.NewSelect().Model(&out).
		Join("JOIN team_members tm ON tm.team_id = ot.id").
		Where("tm.user_id = ?", userID).
		Order("ot.name ASC").
		Scan(ctx)
	return out, err
}

func (s *Store) CountProvidersByOrg(ctx context.Context, orgID uuid.UUID) (int, error) {
	return s.DB.NewSelect().Model((*DBProvider)(nil)).Where("org_id = ?", orgID).Count(ctx)
}

func (s *Store) CountAliasesByOrg(ctx context.Context, orgID uuid.UUID) (int, error) {
	return s.DB.NewSelect().Model((*DBAlias)(nil)).Where("org_id = ?", orgID).Count(ctx)
}

func (s *Store) CountKeysByOrg(ctx context.Context, orgID uuid.UUID) (int, error) {
	return s.DB.NewSelect().Model((*InboundKey)(nil)).Where("org_id = ?", orgID).Count(ctx)
}

func (s *Store) CreateUserWithOrg(ctx context.Context, u *User, org *Organization, role string) error {
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
	org.ID = uuid.New()
	org.CreatedAt = now
	org.UpdatedAt = now
	if _, err := tx.NewInsert().Model(org).Exec(ctx); err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err := tx.NewInsert().Model(&OrganizationMember{UserID: u.ID, OrgID: org.ID, Role: role, CreatedAt: now}).Exec(ctx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func (s *Store) MembershipsWithOrg(ctx context.Context, userID uuid.UUID) ([]Organization, error) {
	var out []Organization
	err := s.DB.NewSelect().Model((*Organization)(nil)).
		Join("JOIN organization_members om ON om.org_id = organization.id").
		Where("om.user_id = ?", userID).
		Order("organization.name ASC").
		Scan(ctx, &out)
	return out, err
}

func (s *Store) UpsertScopeQuota(ctx context.Context, q *ScopeQuota) error {
	q.UpdatedAt = time.Now()
	if q.CreatedAt.IsZero() {
		q.CreatedAt = q.UpdatedAt
	}
	_, err := s.DB.NewInsert().Model(q).
		On("CONFLICT (org_id, scope_type, scope_id, model) DO UPDATE SET budget_micros = EXCLUDED.budget_micros, tpm_ceiling = EXCLUDED.tpm_ceiling, tpm_effective = EXCLUDED.tpm_effective, spent_offset_micros = EXCLUDED.spent_offset_micros, updated_at = EXCLUDED.updated_at").
		Exec(ctx)
	return err
}

func (s *Store) GetScopeQuota(ctx context.Context, orgID uuid.UUID, scopeType string, scopeID uuid.UUID, model string) (ScopeQuota, error) {
	var out ScopeQuota
	err := s.DB.NewSelect().Model(&out).
		Where("org_id = ? AND scope_type = ? AND scope_id = ? AND model = ?", orgID, scopeType, scopeID, model).
		Scan(ctx)
	return out, err
}

func (s *Store) ListScopeQuotasByScope(ctx context.Context, orgID uuid.UUID, scopeType string, scopeID uuid.UUID) ([]ScopeQuota, error) {
	var out []ScopeQuota
	err := s.DB.NewSelect().Model(&out).
		Where("org_id = ? AND scope_type = ? AND scope_id = ?", orgID, scopeType, scopeID).
		Order("model ASC").Scan(ctx)
	return out, err
}

func (s *Store) InsertSpendEntry(ctx context.Context, e *SpendEntry) error {
	e.ID = uuid.New()
	e.CreatedAt = time.Now()
	_, err := s.DB.NewInsert().Model(e).Exec(ctx)
	return err
}

func (s *Store) SumScopeSpend(ctx context.Context, orgID uuid.UUID, scopeType string, scopeID uuid.UUID) (int64, error) {
	var sum int64
	col := "user_id"
	if scopeType == "team" {
		col = "team_id"
	}
	err := s.DB.NewSelect().Model((*SpendEntry)(nil)).
		ColumnExpr("COALESCE(SUM(cost_micros), 0)").
		Where("org_id = ? AND "+col+" = ?", orgID, scopeID).
		Scan(ctx, &sum)
	return sum, err
}

func (s *Store) KeyUsageByOrg(ctx context.Context, orgID uuid.UUID) ([]KeyUsage, error) {
	var out []KeyUsage
	err := s.DB.NewSelect().Model((*SpendEntry)(nil)).
		Column("key_id").
		ColumnExpr("COUNT(*) AS requests").
		ColumnExpr("COALESCE(SUM(tokens), 0) AS tokens").
		ColumnExpr("COALESCE(SUM(cost_micros), 0) AS spend").
		Where("org_id = ?", orgID).
		Group("key_id").
		Scan(ctx, &out)
	return out, err
}
