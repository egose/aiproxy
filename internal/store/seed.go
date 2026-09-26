package store

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

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

func (s *Store) EnsureSystemOrg(ctx context.Context) error {
	var existing Organization
	err := s.DB.NewSelect().Model(&existing).Where("id = ?", SystemOrgID).Scan(ctx)
	if err == nil {
		return s.syncSystemMemberships(ctx)
	}
	org := &Organization{ID: SystemOrgID, Name: "system", DisplayName: "System", IsSystem: true}
	if _, err := s.DB.NewInsert().Model(org).Exec(ctx); err != nil {
		return fmt.Errorf("seed system org: %w", err)
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
	_, err := s.DB.NewInsert().Model(&OrganizationMember{UserID: userID, OrgID: SystemOrgID, Role: "admin", CreatedAt: time.Now()}).
		On("CONFLICT DO NOTHING").
		Exec(ctx)
	return err
}
