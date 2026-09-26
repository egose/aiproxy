package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/egose/aiproxy/internal/store"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

const (
	roleAdmin        = "admin"
	roleUser         = "user"
	roleMember       = "member"
	minPasswordChars = 8
	inviteTTL        = 7 * 24 * time.Hour
)

type adminUserView struct {
	ID       string `json:"id"`
	Email    string `json:"email"`
	Role     string `json:"role"`
	IsAdmin  bool   `json:"is_admin"`
	Disabled bool   `json:"disabled"`
	Source   string `json:"source"`
}

type adminUserOrgView struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Role string `json:"role"`
}

func (h *Handler) userOrgViews(deps Dependencies, ctx context.Context, userID uuid.UUID) []adminUserOrgView {
	memberships, err := deps.AdminStore.ListMembershipsByUser(ctx, userID)
	if err != nil {
		return nil
	}
	out := make([]adminUserOrgView, 0, len(memberships))
	for _, m := range memberships {
		org, err := deps.AdminStore.GetOrganization(ctx, m.OrgID)
		if err != nil {
			continue
		}
		out = append(out, adminUserOrgView{ID: org.ID.String(), Name: org.Name, Role: m.Role})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

type adminInviteView struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	Role      string `json:"role"`
	OrgID     string `json:"org_id,omitempty"`
	OrgName   string `json:"org_name,omitempty"`
	OrgRole   string `json:"org_role,omitempty"`
	ExpiresAt string `json:"expires_at"`
}

func (h *Handler) inviteView(deps Dependencies, r *http.Request, inv store.Invite) adminInviteView {
	view := adminInviteView{ID: inv.ID.String(), Email: inv.Email, Role: userRole(inv.IsAdmin), ExpiresAt: inv.ExpiresAt.UTC().Format(time.RFC3339)}
	if inv.OrgID != nil {
		view.OrgID = inv.OrgID.String()
		view.OrgRole = inv.OrgRole
		if org, err := deps.AdminStore.GetOrganization(r.Context(), *inv.OrgID); err == nil {
			view.OrgName = org.Name
		}
	}
	return view
}

func userRole(isAdmin bool) string {
	if isAdmin {
		return roleAdmin
	}
	return roleUser
}

func parseRole(raw string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case roleAdmin:
		return true, true
	case roleUser, "":
		return false, true
	default:
		return false, false
	}
}

func normalizeEmail(raw string) (string, bool) {
	email := strings.ToLower(strings.TrimSpace(raw))
	if email == "" || strings.ContainsAny(email, " \t\r\n") {
		return "", false
	}
	at := strings.Index(email, "@")
	if at <= 0 || at == len(email)-1 || !strings.Contains(email[at+1:], ".") {
		return "", false
	}
	return email, true
}

func newInviteToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func (h *Handler) adminUsers(deps Dependencies, w http.ResponseWriter, r *http.Request, rest []string) {
	claims, ok := h.requireAdmin(deps, w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	if len(rest) == 0 || rest[0] == "" {
		switch r.Method {
		case http.MethodGet:
			users, err := deps.AdminStore.ListUsers(ctx)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			views := make([]map[string]interface{}, 0, len(users))
			for _, u := range users {
				views = append(views, map[string]interface{}{
					"id": u.ID.String(), "email": u.Email, "role": userRole(u.IsAdmin),
					"is_admin": u.IsAdmin, "disabled": u.Disabled, "source": "database",
					"orgs": h.userOrgViews(deps, ctx, u.ID),
				})
			}
			sort.Slice(views, func(i, j int) bool {
				ei, _ := views[i]["email"].(string)
				ej, _ := views[j]["email"].(string)
				return ei < ej
			})
			writeAdminJSON(w, http.StatusOK, map[string]interface{}{"users": views})
			return
		case http.MethodPost:
			var req struct {
				Email    string `json:"email"`
				Password string `json:"password"`
				Role     string `json:"role"`
				IsAdmin  *bool  `json:"is_admin"`
			}
			if err := json.NewDecoder(ioLimitReader(r)).Decode(&req); err != nil {
				http.Error(w, "invalid body", http.StatusBadRequest)
				return
			}
			email, valid := normalizeEmail(req.Email)
			if !valid {
				http.Error(w, "valid email is required", http.StatusBadRequest)
				return
			}
			if len([]rune(req.Password)) < minPasswordChars {
				http.Error(w, "password must be at least 8 characters", http.StatusBadRequest)
				return
			}
			isAdmin := false
			if req.Role != "" {
				parsed, valid := parseRole(req.Role)
				if !valid {
					http.Error(w, `role must be "admin" or "user"`, http.StatusBadRequest)
					return
				}
				isAdmin = parsed
			} else if req.IsAdmin != nil {
				isAdmin = *req.IsAdmin
			}
			hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			u := &store.User{Email: email, PasswordHash: string(hash), IsAdmin: isAdmin}
			if err := deps.AdminStore.CreateUser(ctx, u); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			writeAdminJSON(w, http.StatusCreated, adminUserView{ID: u.ID.String(), Email: u.Email, Role: userRole(u.IsAdmin), IsAdmin: u.IsAdmin, Source: "database"})
			return
		}
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id, err := uuid.Parse(rest[0])
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	action := ""
	if len(rest) > 1 {
		action = rest[1]
	}
	users, err := deps.AdminStore.ListUsers(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var target *store.User
	for i := range users {
		if users[i].ID == id {
			target = &users[i]
		}
	}
	if target == nil {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}
	if r.Method != http.MethodPost && !(action == "" && r.Method == http.MethodDelete) {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	switch action {
	case "disable":
		if err := h.guardLastAdmin(ctx, deps, claims.Subject, target, "disable"); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		target.Disabled = true
		if err := deps.AdminStore.UpdateUser(ctx, target); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeAdminJSON(w, http.StatusOK, map[string]bool{"ok": true})
	case "enable":
		target.Disabled = false
		if err := deps.AdminStore.UpdateUser(ctx, target); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeAdminJSON(w, http.StatusOK, map[string]bool{"ok": true})
	case "role":
		var req struct {
			Role string `json:"role"`
		}
		if err := json.NewDecoder(ioLimitReader(r)).Decode(&req); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		isAdmin, valid := parseRole(req.Role)
		if !valid || req.Role == "" {
			http.Error(w, `role must be "admin" or "user"`, http.StatusBadRequest)
			return
		}
		if !isAdmin {
			if err := h.guardLastAdmin(ctx, deps, claims.Subject, target, "demote"); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}
		target.IsAdmin = isAdmin
		if err := deps.AdminStore.UpdateUser(ctx, target); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeAdminJSON(w, http.StatusOK, adminUserView{ID: target.ID.String(), Email: target.Email, Role: userRole(target.IsAdmin), IsAdmin: target.IsAdmin, Disabled: target.Disabled, Source: "database"})
	case "reset-password":
		var req struct {
			Password string `json:"password"`
		}
		if err := json.NewDecoder(ioLimitReader(r)).Decode(&req); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		if len([]rune(req.Password)) < minPasswordChars {
			http.Error(w, "password must be at least 8 characters", http.StatusBadRequest)
			return
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		target.PasswordHash = string(hash)
		if err := deps.AdminStore.UpdateUser(ctx, target); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeAdminJSON(w, http.StatusOK, map[string]bool{"ok": true})
	case "":
		if err := h.guardLastAdmin(ctx, deps, claims.Subject, target, "delete"); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := deps.AdminStore.DeleteUser(ctx, target.ID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeAdminJSON(w, http.StatusOK, map[string]bool{"ok": true})
	default:
		http.Error(w, "unknown action", http.StatusNotFound)
	}
}

func (h *Handler) guardLastAdmin(ctx context.Context, deps Dependencies, selfID string, target *store.User, action string) error {
	if target.ID.String() == selfID {
		return errBad("cannot " + action + " your own account")
	}
	if !target.IsAdmin || target.Disabled {
		return nil
	}
	count, err := deps.AdminStore.CountEnabledAdmins(ctx)
	if err != nil {
		return err
	}
	if count <= 1 {
		return errBad("cannot " + action + " the last enabled administrator")
	}
	return nil
}
