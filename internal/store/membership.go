package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

var (
	ErrMembershipExists   = errors.New("membership already exists; use the member's role control (PUT) to change their role")
	ErrLastWorkspaceAdmin = errors.New("cannot remove or demote the last workspace admin; assign another workspace admin first")
	ErrLastTeamAdmin      = errors.New("cannot remove or demote the last team admin; assign another team admin first")
	ErrSelfDemotion       = errors.New("cannot demote your own workspace admin role; ask another workspace admin to edit your role")
	ErrInvalidMemberRole  = errors.New(`role must be "admin" or "member"`)
	ErrNotWorkspaceMember = errors.New("user is not a workspace member; add them to the workspace first")
)

func normalizeWorkspaceKind(kind string) (string, error) {
	if kind == "" {
		return WorkspaceKindOrganization, nil
	}
	if kind != WorkspaceKindPersonal && kind != WorkspaceKindOrganization {
		return "", ErrInvalidWorkspaceKind
	}
	return kind, nil
}

func lockMembershipWorkspace(ctx context.Context, tx bun.Tx, workspaceID uuid.UUID) error {
	var workspace Workspace
	return tx.NewSelect().Model(&workspace).Where("id = ?", workspaceID).For("NO KEY UPDATE").Scan(ctx)
}

func workspaceKindTx(ctx context.Context, tx bun.Tx, workspaceID uuid.UUID) (string, error) {
	var kind string
	if err := tx.NewSelect().Model((*Workspace)(nil)).Column("kind").Where("id = ?", workspaceID).Scan(ctx, &kind); err != nil {
		return "", err
	}
	return kind, nil
}

func personalWorkspaceForUserTx(ctx context.Context, tx bun.Tx, userID uuid.UUID) (uuid.UUID, error) {
	var id uuid.UUID
	err := tx.NewSelect().Model((*Workspace)(nil)).
		Column("w.id").
		Join("JOIN workspace_members wm ON wm.workspace_id = w.id").
		Where("wm.user_id = ? AND w.kind = ?", userID, WorkspaceKindPersonal).
		Scan(ctx, &id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return uuid.Nil, nil
		}
		return uuid.Nil, err
	}
	return id, nil
}

func lockMembershipUser(ctx context.Context, tx bun.Tx, userID uuid.UUID) error {
	var user User
	return tx.NewSelect().Model(&user).Where("id = ?", userID).For("SHARE").Scan(ctx)
}

func insertMembership(ctx context.Context, tx bun.Tx, m *WorkspaceMember) error {
	if m.Role == "" {
		m.Role = "member"
	}
	if m.Role != "admin" && m.Role != "member" {
		return ErrInvalidMemberRole
	}
	if err := lockMembershipUser(ctx, tx, m.UserID); err != nil {
		return err
	}
	if err := lockMembershipWorkspace(ctx, tx, m.WorkspaceID); err != nil {
		return err
	}
	kind, err := workspaceKindTx(ctx, tx, m.WorkspaceID)
	if err != nil {
		return err
	}
	if kind == WorkspaceKindPersonal {
		var existing []WorkspaceMember
		if err := tx.NewSelect().Model(&existing).Where("workspace_id = ?", m.WorkspaceID).Scan(ctx); err != nil {
			return err
		}
		for _, e := range existing {
			if e.UserID != m.UserID {
				return ErrPersonalWorkspaceMembers
			}
		}
	}
	m.CreatedAt = time.Now()
	result, err := tx.NewInsert().Model(m).On("CONFLICT (user_id, workspace_id) DO NOTHING").Exec(ctx)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err == nil && n == 0 {
		return ErrMembershipExists
	}
	return err
}

func (s *Store) AddMembership(ctx context.Context, m *WorkspaceMember) error {
	return s.DB.RunInTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted}, func(ctx context.Context, tx bun.Tx) error {
		return insertMembership(ctx, tx, m)
	})
}

func protectWorkspaceAdmin(ctx context.Context, tx bun.Tx, userID, workspaceID uuid.UUID) error {
	var m WorkspaceMember
	if err := tx.NewSelect().Model(&m).Where("user_id = ? AND workspace_id = ?", userID, workspaceID).Scan(ctx); err != nil {
		return err
	}
	if m.Role != "admin" {
		return nil
	}
	n, err := tx.NewSelect().Model((*WorkspaceMember)(nil)).Where("workspace_id = ? AND role = 'admin'", workspaceID).Count(ctx)
	if err == nil && n <= 1 {
		return ErrLastWorkspaceAdmin
	}
	return err
}

func (s *Store) SetMembershipRole(ctx context.Context, actorID, userID, workspaceID uuid.UUID, role string) error {
	if role != "admin" && role != "member" {
		return ErrInvalidMemberRole
	}
	return s.DB.RunInTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted}, func(ctx context.Context, tx bun.Tx) error {
		if err := lockMembershipUser(ctx, tx, userID); err != nil {
			return err
		}
		if err := lockMembershipWorkspace(ctx, tx, workspaceID); err != nil {
			return err
		}
		var m WorkspaceMember
		if err := tx.NewSelect().Model(&m).Where("user_id = ? AND workspace_id = ?", userID, workspaceID).Scan(ctx); err != nil {
			return err
		}
		if role != "admin" && m.Role == "admin" {
			if actorID == userID {
				return ErrSelfDemotion
			}
			if err := protectWorkspaceAdmin(ctx, tx, userID, workspaceID); err != nil {
				return err
			}
			if err := invalidateCopilotFlows(ctx, tx, userID, &workspaceID); err != nil {
				return err
			}
		}
		_, err := tx.NewUpdate().Model(&m).Set("role = ?", role).Where("user_id = ? AND workspace_id = ?", userID, workspaceID).Exec(ctx)
		return err
	})
}

func (s *Store) DeleteMembership(ctx context.Context, userID, workspaceID uuid.UUID) error {
	return s.DB.RunInTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted}, func(ctx context.Context, tx bun.Tx) error {
		if err := lockMembershipUser(ctx, tx, userID); err != nil {
			return err
		}
		if err := lockMembershipWorkspace(ctx, tx, workspaceID); err != nil {
			return err
		}
		if err := protectWorkspaceAdmin(ctx, tx, userID, workspaceID); err != nil {
			return err
		}
		var teams []TeamMember
		if err := tx.NewSelect().Model(&teams).Join("JOIN workspace_teams wt ON wt.id = tm.team_id").Where("tm.user_id = ? AND wt.workspace_id = ?", userID, workspaceID).Scan(ctx); err != nil {
			return err
		}
		for _, team := range teams {
			if err := protectTeamAdmin(ctx, tx, userID, team.TeamID); err != nil {
				return err
			}
		}
		if _, err := tx.NewDelete().Model((*TeamMember)(nil)).Where("user_id = ?", userID).
			Where("team_id IN (?)", tx.NewSelect().Model((*WorkspaceTeam)(nil)).Column("id").Where("workspace_id = ?", workspaceID)).Exec(ctx); err != nil {
			return err
		}
		if _, err := tx.NewDelete().Model((*KeyUser)(nil)).Where("user_id = ?", userID).
			Where("key_id IN (?)", tx.NewSelect().Model((*InboundKey)(nil)).Column("id").Where("workspace_id = ?", workspaceID)).Exec(ctx); err != nil {
			return err
		}
		if err := invalidateCopilotFlows(ctx, tx, userID, &workspaceID); err != nil {
			return err
		}
		_, err := tx.NewDelete().Model((*WorkspaceMember)(nil)).Where("user_id = ? AND workspace_id = ?", userID, workspaceID).Exec(ctx)
		return err
	})
}

func lockTeamMembershipWorkspace(ctx context.Context, tx bun.Tx, teamID uuid.UUID) (uuid.UUID, error) {
	var team WorkspaceTeam
	if err := tx.NewSelect().Model(&team).Where("id = ?", teamID).Scan(ctx); err != nil {
		return uuid.Nil, err
	}
	if err := lockMembershipWorkspace(ctx, tx, team.WorkspaceID); err != nil {
		return uuid.Nil, err
	}
	kind, err := workspaceKindTx(ctx, tx, team.WorkspaceID)
	if err != nil {
		return uuid.Nil, err
	}
	if kind == WorkspaceKindPersonal {
		return uuid.Nil, ErrPersonalWorkspaceTeams
	}
	return team.WorkspaceID, tx.NewSelect().Model(&team).Where("id = ? AND workspace_id = ?", teamID, team.WorkspaceID).Scan(ctx)
}

func (s *Store) AddTeamMember(ctx context.Context, userID, teamID uuid.UUID, role string) error {
	if role == "" {
		role = "member"
	}
	if role != "admin" && role != "member" {
		return ErrInvalidMemberRole
	}
	return s.DB.RunInTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted}, func(ctx context.Context, tx bun.Tx) error {
		if err := lockMembershipUser(ctx, tx, userID); err != nil {
			return err
		}
		workspaceID, err := lockTeamMembershipWorkspace(ctx, tx, teamID)
		if err != nil {
			return err
		}
		exists, err := tx.NewSelect().Model((*WorkspaceMember)(nil)).Where("user_id = ? AND workspace_id = ?", userID, workspaceID).Exists(ctx)
		if err != nil {
			return err
		}
		if !exists {
			return ErrNotWorkspaceMember
		}
		m := &TeamMember{UserID: userID, TeamID: teamID, Role: role, CreatedAt: time.Now()}
		result, err := tx.NewInsert().Model(m).On("CONFLICT (user_id, team_id) DO NOTHING").Exec(ctx)
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err == nil && n == 0 {
			return ErrMembershipExists
		}
		return err
	})
}

func protectTeamAdmin(ctx context.Context, tx bun.Tx, userID, teamID uuid.UUID) error {
	var m TeamMember
	if err := tx.NewSelect().Model(&m).Where("user_id = ? AND team_id = ?", userID, teamID).Scan(ctx); err != nil {
		return err
	}
	if m.Role != "admin" {
		return nil
	}
	n, err := tx.NewSelect().Model((*TeamMember)(nil)).Where("team_id = ? AND role = 'admin'", teamID).Count(ctx)
	if err == nil && n <= 1 {
		return ErrLastTeamAdmin
	}
	return err
}

func (s *Store) transitionTeamMember(ctx context.Context, userID, teamID uuid.UUID, role string, remove bool) error {
	return s.DB.RunInTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted}, func(ctx context.Context, tx bun.Tx) error {
		if _, err := lockTeamMembershipWorkspace(ctx, tx, teamID); err != nil {
			return err
		}
		if remove || role != "admin" {
			if err := protectTeamAdmin(ctx, tx, userID, teamID); err != nil {
				return err
			}
		}
		var result sql.Result
		var err error
		if remove {
			result, err = tx.NewDelete().Model((*TeamMember)(nil)).Where("user_id = ? AND team_id = ?", userID, teamID).Exec(ctx)
		} else {
			result, err = tx.NewUpdate().Model((*TeamMember)(nil)).Set("role = ?", role).Where("user_id = ? AND team_id = ?", userID, teamID).Exec(ctx)
		}
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err == nil && n == 0 {
			return sql.ErrNoRows
		}
		return err
	})
}

func (s *Store) SetTeamMemberRole(ctx context.Context, userID, teamID uuid.UUID, role string) error {
	if role != "admin" && role != "member" {
		return ErrInvalidMemberRole
	}
	return s.transitionTeamMember(ctx, userID, teamID, role, false)
}

func (s *Store) RemoveTeamMember(ctx context.Context, userID, teamID uuid.UUID) error {
	return s.transitionTeamMember(ctx, userID, teamID, "", true)
}

func (s *Store) DeleteUser(ctx context.Context, id uuid.UUID) error {
	return s.DB.RunInTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted}, func(ctx context.Context, tx bun.Tx) error {
		var user User
		if err := tx.NewSelect().Model(&user).Where("id = ?", id).For("UPDATE").Scan(ctx); err != nil {
			return err
		}
		var workspaceIDs []uuid.UUID
		if err := tx.NewRaw(`SELECT workspace_id FROM workspace_members WHERE user_id = ?
			UNION SELECT wt.workspace_id FROM workspace_teams wt JOIN team_members tm ON tm.team_id = wt.id WHERE tm.user_id = ?
			UNION SELECT workspace_id FROM copilot_device_flows WHERE actor_id = ?
			ORDER BY workspace_id`, id, id, id).Scan(ctx, &workspaceIDs); err != nil {
			return err
		}
		for _, workspaceID := range workspaceIDs {
			if err := lockMembershipWorkspace(ctx, tx, workspaceID); err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					continue
				}
				return err
			}
			if err := protectWorkspaceAdmin(ctx, tx, id, workspaceID); err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			var teams []TeamMember
			if err := tx.NewSelect().Model(&teams).Join("JOIN workspace_teams wt ON wt.id = tm.team_id").Where("tm.user_id = ? AND wt.workspace_id = ?", id, workspaceID).Scan(ctx); err != nil {
				return err
			}
			for _, team := range teams {
				if err := protectTeamAdmin(ctx, tx, id, team.TeamID); err != nil {
					return err
				}
			}
		}
		_, err := tx.NewDelete().Model((*User)(nil)).Where("id = ?", id).Exec(ctx)
		return err
	})
}
