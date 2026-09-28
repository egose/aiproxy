package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/egose/aiproxy/internal/adminauth"
	"github.com/egose/aiproxy/internal/store"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type adminWorkspaceView struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"display_name,omitempty"`
	IsSystem    bool   `json:"is_system"`
	Role        string `json:"role,omitempty"`
}

type adminTeamView struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspace_id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Members     int    `json:"members,omitempty"`
	MyRole      string `json:"my_role,omitempty"`
}

type adminMemberView struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
	Role   string `json:"role"`
}

func (h *Handler) requireWorkspaceMember(deps Dependencies, w http.ResponseWriter, r *http.Request, workspaceID uuid.UUID) (store.Workspace, *adminauth.Claims, bool) {
	claims, ok := h.adminClaims(deps, r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return store.Workspace{}, nil, false
	}
	workspace, err := deps.AdminStore.GetWorkspace(r.Context(), workspaceID)
	if err != nil {
		http.Error(w, "workspace not found", http.StatusNotFound)
		return store.Workspace{}, nil, false
	}
	if claims.IsAdmin {
		return workspace, claims, true
	}
	if workspace.IsSystem {
		http.Error(w, "workspace not found", http.StatusNotFound)
		return store.Workspace{}, nil, false
	}
	if _, err := deps.AdminStore.GetMembership(r.Context(), mustParseUUID(claims.Subject), workspaceID); err != nil {
		http.Error(w, "workspace not found", http.StatusNotFound)
		return store.Workspace{}, nil, false
	}
	return workspace, claims, true
}

func (h *Handler) requireWorkspaceAdmin(deps Dependencies, w http.ResponseWriter, r *http.Request, workspaceID uuid.UUID) (store.Workspace, *adminauth.Claims, bool) {
	claims, ok := h.adminClaims(deps, r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return store.Workspace{}, nil, false
	}
	workspace, err := deps.AdminStore.GetWorkspace(r.Context(), workspaceID)
	if err != nil {
		http.Error(w, "workspace not found", http.StatusNotFound)
		return store.Workspace{}, nil, false
	}
	if claims.IsAdmin {
		return workspace, claims, true
	}
	if workspace.IsSystem {
		http.Error(w, "workspace not found", http.StatusNotFound)
		return store.Workspace{}, nil, false
	}
	membership, err := deps.AdminStore.GetMembership(r.Context(), mustParseUUID(claims.Subject), workspaceID)
	if err != nil {
		http.Error(w, "workspace not found", http.StatusNotFound)
		return store.Workspace{}, nil, false
	}
	if membership.Role != roleAdmin {
		http.Error(w, "workspace admin required", http.StatusForbidden)
		return store.Workspace{}, nil, false
	}
	return workspace, claims, true
}

func mustParseUUID(raw string) uuid.UUID {
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil
	}
	return id
}

func (h *Handler) callerTeamRoles(deps Dependencies, ctx context.Context, claims *adminauth.Claims) map[string]string {
	out := map[string]string{}
	if claims.IsAdmin {
		return out
	}
	memberships, err := deps.AdminStore.ListTeamMembershipsByUser(ctx, mustParseUUID(claims.Subject))
	if err != nil {
		return out
	}
	for _, m := range memberships {
		role := m.Role
		if role == "" {
			role = roleMember
		}
		out[m.TeamID.String()] = role
	}
	return out
}

func (h *Handler) isTeamAdmin(deps Dependencies, ctx context.Context, claims *adminauth.Claims, teamID uuid.UUID) bool {
	if claims.IsAdmin {
		return true
	}
	team, err := deps.AdminStore.GetTeam(ctx, teamID)
	if err != nil || !h.canAccessWorkspace(deps, ctx, claims, team.WorkspaceID) {
		return false
	}
	m, err := deps.AdminStore.GetTeamMember(ctx, mustParseUUID(claims.Subject), teamID)
	if err != nil {
		return false
	}
	role := m.Role
	if role == "" {
		role = roleMember
	}
	return role == roleAdmin
}

func (h *Handler) requireTeamManager(deps Dependencies, w http.ResponseWriter, r *http.Request, workspaceID, teamID uuid.UUID) (store.Workspace, *adminauth.Claims, store.WorkspaceTeam, bool) {
	claims, ok := h.adminClaims(deps, r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return store.Workspace{}, nil, store.WorkspaceTeam{}, false
	}
	workspace, err := deps.AdminStore.GetWorkspace(r.Context(), workspaceID)
	if err != nil {
		http.Error(w, "workspace not found", http.StatusNotFound)
		return store.Workspace{}, nil, store.WorkspaceTeam{}, false
	}
	if claims.IsAdmin {
		team, err := deps.AdminStore.GetTeam(r.Context(), teamID)
		if err != nil || team.WorkspaceID != workspaceID {
			http.Error(w, "team not found", http.StatusNotFound)
			return store.Workspace{}, nil, store.WorkspaceTeam{}, false
		}
		return workspace, claims, team, true
	}
	if workspace.IsSystem {
		http.Error(w, "workspace not found", http.StatusNotFound)
		return store.Workspace{}, nil, store.WorkspaceTeam{}, false
	}
	membership, err := deps.AdminStore.GetMembership(r.Context(), mustParseUUID(claims.Subject), workspaceID)
	if err != nil {
		http.Error(w, "workspace not found", http.StatusNotFound)
		return store.Workspace{}, nil, store.WorkspaceTeam{}, false
	}
	team, err := deps.AdminStore.GetTeam(r.Context(), teamID)
	if err != nil || team.WorkspaceID != workspaceID {
		http.Error(w, "team not found", http.StatusNotFound)
		return store.Workspace{}, nil, store.WorkspaceTeam{}, false
	}
	if membership.Role == roleAdmin || h.isTeamAdmin(deps, r.Context(), claims, teamID) {
		return workspace, claims, team, true
	}
	http.Error(w, "team admin required", http.StatusForbidden)
	return store.Workspace{}, nil, store.WorkspaceTeam{}, false
}

func (h *Handler) callerWorkspaces(deps Dependencies, r *http.Request, claims *adminauth.Claims) ([]store.Workspace, error) {
	if claims.IsAdmin {
		return deps.AdminStore.ListWorkspaces(r.Context())
	}
	memberships, err := deps.AdminStore.ListMembershipsByUser(r.Context(), mustParseUUID(claims.Subject))
	if err != nil {
		return nil, err
	}
	out := make([]store.Workspace, 0, len(memberships))
	for _, m := range memberships {
		workspace, err := deps.AdminStore.GetWorkspace(r.Context(), m.WorkspaceID)
		if err != nil {
			continue
		}
		out = append(out, workspace)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (h *Handler) adminWorkspaces(deps Dependencies, w http.ResponseWriter, r *http.Request, rest []string) {
	if len(rest) == 0 || rest[0] == "" {
		switch r.Method {
		case http.MethodGet:
			claims, ok := h.adminClaims(deps, r)
			if !ok {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			workspaces, err := h.callerWorkspaces(deps, r, claims)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			roleByWorkspace := map[string]string{}
			if !claims.IsAdmin {
				memberships, err := deps.AdminStore.ListMembershipsByUser(r.Context(), mustParseUUID(claims.Subject))
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				for _, m := range memberships {
					roleByWorkspace[m.WorkspaceID.String()] = m.Role
				}
			}
			views := make([]adminWorkspaceView, 0, len(workspaces))
			for _, workspace := range workspaces {
				views = append(views, adminWorkspaceView{ID: workspace.ID.String(), Name: workspace.Name, DisplayName: workspace.DisplayName, IsSystem: workspace.IsSystem, Role: roleByWorkspace[workspace.ID.String()]})
			}
			writeAdminJSON(w, http.StatusOK, map[string]interface{}{"workspaces": views})
			return
		case http.MethodPost:
			claims, ok := h.adminClaims(deps, r)
			if !ok {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			var req struct {
				Name        string `json:"name"`
				DisplayName string `json:"display_name"`
			}
			if err := json.NewDecoder(ioLimitReader(r)).Decode(&req); err != nil {
				http.Error(w, "invalid body", http.StatusBadRequest)
				return
			}
			name := strings.ToLower(strings.TrimSpace(req.Name))
			if !validResourceName(name) {
				http.Error(w, "invalid workspace name", http.StatusBadRequest)
				return
			}
			workspace := &store.Workspace{Name: name, DisplayName: strings.TrimSpace(req.DisplayName)}
			if err := deps.AdminStore.CreateWorkspace(r.Context(), workspace); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if err := deps.AdminStore.AddMembership(r.Context(), &store.WorkspaceMember{UserID: mustParseUUID(claims.Subject), WorkspaceID: workspace.ID, Role: roleAdmin}); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeAdminJSON(w, http.StatusCreated, adminWorkspaceView{ID: workspace.ID.String(), Name: workspace.Name, DisplayName: workspace.DisplayName, Role: roleAdmin})
			return
		}
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	workspaceID, err := uuid.Parse(rest[0])
	if err != nil {
		http.Error(w, "invalid workspace id", http.StatusBadRequest)
		return
	}
	if len(rest) == 1 {
		switch r.Method {
		case http.MethodGet:
			workspace, _, ok := h.requireWorkspaceMember(deps, w, r, workspaceID)
			if !ok {
				return
			}
			writeAdminJSON(w, http.StatusOK, h.workspaceDetailView(deps, r, workspace))
			return
		case http.MethodPut:
			workspace, _, ok := h.requireWorkspaceAdmin(deps, w, r, workspaceID)
			if !ok {
				return
			}
			var req struct {
				DisplayName *string `json:"display_name"`
			}
			if err := json.NewDecoder(ioLimitReader(r)).Decode(&req); err != nil {
				http.Error(w, "invalid body", http.StatusBadRequest)
				return
			}
			if req.DisplayName != nil {
				workspace.DisplayName = strings.TrimSpace(*req.DisplayName)
			}
			if err := deps.AdminStore.UpdateWorkspace(r.Context(), &workspace); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeAdminJSON(w, http.StatusOK, h.workspaceDetailView(deps, r, workspace))
			return
		case http.MethodDelete:
			workspace, claims, ok := h.requireWorkspaceAdmin(deps, w, r, workspaceID)
			if !ok {
				return
			}
			if workspace.IsSystem {
				http.Error(w, "system workspace cannot be deleted", http.StatusBadRequest)
				return
			}
			if n, err := deps.AdminStore.CountProvidersByWorkspace(r.Context(), workspaceID); err != nil || n > 0 {
				if err == nil {
					http.Error(w, "workspace still owns providers", http.StatusBadRequest)
					return
				}
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if n, err := deps.AdminStore.CountAliasesByWorkspace(r.Context(), workspaceID); err != nil || n > 0 {
				if err == nil {
					http.Error(w, "workspace still owns aliases", http.StatusBadRequest)
					return
				}
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if n, err := deps.AdminStore.CountKeysByWorkspace(r.Context(), workspaceID); err != nil || n > 0 {
				if err == nil {
					http.Error(w, "workspace still owns api keys", http.StatusBadRequest)
					return
				}
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			members, err := deps.AdminStore.ListMembershipsByWorkspace(r.Context(), workspaceID)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			for _, m := range members {
				if m.UserID.String() != claims.Subject {
					http.Error(w, "workspace still has members", http.StatusBadRequest)
					return
				}
			}
			if err := deps.AdminStore.DeleteWorkspace(r.Context(), workspaceID); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeAdminJSON(w, http.StatusOK, map[string]bool{"ok": true})
			return
		}
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	switch rest[1] {
	case "teams":
		h.adminWorkspaceTeams(deps, w, r, workspaceID, rest[2:])
		return
	case "members":
		h.adminWorkspaceMembers(deps, w, r, workspaceID, rest[2:])
		return
	case "users":
		if len(rest) < 3 {
			http.Error(w, "unknown admin endpoint", http.StatusNotFound)
			return
		}
		userID, err := uuid.Parse(rest[2])
		if err != nil {
			http.Error(w, "invalid user id", http.StatusBadRequest)
			return
		}
		h.adminUserQuota(deps, w, r, workspaceID, userID, rest[3:])
		return
	}
	http.Error(w, "unknown admin endpoint", http.StatusNotFound)
}

type adminWorkspaceMemberView struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
	Role   string `json:"role"`
}

func (h *Handler) workspaceDetailView(deps Dependencies, r *http.Request, workspace store.Workspace) map[string]interface{} {
	ctx := r.Context()
	teams, _ := deps.AdminStore.ListTeamsByWorkspace(ctx, workspace.ID)
	members, _ := deps.AdminStore.ListMembershipsByWorkspace(ctx, workspace.ID)
	users, _ := deps.AdminStore.ListUsers(ctx)
	emailByID := make(map[string]string, len(users))
	for _, u := range users {
		emailByID[u.ID.String()] = u.Email
	}
	teamViews := make([]adminTeamView, 0, len(teams))
	for _, t := range teams {
		teamViews = append(teamViews, adminTeamView{ID: t.ID.String(), WorkspaceID: t.WorkspaceID.String(), Name: t.Name, Description: t.Description})
	}
	memberViews := make([]adminWorkspaceMemberView, 0, len(members))
	for _, m := range members {
		memberViews = append(memberViews, adminWorkspaceMemberView{UserID: m.UserID.String(), Email: emailByID[m.UserID.String()], Role: m.Role})
	}
	providers, _ := deps.AdminStore.CountProvidersByWorkspace(ctx, workspace.ID)
	aliases, _ := deps.AdminStore.CountAliasesByWorkspace(ctx, workspace.ID)
	keys, _ := deps.AdminStore.CountKeysByWorkspace(ctx, workspace.ID)
	return map[string]interface{}{
		"id": workspace.ID.String(), "name": workspace.Name, "display_name": workspace.DisplayName, "is_system": workspace.IsSystem,
		"teams": teamViews, "members": memberViews,
		"resources": map[string]int{"providers": providers, "aliases": aliases, "keys": keys},
	}
}

func (h *Handler) adminWorkspaceTeams(deps Dependencies, w http.ResponseWriter, r *http.Request, workspaceID uuid.UUID, rest []string) {
	ctx := r.Context()
	if len(rest) == 0 || rest[0] == "" {
		switch r.Method {
		case http.MethodGet:
			workspace, claims, ok := h.requireWorkspaceMember(deps, w, r, workspaceID)
			if !ok {
				return
			}
			_ = workspace
			teams, err := deps.AdminStore.ListTeamsByWorkspace(ctx, workspaceID)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			roles := h.callerTeamRoles(deps, ctx, claims)
			views := make([]adminTeamView, 0, len(teams))
			for _, t := range teams {
				views = append(views, adminTeamView{ID: t.ID.String(), WorkspaceID: t.WorkspaceID.String(), Name: t.Name, Description: t.Description, MyRole: roles[t.ID.String()]})
			}
			writeAdminJSON(w, http.StatusOK, map[string]interface{}{"teams": views})
			return
		case http.MethodPost:
			if _, _, ok := h.requireWorkspaceAdmin(deps, w, r, workspaceID); !ok {
				return
			}
			var req struct {
				Name        string `json:"name"`
				Description string `json:"description"`
			}
			if err := json.NewDecoder(ioLimitReader(r)).Decode(&req); err != nil {
				http.Error(w, "invalid body", http.StatusBadRequest)
				return
			}
			name := strings.ToLower(strings.TrimSpace(req.Name))
			if !validResourceName(name) {
				http.Error(w, "invalid team name", http.StatusBadRequest)
				return
			}
			team := &store.WorkspaceTeam{WorkspaceID: workspaceID, Name: name, Description: strings.TrimSpace(req.Description)}
			if err := deps.AdminStore.CreateTeam(ctx, team); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			writeAdminJSON(w, http.StatusCreated, adminTeamView{ID: team.ID.String(), WorkspaceID: team.WorkspaceID.String(), Name: team.Name, Description: team.Description})
			return
		}
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	teamID, err := uuid.Parse(rest[0])
	if err != nil {
		http.Error(w, "invalid team id", http.StatusBadRequest)
		return
	}
	team, err := deps.AdminStore.GetTeam(ctx, teamID)
	if err != nil || team.WorkspaceID != workspaceID {
		http.Error(w, "team not found", http.StatusNotFound)
		return
	}
	if len(rest) == 2 && rest[1] == "quota" {
		h.adminTeamQuota(deps, w, r, workspaceID, teamID)
		return
	}
	if len(rest) == 1 {
		switch r.Method {
		case http.MethodGet:
			if _, _, ok := h.requireWorkspaceMember(deps, w, r, workspaceID); !ok {
				return
			}
			members, err := deps.AdminStore.ListTeamMembers(ctx, teamID)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeAdminJSON(w, http.StatusOK, map[string]interface{}{
				"team":    adminTeamView{ID: team.ID.String(), WorkspaceID: team.WorkspaceID.String(), Name: team.Name, Description: team.Description},
				"members": h.teamMemberViews(deps, r, members),
			})
			return
		case http.MethodPut:
			if _, _, ok := h.requireWorkspaceAdmin(deps, w, r, workspaceID); !ok {
				return
			}
			var req struct {
				Name        *string `json:"name"`
				Description *string `json:"description"`
			}
			if err := json.NewDecoder(ioLimitReader(r)).Decode(&req); err != nil {
				http.Error(w, "invalid body", http.StatusBadRequest)
				return
			}
			if req.Name != nil {
				name := strings.ToLower(strings.TrimSpace(*req.Name))
				if !validResourceName(name) {
					http.Error(w, "invalid team name", http.StatusBadRequest)
					return
				}
				team.Name = name
			}
			if req.Description != nil {
				team.Description = strings.TrimSpace(*req.Description)
			}
			if err := deps.AdminStore.UpdateTeam(ctx, &team); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			writeAdminJSON(w, http.StatusOK, adminTeamView{ID: team.ID.String(), WorkspaceID: team.WorkspaceID.String(), Name: team.Name, Description: team.Description})
			return
		case http.MethodDelete:
			if _, _, ok := h.requireWorkspaceAdmin(deps, w, r, workspaceID); !ok {
				return
			}
			if n, err := deps.AdminStore.CountKeysByTeam(ctx, teamID); err != nil || n > 0 {
				if err == nil {
					http.Error(w, "team still owns api keys", http.StatusBadRequest)
					return
				}
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if err := deps.AdminStore.DeleteTeam(ctx, teamID); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeAdminJSON(w, http.StatusOK, map[string]bool{"ok": true})
			return
		}
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if len(rest) == 2 && rest[1] == "members" {
		switch r.Method {
		case http.MethodGet:
			if _, _, ok := h.requireWorkspaceMember(deps, w, r, workspaceID); !ok {
				return
			}
			members, err := deps.AdminStore.ListTeamMembers(ctx, teamID)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeAdminJSON(w, http.StatusOK, map[string]interface{}{"members": h.teamMemberViews(deps, r, members)})
			return
		case http.MethodPost:
			if _, _, _, ok := h.requireTeamManager(deps, w, r, workspaceID, teamID); !ok {
				return
			}
			var req struct {
				UserID string `json:"user_id"`
				Email  string `json:"email"`
				Role   string `json:"role"`
			}
			if err := json.NewDecoder(ioLimitReader(r)).Decode(&req); err != nil {
				http.Error(w, "invalid body", http.StatusBadRequest)
				return
			}
			role := strings.ToLower(strings.TrimSpace(req.Role))
			if role == "" {
				role = roleMember
			}
			if role != roleAdmin && role != roleMember {
				http.Error(w, `role must be "admin" or "member"`, http.StatusBadRequest)
				return
			}
			userID, ok := h.resolveMemberUser(deps, w, r, workspaceID, req.UserID, req.Email)
			if !ok {
				return
			}
			if err := deps.AdminStore.AddTeamMember(ctx, userID, teamID, role); err != nil {
				writeMembershipError(w, err)
				return
			}
			writeAdminJSON(w, http.StatusCreated, map[string]bool{"ok": true})
			return
		}
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if len(rest) == 3 && rest[1] == "members" {
		userID, err := uuid.Parse(rest[2])
		if err != nil {
			http.Error(w, "invalid user id", http.StatusBadRequest)
			return
		}
		switch r.Method {
		case http.MethodPut:
			_, claims, _, ok := h.requireTeamManager(deps, w, r, workspaceID, teamID)
			if !ok {
				return
			}
			_ = claims
			var req struct {
				Role string `json:"role"`
			}
			if err := json.NewDecoder(ioLimitReader(r)).Decode(&req); err != nil {
				http.Error(w, "invalid body", http.StatusBadRequest)
				return
			}
			role := strings.ToLower(strings.TrimSpace(req.Role))
			if role != roleAdmin && role != roleMember {
				http.Error(w, `role must be "admin" or "member"`, http.StatusBadRequest)
				return
			}
			if err := deps.AdminStore.SetTeamMemberRole(ctx, userID, teamID, role); err != nil {
				writeMembershipError(w, err)
				return
			}
			writeAdminJSON(w, http.StatusOK, map[string]bool{"ok": true})
			return
		case http.MethodDelete:
			if _, _, _, ok := h.requireTeamManager(deps, w, r, workspaceID, teamID); !ok {
				return
			}
			if err := deps.AdminStore.RemoveTeamMember(ctx, userID, teamID); err != nil {
				writeMembershipError(w, err)
				return
			}
			writeAdminJSON(w, http.StatusOK, map[string]bool{"ok": true})
			return
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
	}
	http.Error(w, "unknown admin endpoint", http.StatusNotFound)
}

func (h *Handler) resolveMemberUser(deps Dependencies, w http.ResponseWriter, r *http.Request, workspaceID uuid.UUID, userID, email string) (uuid.UUID, bool) {
	ctx := r.Context()
	var id uuid.UUID
	var err error
	if strings.TrimSpace(userID) != "" {
		id, err = uuid.Parse(strings.TrimSpace(userID))
		if err != nil {
			http.Error(w, "invalid user id", http.StatusBadRequest)
			return uuid.Nil, false
		}
	} else if strings.TrimSpace(email) != "" {
		normalized, valid := normalizeEmail(email)
		if !valid {
			http.Error(w, "valid email or user id is required", http.StatusBadRequest)
			return uuid.Nil, false
		}
		var u store.User
		u, err = deps.AdminStore.GetUserByEmail(ctx, normalized)
		if err != nil {
			http.Error(w, "user not found, invite them first", http.StatusNotFound)
			return uuid.Nil, false
		}
		id = u.ID
	} else {
		http.Error(w, "user id or email is required", http.StatusBadRequest)
		return uuid.Nil, false
	}
	if _, err := deps.AdminStore.GetMembership(ctx, id, workspaceID); err != nil {
		http.Error(w, "user is not an workspace member", http.StatusBadRequest)
		return uuid.Nil, false
	}
	return id, true
}

func (h *Handler) adminWorkspaceMembers(deps Dependencies, w http.ResponseWriter, r *http.Request, workspaceID uuid.UUID, rest []string) {
	ctx := r.Context()
	if len(rest) == 0 || rest[0] == "" {
		switch r.Method {
		case http.MethodGet:
			if _, _, ok := h.requireWorkspaceMember(deps, w, r, workspaceID); !ok {
				return
			}
			members, err := deps.AdminStore.ListMembershipsByWorkspace(ctx, workspaceID)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			users, err := deps.AdminStore.ListUsers(ctx)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			emailByID := make(map[string]string, len(users))
			for _, u := range users {
				emailByID[u.ID.String()] = u.Email
			}
			views := make([]adminWorkspaceMemberView, 0, len(members))
			for _, m := range members {
				views = append(views, adminWorkspaceMemberView{UserID: m.UserID.String(), Email: emailByID[m.UserID.String()], Role: m.Role})
			}
			sort.Slice(views, func(i, j int) bool { return views[i].Email < views[j].Email })
			writeAdminJSON(w, http.StatusOK, map[string]interface{}{"members": views})
			return
		case http.MethodPost:
			workspace, claims, ok := h.requireWorkspaceAdmin(deps, w, r, workspaceID)
			if !ok {
				return
			}
			_ = workspace
			_ = claims
			var req struct {
				UserID string `json:"user_id"`
				Email  string `json:"email"`
				Role   string `json:"role"`
			}
			if err := json.NewDecoder(ioLimitReader(r)).Decode(&req); err != nil {
				http.Error(w, "invalid body", http.StatusBadRequest)
				return
			}
			role := strings.ToLower(strings.TrimSpace(req.Role))
			if role == "" {
				role = roleMember
			}
			if role != roleAdmin && role != roleMember {
				http.Error(w, `role must be "admin" or "member"`, http.StatusBadRequest)
				return
			}
			var userID uuid.UUID
			if strings.TrimSpace(req.UserID) != "" {
				var err error
				userID, err = uuid.Parse(strings.TrimSpace(req.UserID))
				if err != nil {
					http.Error(w, "invalid user id", http.StatusBadRequest)
					return
				}
			} else if strings.TrimSpace(req.Email) != "" {
				email, valid := normalizeEmail(req.Email)
				if !valid {
					http.Error(w, "valid email or user id is required", http.StatusBadRequest)
					return
				}
				u, err := deps.AdminStore.GetUserByEmail(ctx, email)
				if err != nil {
					http.Error(w, "user not found, invite them first", http.StatusNotFound)
					return
				}
				userID = u.ID
			} else {
				http.Error(w, "user id or email is required", http.StatusBadRequest)
				return
			}
			if err := deps.AdminStore.AddMembership(ctx, &store.WorkspaceMember{UserID: userID, WorkspaceID: workspaceID, Role: role}); err != nil {
				writeMembershipError(w, err)
				return
			}
			writeAdminJSON(w, http.StatusCreated, map[string]bool{"ok": true})
			return
		}
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	memberID, err := uuid.Parse(rest[0])
	if err != nil {
		http.Error(w, "invalid user id", http.StatusBadRequest)
		return
	}
	switch r.Method {
	case http.MethodPut:
		_, claims, ok := h.requireWorkspaceAdmin(deps, w, r, workspaceID)
		if !ok {
			return
		}
		var req struct {
			Role string `json:"role"`
		}
		if err := json.NewDecoder(ioLimitReader(r)).Decode(&req); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		role := strings.ToLower(strings.TrimSpace(req.Role))
		if role != roleAdmin && role != roleMember {
			http.Error(w, `role must be "admin" or "member"`, http.StatusBadRequest)
			return
		}
		if err := deps.AdminStore.SetMembershipRole(ctx, mustParseUUID(claims.Subject), memberID, workspaceID, role); err != nil {
			writeMembershipError(w, err)
			return
		}
		writeAdminJSON(w, http.StatusOK, map[string]bool{"ok": true})
	case http.MethodDelete:
		_, claims, ok := h.requireWorkspaceAdmin(deps, w, r, workspaceID)
		if !ok {
			return
		}
		_ = claims
		if err := deps.AdminStore.DeleteMembership(ctx, memberID, workspaceID); err != nil {
			writeMembershipError(w, err)
			return
		}
		writeAdminJSON(w, http.StatusOK, map[string]bool{"ok": true})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) adminRegister(deps Dependencies, w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !deps.MultiTenancy.AllowPublicRegistration {
		http.Error(w, "public registration is disabled", http.StatusForbidden)
		return
	}
	var req struct {
		Email         string `json:"email"`
		Password      string `json:"password"`
		WorkspaceName string `json:"workspace_name"`
		DisplayName   string `json:"display_name"`
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
	if _, err := deps.AdminStore.GetUserByEmail(ctx, email); err == nil {
		http.Error(w, "email is already registered", http.StatusBadRequest)
		return
	}
	workspaceName := strings.ToLower(strings.TrimSpace(req.WorkspaceName))
	if workspaceName == "" {
		workspaceName = deriveWorkspaceName(email)
	}
	if !validResourceName(workspaceName) {
		http.Error(w, "invalid workspace name", http.StatusBadRequest)
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	u := &store.User{Email: email, PasswordHash: string(hash)}
	workspace := &store.Workspace{Name: workspaceName, DisplayName: strings.TrimSpace(req.DisplayName)}
	if err := deps.AdminStore.CreateUserWithWorkspace(ctx, u, workspace, roleAdmin); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeAdminJSON(w, http.StatusCreated, map[string]interface{}{
		"user":      adminUserView{ID: u.ID.String(), Email: u.Email, Role: roleUser, Source: "database"},
		"workspace": adminWorkspaceView{ID: workspace.ID.String(), Name: workspace.Name, DisplayName: workspace.DisplayName, Role: roleAdmin},
	})
}

func deriveWorkspaceName(email string) string {
	local := email
	if at := strings.Index(email, "@"); at > 0 {
		local = email[:at]
	}
	var b strings.Builder
	for _, c := range strings.ToLower(local) {
		if c >= 'a' && c <= 'z' || c >= '0' && c <= '9' {
			b.WriteRune(c)
		}
	}
	name := b.String()
	if name == "" {
		name = "personal"
	}
	if len(name) > 50 {
		name = name[:50]
	}
	return name + "-workspace"
}

func (h *Handler) ensurePersonalWorkspace(ctx context.Context, deps Dependencies, userID uuid.UUID, email string) error {
	memberships, err := deps.AdminStore.ListMembershipsByUser(ctx, userID)
	if err != nil {
		return err
	}
	if len(memberships) > 0 {
		return nil
	}
	base := deriveWorkspaceName(email)
	for i := 0; i < 100; i++ {
		name := base
		if i > 0 {
			name = fmt.Sprintf("%.50s-%d", strings.TrimSuffix(base, "-workspace"), i+1)
		}
		workspace := &store.Workspace{Name: name}
		if err := deps.AdminStore.CreatePersonalWorkspace(ctx, userID, workspace); err != nil {
			continue
		}
		return nil
	}
	return errBad("could not allocate a personal workspace")
}

func writeMembershipError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrMembershipExists):
		http.Error(w, err.Error(), http.StatusConflict)
	case errors.Is(err, store.ErrLastWorkspaceAdmin), errors.Is(err, store.ErrLastTeamAdmin),
		errors.Is(err, store.ErrSelfDemotion), errors.Is(err, store.ErrInvalidMemberRole), errors.Is(err, store.ErrNotWorkspaceMember),
		errors.Is(err, store.ErrPersonalWorkspaceTeams), errors.Is(err, store.ErrPersonalWorkspaceMembers),
		errors.Is(err, store.ErrPersonalWorkspaceExists), errors.Is(err, store.ErrInvalidWorkspaceKind):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, sql.ErrNoRows):
		http.Error(w, "membership or user not found", http.StatusNotFound)
	default:
		http.Error(w, "could not update membership", http.StatusInternalServerError)
	}
}

func (h *Handler) requestWorkspaceID(r *http.Request, workspaceIDStr string) string {
	if strings.TrimSpace(workspaceIDStr) != "" {
		return strings.TrimSpace(workspaceIDStr)
	}
	return strings.TrimSpace(r.Header.Get("X-Workspace-ID"))
}

func (h *Handler) resolveWriteWorkspace(deps Dependencies, w http.ResponseWriter, r *http.Request, claims *adminauth.Claims, workspaceIDStr string) (store.Workspace, bool) {
	ctx := r.Context()
	workspaceIDStr = h.requestWorkspaceID(r, workspaceIDStr)
	if workspaceIDStr != "" {
		workspaceID, err := uuid.Parse(strings.TrimSpace(workspaceIDStr))
		if err != nil {
			http.Error(w, "invalid workspace id", http.StatusBadRequest)
			return store.Workspace{}, false
		}
		if _, _, ok := h.requireWorkspaceAdmin(deps, w, r, workspaceID); !ok {
			return store.Workspace{}, false
		}
		workspace, err := deps.AdminStore.GetWorkspace(ctx, workspaceID)
		if err != nil {
			http.Error(w, "workspace not found", http.StatusNotFound)
			return store.Workspace{}, false
		}
		return workspace, true
	}
	memberships, err := deps.AdminStore.ListMembershipsByUser(ctx, mustParseUUID(claims.Subject))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return store.Workspace{}, false
	}
	if claims.IsAdmin && len(memberships) == 0 {
		workspace, err := deps.AdminStore.GetWorkspace(ctx, store.SystemWorkspaceID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return store.Workspace{}, false
		}
		return workspace, true
	}
	adminOf := []store.WorkspaceMember{}
	for _, m := range memberships {
		if m.Role != roleAdmin {
			continue
		}
		workspace, err := deps.AdminStore.GetWorkspace(ctx, m.WorkspaceID)
		if err != nil || workspace.IsSystem {
			continue
		}
		adminOf = append(adminOf, m)
	}
	if len(adminOf) == 1 {
		workspace, err := deps.AdminStore.GetWorkspace(ctx, adminOf[0].WorkspaceID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return store.Workspace{}, false
		}
		return workspace, true
	}
	http.Error(w, "specify workspace id", http.StatusBadRequest)
	return store.Workspace{}, false
}

func (h *Handler) visibleWorkspaceIDs(deps Dependencies, r *http.Request, claims *adminauth.Claims) (map[string]bool, bool) {
	if claims.IsAdmin {
		return nil, true
	}
	memberships, err := deps.AdminStore.ListMembershipsByUser(r.Context(), mustParseUUID(claims.Subject))
	if err != nil {
		return nil, false
	}
	out := make(map[string]bool, len(memberships))
	for _, m := range memberships {
		out[m.WorkspaceID.String()] = true
	}
	return out, true
}

func (h *Handler) canSeeSystemWorkspace(deps Dependencies, r *http.Request, claims *adminauth.Claims, filterWorkspace string) bool {
	if filterWorkspace != "" && filterWorkspace != store.SystemWorkspaceID.String() {
		return false
	}
	visible, _ := h.visibleWorkspaceIDs(deps, r, claims)
	return visible == nil || visible[store.SystemWorkspaceID.String()]
}

func (h *Handler) workspaceNameMap(deps Dependencies, r *http.Request, claims *adminauth.Claims) map[string]string {
	out := map[string]string{}
	var workspaces []store.Workspace
	var err error
	if claims.IsAdmin {
		workspaces, err = deps.AdminStore.ListWorkspaces(r.Context())
	} else {
		workspaces, err = h.callerWorkspaces(deps, r, claims)
	}
	if err != nil {
		return out
	}
	for _, workspace := range workspaces {
		out[workspace.ID.String()] = workspace.Name
	}
	return out
}

func (h *Handler) resolveWorkspaceFilter(deps Dependencies, w http.ResponseWriter, r *http.Request, claims *adminauth.Claims, queryWorkspace string) (string, bool) {
	queryWorkspace = h.requestWorkspaceID(r, queryWorkspace)
	if queryWorkspace == "" {
		return "", true
	}
	if _, err := uuid.Parse(queryWorkspace); err != nil {
		http.Error(w, "invalid workspace id", http.StatusBadRequest)
		return "", false
	}
	visible, _ := h.visibleWorkspaceIDs(deps, r, claims)
	if visible != nil && !visible[queryWorkspace] {
		http.Error(w, "workspace not found", http.StatusNotFound)
		return "", false
	}
	if _, err := deps.AdminStore.GetWorkspace(r.Context(), mustParseUUID(queryWorkspace)); err != nil {
		http.Error(w, "workspace not found", http.StatusNotFound)
		return "", false
	}
	return queryWorkspace, true
}

func (h *Handler) requireResourceWorkspace(deps Dependencies, w http.ResponseWriter, r *http.Request, claims *adminauth.Claims, workspaceID uuid.UUID) bool {
	if h.canWriteWorkspace(deps, r.Context(), claims, workspaceID) {
		return true
	}
	if claims.IsAdmin {
		http.Error(w, "workspace not found", http.StatusNotFound)
		return false
	}
	workspace, err := deps.AdminStore.GetWorkspace(r.Context(), workspaceID)
	if err != nil || workspace.IsSystem {
		http.Error(w, "workspace not found", http.StatusNotFound)
		return false
	}
	http.Error(w, "workspace admin required", http.StatusForbidden)
	return false
}

func (h *Handler) canAccessWorkspace(deps Dependencies, ctx context.Context, claims *adminauth.Claims, workspaceID uuid.UUID) bool {
	if claims.IsAdmin {
		return true
	}
	workspace, err := deps.AdminStore.GetWorkspace(ctx, workspaceID)
	if err != nil || workspace.IsSystem {
		return false
	}
	_, err = deps.AdminStore.GetMembership(ctx, mustParseUUID(claims.Subject), workspaceID)
	return err == nil
}

func (h *Handler) canWriteWorkspace(deps Dependencies, ctx context.Context, claims *adminauth.Claims, workspaceID uuid.UUID) bool {
	if claims.IsAdmin {
		return true
	}
	workspace, err := deps.AdminStore.GetWorkspace(ctx, workspaceID)
	if err != nil || workspace.IsSystem {
		return false
	}
	membership, err := deps.AdminStore.GetMembership(ctx, mustParseUUID(claims.Subject), workspaceID)
	if err != nil {
		return false
	}
	return membership.Role == roleAdmin
}

type adminTeamMemberView struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
	Role   string `json:"role"`
}

func (h *Handler) teamMemberViews(deps Dependencies, r *http.Request, members []store.TeamMember) []adminTeamMemberView {
	users, err := deps.AdminStore.ListUsers(r.Context())
	emailByID := make(map[string]string, len(users))
	if err == nil {
		for _, u := range users {
			emailByID[u.ID.String()] = u.Email
		}
	}
	out := make([]adminTeamMemberView, 0, len(members))
	for _, m := range members {
		role := m.Role
		if role == "" {
			role = roleMember
		}
		out = append(out, adminTeamMemberView{UserID: m.UserID.String(), Email: emailByID[m.UserID.String()], Role: role})
	}
	return out
}
