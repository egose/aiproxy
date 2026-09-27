package httpapi

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/egose/aiproxy/internal/store"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

func (h *Handler) adminInvites(deps Dependencies, w http.ResponseWriter, r *http.Request, rest []string) {
	ctx := r.Context()
	if len(rest) == 1 && rest[0] == "accept" {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h.acceptInvite(deps, w, r)
		return
	}
	if len(rest) == 0 || rest[0] == "" {
		switch r.Method {
		case http.MethodGet:
			claims, ok := h.adminClaims(deps, r)
			if !ok {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			invites, err := deps.AdminStore.ListInvites(ctx)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			visible, _ := h.visibleWorkspaceIDs(deps, r, claims)
			views := make([]adminInviteView, 0)
			for _, inv := range invites {
				if inv.AcceptedAt != nil || !inv.ExpiresAt.After(time.Now()) {
					continue
				}
				if visible != nil && inv.WorkspaceID != nil && !visible[inv.WorkspaceID.String()] {
					continue
				}
				views = append(views, h.inviteView(deps, r, inv))
			}
			sort.Slice(views, func(i, j int) bool { return views[i].Email < views[j].Email })
			writeAdminJSON(w, http.StatusOK, map[string]interface{}{"invites": views})
			return
		case http.MethodPost:
			claims, ok := h.adminClaims(deps, r)
			if !ok {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			var req struct {
				Email         string `json:"email"`
				Role          string `json:"role"`
				WorkspaceID   string `json:"workspace_id"`
				WorkspaceRole string `json:"workspace_role"`
				IsAdmin       *bool  `json:"is_admin"`
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
			isAdmin := false
			if req.Role != "" || req.IsAdmin != nil {
				parsed := req.Role
				if parsed == "" && req.IsAdmin != nil && *req.IsAdmin {
					parsed = roleAdmin
				}
				if parsed == "" {
					parsed = roleUser
				}
				parsedAdmin, valid := parseRole(parsed)
				if !valid {
					http.Error(w, `role must be "admin" or "user"`, http.StatusBadRequest)
					return
				}
				isAdmin = parsedAdmin
			}
			if isAdmin && !claims.IsAdmin {
				http.Error(w, "only global administrators can invite global administrators", http.StatusForbidden)
				return
			}
			workspaceRole := strings.ToLower(strings.TrimSpace(req.WorkspaceRole))
			if workspaceRole == "" {
				workspaceRole = roleMember
			}
			if workspaceRole != roleAdmin && workspaceRole != roleMember {
				http.Error(w, `workspace_role must be "admin" or "member"`, http.StatusBadRequest)
				return
			}
			workspace, ok := h.resolveWriteWorkspace(deps, w, r, claims, req.WorkspaceID)
			if !ok {
				return
			}
			if _, err := deps.AdminStore.GetUserByEmail(ctx, email); err == nil {
				http.Error(w, "email is already registered", http.StatusBadRequest)
				return
			}
			token, err := newInviteToken()
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			inv := &store.Invite{Email: email, IsAdmin: isAdmin, WorkspaceID: &workspace.ID, WorkspaceRole: workspaceRole, TokenHash: store.TokenHash(token), ExpiresAt: time.Now().Add(inviteTTL)}
			if err := deps.AdminStore.CreateInvite(ctx, inv); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			writeAdminJSON(w, http.StatusCreated, map[string]interface{}{
				"invite": h.inviteView(deps, r, *inv),
				"token":  token,
			})
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
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	claims, ok := h.adminClaims(deps, r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	invites, err := deps.AdminStore.ListInvites(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var target *store.Invite
	for i := range invites {
		if invites[i].ID == id {
			target = &invites[i]
		}
	}
	if target == nil {
		http.Error(w, "invite not found", http.StatusNotFound)
		return
	}
	if target.WorkspaceID == nil {
		if !claims.IsAdmin {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
	} else if _, _, ok := h.requireWorkspaceAdmin(deps, w, r, *target.WorkspaceID); !ok {
		return
	}
	if err := deps.AdminStore.DeleteInvite(ctx, id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeAdminJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *Handler) acceptInvite(deps Dependencies, w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(ioLimitReader(r)).Decode(&req); err != nil || req.Token == "" {
		http.Error(w, "invite token is required", http.StatusBadRequest)
		return
	}
	if len([]rune(req.Password)) < minPasswordChars {
		http.Error(w, "password must be at least 8 characters", http.StatusBadRequest)
		return
	}
	inv, err := deps.AdminStore.GetInviteByHash(ctx, store.TokenHash(req.Token))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "invite not found or revoked", http.StatusNotFound)
		} else {
			http.Error(w, "could not accept invite", http.StatusInternalServerError)
		}
		return
	}
	if inv.AcceptedAt != nil || !inv.ExpiresAt.After(time.Now()) {
		http.Error(w, "invite is expired or already accepted", http.StatusBadRequest)
		return
	}
	if _, err := deps.AdminStore.GetUserByEmail(ctx, inv.Email); err == nil {
		http.Error(w, "email is already registered", http.StatusBadRequest)
		return
	} else if !errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "could not accept invite", http.StatusInternalServerError)
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, "could not accept invite", http.StatusInternalServerError)
		return
	}
	u := &store.User{PasswordHash: string(hash)}
	if err := deps.AdminStore.AcceptInvite(ctx, &inv, u); err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			http.Error(w, "invite not found or revoked", http.StatusNotFound)
		case errors.Is(err, store.ErrInviteUnavailable):
			http.Error(w, "invite is expired or already accepted", http.StatusBadRequest)
		case errors.Is(err, store.ErrInviteEmailRegistered):
			http.Error(w, "email is already registered", http.StatusBadRequest)
		case errors.Is(err, store.ErrInviteChanged):
			http.Error(w, err.Error(), http.StatusBadRequest)
		default:
			http.Error(w, "could not accept invite", http.StatusInternalServerError)
		}
		return
	}
	writeAdminJSON(w, http.StatusCreated, adminUserView{ID: u.ID.String(), Email: u.Email, Role: userRole(u.IsAdmin), IsAdmin: u.IsAdmin, Source: "database"})
}
