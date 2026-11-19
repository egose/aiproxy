package store

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/driver/pgdriver"
)

var (
	ErrKeyManagerRequired = errors.New("key manager required")
	ErrInvalidKeyBinding  = errors.New("key bindings must reference current workspace members and teams")
	ErrKeyNameExists      = errors.New("key name already exists; read the existing key and rotate to recover its bearer")
)

func (s *Store) IssueInboundKey(ctx context.Context, output *InboundKey, actorID uuid.UUID, userIDs, teamIDs []uuid.UUID) error {
	key := *output
	userIDs = append([]uuid.UUID(nil), userIDs...)
	teamIDs = append([]uuid.UUID(nil), teamIDs...)
	if key.OwnerUserID != nil && key.OwnerTeamID != nil {
		return ErrInvalidKeyBinding
	}
	if key.OwnerUserID != nil {
		userIDs = append(userIDs, *key.OwnerUserID)
	}
	if key.OwnerTeamID != nil {
		teamIDs = append(teamIDs, *key.OwnerTeamID)
	}
	userIDs = uniqueKeyBindingIDs(userIDs)
	teamIDs = uniqueKeyBindingIDs(teamIDs)
	err := s.DB.RunInTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted}, func(ctx context.Context, tx bun.Tx) error {
		for _, id := range uniqueKeyBindingIDs(append(append([]uuid.UUID(nil), userIDs...), actorID)) {
			if err := lockMembershipUser(ctx, tx, id); err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					if id == actorID {
						return ErrKeyManagerRequired
					}
					return ErrInvalidKeyBinding
				}
				return err
			}
		}
		if err := lockMembershipWorkspace(ctx, tx, key.WorkspaceID); err != nil {
			return err
		}
		if err := validateKeyManager(ctx, tx, key, actorID); err != nil {
			return err
		}
		for _, id := range userIDs {
			exists, err := tx.NewSelect().Model((*WorkspaceMember)(nil)).Where("user_id = ? AND workspace_id = ?", id, key.WorkspaceID).Exists(ctx)
			if err != nil {
				return err
			}
			if !exists {
				return ErrInvalidKeyBinding
			}
		}
		for _, id := range teamIDs {
			var team WorkspaceTeam
			if err := tx.NewSelect().Model(&team).Where("id = ? AND workspace_id = ?", id, key.WorkspaceID).For("KEY SHARE").Scan(ctx); err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					return ErrInvalidKeyBinding
				}
				return err
			}
		}
		key.ID = uuid.New()
		key.CreatedAt = time.Now()
		key.UpdatedAt = key.CreatedAt
		if _, err := tx.NewInsert().Model(&key).Exec(ctx); err != nil {
			var pgErr pgdriver.Error
			if errors.As(err, &pgErr) && pgErr.Field('C') == "23505" && pgErr.Field('n') == "inbound_keys_name_key" {
				return ErrKeyNameExists
			}
			return err
		}
		return replaceKeyBindings(ctx, tx, key.ID, userIDs, teamIDs)
	})
	if err == nil {
		*output = key
	}
	return err
}

func uniqueKeyBindingIDs(ids []uuid.UUID) []uuid.UUID {
	sort.Slice(ids, func(i, j int) bool { return ids[i].String() < ids[j].String() })
	out := ids[:0]
	for _, id := range ids {
		if len(out) == 0 || out[len(out)-1] != id {
			out = append(out, id)
		}
	}
	return out
}

type InboundKeyPolicyPatch struct {
	Description     *string
	Tenant          *string
	AllowedModels   []string
	SetExpiry       bool
	ExpiresAt       *time.Time
	ReplaceBindings bool
	UserIDs         []uuid.UUID
	TeamIDs         []uuid.UUID
}

func (s *Store) UpdateInboundKeyPolicy(ctx context.Context, keyID, actorID uuid.UUID, patch InboundKeyPolicyPatch) error {
	return s.updateKeyPolicy(ctx, keyID, &actorID, patch)
}

func (s *Store) updateKeyPolicy(ctx context.Context, keyID uuid.UUID, actorID *uuid.UUID, patch InboundKeyPolicyPatch) error {
	return s.DB.RunInTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted}, func(ctx context.Context, tx bun.Tx) error {
		users := append([]uuid.UUID{}, patch.UserIDs...)
		if actorID != nil {
			users = append(users, *actorID)
		}
		sort.Slice(users, func(i, j int) bool { return users[i].String() < users[j].String() })
		for i, id := range users {
			if i > 0 && users[i-1] == id {
				continue
			}
			if err := lockMembershipUser(ctx, tx, id); err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					if actorID != nil && id == *actorID {
						return ErrKeyManagerRequired
					}
					return ErrInvalidKeyBinding
				}
				return err
			}
		}
		var key InboundKey
		if err := tx.NewSelect().Model(&key).Where("id = ?", keyID).Scan(ctx); err != nil {
			return err
		}
		if err := lockMembershipWorkspace(ctx, tx, key.WorkspaceID); err != nil {
			return err
		}
		if patch.ReplaceBindings {
			for _, id := range patch.UserIDs {
				exists, err := tx.NewSelect().Model((*WorkspaceMember)(nil)).Where("user_id = ? AND workspace_id = ?", id, key.WorkspaceID).Exists(ctx)
				if err != nil {
					return err
				}
				if !exists {
					return ErrInvalidKeyBinding
				}
			}
			teams := append([]uuid.UUID{}, patch.TeamIDs...)
			sort.Slice(teams, func(i, j int) bool { return teams[i].String() < teams[j].String() })
			for _, id := range teams {
				var team WorkspaceTeam
				if err := tx.NewSelect().Model(&team).Where("id = ? AND workspace_id = ?", id, key.WorkspaceID).For("KEY SHARE").Scan(ctx); err != nil {
					if errors.Is(err, sql.ErrNoRows) {
						return ErrInvalidKeyBinding
					}
					return err
				}
			}
		}
		if err := tx.NewSelect().Model(&key).Where("id = ? AND workspace_id = ?", keyID, key.WorkspaceID).For("NO KEY UPDATE").Scan(ctx); err != nil {
			return err
		}
		if actorID != nil {
			if err := validateKeyManager(ctx, tx, key, *actorID); err != nil {
				return err
			}
		}
		columns := []string{"updated_at"}
		key.UpdatedAt = time.Now()
		if patch.Description != nil {
			key.Description = *patch.Description
			columns = append(columns, "description")
		}
		if patch.Tenant != nil {
			key.Tenant = *patch.Tenant
			columns = append(columns, "tenant")
		}
		if patch.AllowedModels != nil {
			key.AllowedModels = patch.AllowedModels
			columns = append(columns, "allowed_models")
		}
		if patch.SetExpiry {
			key.ExpiresAt = patch.ExpiresAt
			columns = append(columns, "expires_at")
		}
		if actorID != nil {
			if _, err := tx.NewUpdate().Model(&key).Column(columns...).WherePK().Exec(ctx); err != nil {
				return err
			}
		}
		if patch.ReplaceBindings {
			return replaceKeyBindings(ctx, tx, keyID, patch.UserIDs, patch.TeamIDs)
		}
		return nil
	})
}

func validateKeyManager(ctx context.Context, tx bun.Tx, key InboundKey, actorID uuid.UUID) error {
	var actor User
	if err := tx.NewSelect().Model(&actor).Where("id = ?", actorID).Scan(ctx); err != nil {
		return err
	}
	if actor.Disabled {
		return ErrKeyManagerRequired
	}
	if actor.IsAdmin {
		return nil
	}
	var member WorkspaceMember
	if err := tx.NewSelect().Model(&member).Where("user_id = ? AND workspace_id = ?", actorID, key.WorkspaceID).Scan(ctx); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrKeyManagerRequired
		}
		return err
	}
	if member.Role == "admin" || (key.OwnerUserID != nil && *key.OwnerUserID == actorID) {
		return nil
	}
	if key.OwnerTeamID != nil {
		exists, err := tx.NewSelect().Model((*TeamMember)(nil)).Where("user_id = ? AND team_id = ? AND role = 'admin'", actorID, *key.OwnerTeamID).Exists(ctx)
		if err != nil {
			return err
		}
		if exists {
			return nil
		}
	}
	return ErrKeyManagerRequired
}

func replaceKeyBindings(ctx context.Context, tx bun.Tx, keyID uuid.UUID, userIDs, teamIDs []uuid.UUID) error {
	if _, err := tx.NewDelete().Model((*KeyUser)(nil)).Where("key_id = ?", keyID).Exec(ctx); err != nil {
		return err
	}
	if _, err := tx.NewDelete().Model((*KeyTeam)(nil)).Where("key_id = ?", keyID).Exec(ctx); err != nil {
		return err
	}
	now := time.Now()
	for _, id := range userIDs {
		if _, err := tx.NewInsert().Model(&KeyUser{KeyID: keyID, UserID: id, CreatedAt: now}).Exec(ctx); err != nil {
			return err
		}
	}
	for _, id := range teamIDs {
		if _, err := tx.NewInsert().Model(&KeyTeam{KeyID: keyID, TeamID: id, CreatedAt: now}).Exec(ctx); err != nil {
			return err
		}
	}
	return nil
}
