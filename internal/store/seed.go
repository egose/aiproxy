package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

func EnsureAdmin(ctx context.Context, s *Store) (bool, error) {
	email := strings.TrimSpace(os.Getenv("AIPROXY_ADMIN_EMAIL"))
	password := os.Getenv("AIPROXY_ADMIN_PASSWORD")
	if email == "" || password == "" {
		return false, nil
	}
	count, err := s.DB.NewSelect().Model((*User)(nil)).Count(ctx)
	if err != nil {
		return false, err
	}
	if count > 0 {
		return false, nil
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return false, err
	}
	u := &User{Email: email, PasswordHash: string(hash), IsAdmin: true}
	if _, err := s.DB.NewInsert().Model(u).Exec(ctx); err != nil {
		return false, fmt.Errorf("seed admin: %w", err)
	}
	if err := s.addSystemMembership(ctx, u.ID); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) EnsureSystemWorkspace(ctx context.Context) error {
	var existing Workspace
	err := s.DB.NewSelect().Model(&existing).Where("id = ?", SystemWorkspaceID).Scan(ctx)
	if err == nil {
		return s.syncSystemMemberships(ctx)
	}
	workspace := &Workspace{ID: SystemWorkspaceID, Name: "system", DisplayName: "System", IsSystem: true, Kind: WorkspaceKindOrganization}
	if _, err := s.DB.NewInsert().Model(workspace).Exec(ctx); err != nil {
		return fmt.Errorf("seed system workspace: %w", err)
	}
	return s.syncSystemMemberships(ctx)
}

func (s *Store) syncSystemMemberships(ctx context.Context) error {
	var admins []User
	if err := s.DB.NewSelect().Model(&admins).Where("is_admin = true AND disabled = false").Scan(ctx); err != nil {
		return err
	}
	for _, admin := range admins {
		if err := s.addSystemMembership(ctx, admin.ID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) addSystemMembership(ctx context.Context, userID uuid.UUID) error {
	err := s.AddMembership(ctx, &WorkspaceMember{UserID: userID, WorkspaceID: SystemWorkspaceID, Role: "admin"})
	if errors.Is(err, ErrMembershipExists) {
		return nil
	}
	return err
}
