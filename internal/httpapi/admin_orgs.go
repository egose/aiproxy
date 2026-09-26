package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/egose/aiproxy/internal/adminauth"
	"github.com/egose/aiproxy/internal/store"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type adminOrgView struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"display_name,omitempty"`
	IsSystem    bool   `json:"is_system"`
	Role        string `json:"role,omitempty"`
}

type adminTeamView struct {
	ID          string `json:"id"`
	OrgID       string `json:"org_id"`
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

func (h *Handler) requireOrgMember(deps Dependencies, w http.ResponseWriter, r *http.Request, orgID uuid.UUID) (store.Organization, *adminauth.Claims, bool) {
	claims, ok := h.adminClaims(deps, r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return store.Organization{}, nil, false
	}
	org, err := deps.AdminStore.GetOrganization(r.Context(), orgID)
	if err != nil {
		http.Error(w, "organization not found", http.StatusNotFound)
		return store.Organization{}, nil, false
	}
	if claims.IsAdmin {
		return org, claims, true
	}
	if org.IsSystem {
		http.Error(w, "organization not found", http.StatusNotFound)
		return store.Organization{}, nil, false
	}
	if _, err := deps.AdminStore.GetMembership(r.Context(), mustParseUUID(claims.Subject), orgID); err != nil {
		http.Error(w, "organization not found", http.StatusNotFound)
		return store.Organization{}, nil, false
	}
	return org, claims, true
}

func (h *Handler) requireOrgAdmin(deps Dependencies, w http.ResponseWriter, r *http.Request, orgID uuid.UUID) (store.Organization, *adminauth.Claims, bool) {
	claims, ok := h.adminClaims(deps, r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return store.Organization{}, nil, false
	}
	org, err := deps.AdminStore.GetOrganization(r.Context(), orgID)
	if err != nil {
		http.Error(w, "organization not found", http.StatusNotFound)
		return store.Organization{}, nil, false
	}
	if claims.IsAdmin {
		return org, claims, true
	}
	if org.IsSystem {
		http.Error(w, "organization not found", http.StatusNotFound)
		return store.Organization{}, nil, false
	}
	membership, err := deps.AdminStore.GetMembership(r.Context(), mustParseUUID(claims.Subject), orgID)
	if err != nil {
		http.Error(w, "organization not found", http.StatusNotFound)
		return store.Organization{}, nil, false
	}
	if membership.Role != roleAdmin {
		http.Error(w, "organization admin required", http.StatusForbidden)
		return store.Organization{}, nil, false
	}
	return org, claims, true
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

func (h *Handler) requireTeamManager(deps Dependencies, w http.ResponseWriter, r *http.Request, orgID, teamID uuid.UUID) (store.Organization, *adminauth.Claims, store.OrganizationTeam, bool) {
	claims, ok := h.adminClaims(deps, r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return store.Organization{}, nil, store.OrganizationTeam{}, false
	}
	org, err := deps.AdminStore.GetOrganization(r.Context(), orgID)
	if err != nil {
		http.Error(w, "organization not found", http.StatusNotFound)
		return store.Organization{}, nil, store.OrganizationTeam{}, false
	}
	if claims.IsAdmin {
		team, err := deps.AdminStore.GetTeam(r.Context(), teamID)
		if err != nil || team.OrgID != orgID {
			http.Error(w, "team not found", http.StatusNotFound)
			return store.Organization{}, nil, store.OrganizationTeam{}, false
		}
		return org, claims, team, true
	}
	if org.IsSystem {
		http.Error(w, "organization not found", http.StatusNotFound)
		return store.Organization{}, nil, store.OrganizationTeam{}, false
	}
	membership, err := deps.AdminStore.GetMembership(r.Context(), mustParseUUID(claims.Subject), orgID)
	if err != nil {
		http.Error(w, "organization not found", http.StatusNotFound)
		return store.Organization{}, nil, store.OrganizationTeam{}, false
	}
	team, err := deps.AdminStore.GetTeam(r.Context(), teamID)
	if err != nil || team.OrgID != orgID {
		http.Error(w, "team not found", http.StatusNotFound)
		return store.Organization{}, nil, store.OrganizationTeam{}, false
	}
	if membership.Role == roleAdmin || h.isTeamAdmin(deps, r.Context(), claims, teamID) {
		return org, claims, team, true
	}
	http.Error(w, "team admin required", http.StatusForbidden)
	return store.Organization{}, nil, store.OrganizationTeam{}, false
}

func (h *Handler) callerOrgs(deps Dependencies, r *http.Request, claims *adminauth.Claims) ([]store.Organization, error) {
	if claims.IsAdmin {
		return deps.AdminStore.ListOrganizations(r.Context())
	}
	memberships, err := deps.AdminStore.ListMembershipsByUser(r.Context(), mustParseUUID(claims.Subject))
	if err != nil {
		return nil, err
	}
	out := make([]store.Organization, 0, len(memberships))
	for _, m := range memberships {
		org, err := deps.AdminStore.GetOrganization(r.Context(), m.OrgID)
		if err != nil {
			continue
		}
		out = append(out, org)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (h *Handler) adminOrgs(deps Dependencies, w http.ResponseWriter, r *http.Request, rest []string) {
	if len(rest) == 0 || rest[0] == "" {
		switch r.Method {
		case http.MethodGet:
			claims, ok := h.adminClaims(deps, r)
			if !ok {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			orgs, err := h.callerOrgs(deps, r, claims)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			roleByOrg := map[string]string{}
			if !claims.IsAdmin {
				memberships, err := deps.AdminStore.ListMembershipsByUser(r.Context(), mustParseUUID(claims.Subject))
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				for _, m := range memberships {
					roleByOrg[m.OrgID.String()] = m.Role
				}
			}
			views := make([]adminOrgView, 0, len(orgs))
			for _, org := range orgs {
				views = append(views, adminOrgView{ID: org.ID.String(), Name: org.Name, DisplayName: org.DisplayName, IsSystem: org.IsSystem, Role: roleByOrg[org.ID.String()]})
			}
			writeAdminJSON(w, http.StatusOK, map[string]interface{}{"organizations": views})
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
				http.Error(w, "invalid organization name", http.StatusBadRequest)
				return
			}
			org := &store.Organization{Name: name, DisplayName: strings.TrimSpace(req.DisplayName)}
			if err := deps.AdminStore.CreateOrganization(r.Context(), org); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if err := deps.AdminStore.UpsertMembership(r.Context(), &store.OrganizationMember{UserID: mustParseUUID(claims.Subject), OrgID: org.ID, Role: roleAdmin}); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeAdminJSON(w, http.StatusCreated, adminOrgView{ID: org.ID.String(), Name: org.Name, DisplayName: org.DisplayName, Role: roleAdmin})
			return
		}
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	orgID, err := uuid.Parse(rest[0])
	if err != nil {
		http.Error(w, "invalid organization id", http.StatusBadRequest)
		return
	}
	if len(rest) == 1 {
		switch r.Method {
		case http.MethodGet:
			org, _, ok := h.requireOrgMember(deps, w, r, orgID)
			if !ok {
				return
			}
			writeAdminJSON(w, http.StatusOK, h.orgDetailView(deps, r, org))
			return
		case http.MethodPut:
			org, _, ok := h.requireOrgAdmin(deps, w, r, orgID)
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
				org.DisplayName = strings.TrimSpace(*req.DisplayName)
			}
			if err := deps.AdminStore.UpdateOrganization(r.Context(), &org); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeAdminJSON(w, http.StatusOK, h.orgDetailView(deps, r, org))
			return
		case http.MethodDelete:
			org, claims, ok := h.requireOrgAdmin(deps, w, r, orgID)
			if !ok {
				return
			}
			if org.IsSystem {
				http.Error(w, "system organization cannot be deleted", http.StatusBadRequest)
				return
			}
			if n, err := deps.AdminStore.CountProvidersByOrg(r.Context(), orgID); err != nil || n > 0 {
				if err == nil {
					http.Error(w, "organization still owns providers", http.StatusBadRequest)
					return
				}
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if n, err := deps.AdminStore.CountAliasesByOrg(r.Context(), orgID); err != nil || n > 0 {
				if err == nil {
					http.Error(w, "organization still owns aliases", http.StatusBadRequest)
					return
				}
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if n, err := deps.AdminStore.CountKeysByOrg(r.Context(), orgID); err != nil || n > 0 {
				if err == nil {
					http.Error(w, "organization still owns api keys", http.StatusBadRequest)
					return
				}
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			members, err := deps.AdminStore.ListMembershipsByOrg(r.Context(), orgID)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			for _, m := range members {
				if m.UserID.String() != claims.Subject {
					http.Error(w, "organization still has members", http.StatusBadRequest)
					return
				}
			}
			if err := deps.AdminStore.DeleteOrganization(r.Context(), orgID); err != nil {
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
		h.adminOrgTeams(deps, w, r, orgID, rest[2:])
		return
	case "members":
		h.adminOrgMembers(deps, w, r, orgID, rest[2:])
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
		h.adminUserQuota(deps, w, r, orgID, userID, rest[3:])
		return
	}
	http.Error(w, "unknown admin endpoint", http.StatusNotFound)
}

type adminOrgMemberView struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
	Role   string `json:"role"`
}

func (h *Handler) orgDetailView(deps Dependencies, r *http.Request, org store.Organization) map[string]interface{} {
	ctx := r.Context()
	teams, _ := deps.AdminStore.ListTeamsByOrg(ctx, org.ID)
	members, _ := deps.AdminStore.ListMembershipsByOrg(ctx, org.ID)
	users, _ := deps.AdminStore.ListUsers(ctx)
	emailByID := make(map[string]string, len(users))
	for _, u := range users {
		emailByID[u.ID.String()] = u.Email
	}
	teamViews := make([]adminTeamView, 0, len(teams))
	for _, t := range teams {
		teamViews = append(teamViews, adminTeamView{ID: t.ID.String(), OrgID: t.OrgID.String(), Name: t.Name, Description: t.Description})
	}
	memberViews := make([]adminOrgMemberView, 0, len(members))
	for _, m := range members {
		memberViews = append(memberViews, adminOrgMemberView{UserID: m.UserID.String(), Email: emailByID[m.UserID.String()], Role: m.Role})
	}
	providers, _ := deps.AdminStore.CountProvidersByOrg(ctx, org.ID)
	aliases, _ := deps.AdminStore.CountAliasesByOrg(ctx, org.ID)
	keys, _ := deps.AdminStore.CountKeysByOrg(ctx, org.ID)
	return map[string]interface{}{
		"id": org.ID.String(), "name": org.Name, "display_name": org.DisplayName, "is_system": org.IsSystem,
		"teams": teamViews, "members": memberViews,
		"resources": map[string]int{"providers": providers, "aliases": aliases, "keys": keys},
	}
}

func (h *Handler) adminOrgTeams(deps Dependencies, w http.ResponseWriter, r *http.Request, orgID uuid.UUID, rest []string) {
	ctx := r.Context()
	if len(rest) == 0 || rest[0] == "" {
		switch r.Method {
		case http.MethodGet:
			org, claims, ok := h.requireOrgMember(deps, w, r, orgID)
			if !ok {
				return
			}
			_ = org
			teams, err := deps.AdminStore.ListTeamsByOrg(ctx, orgID)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			roles := h.callerTeamRoles(deps, ctx, claims)
			views := make([]adminTeamView, 0, len(teams))
			for _, t := range teams {
				views = append(views, adminTeamView{ID: t.ID.String(), OrgID: t.OrgID.String(), Name: t.Name, Description: t.Description, MyRole: roles[t.ID.String()]})
			}
			writeAdminJSON(w, http.StatusOK, map[string]interface{}{"teams": views})
			return
		case http.MethodPost:
			if _, _, ok := h.requireOrgAdmin(deps, w, r, orgID); !ok {
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
			team := &store.OrganizationTeam{OrgID: orgID, Name: name, Description: strings.TrimSpace(req.Description)}
			if err := deps.AdminStore.CreateTeam(ctx, team); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			writeAdminJSON(w, http.StatusCreated, adminTeamView{ID: team.ID.String(), OrgID: team.OrgID.String(), Name: team.Name, Description: team.Description})
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
	if err != nil || team.OrgID != orgID {
		http.Error(w, "team not found", http.StatusNotFound)
		return
	}
	if len(rest) == 2 && rest[1] == "quota" {
		h.adminTeamQuota(deps, w, r, orgID, teamID)
		return
	}
	if len(rest) == 1 {
		switch r.Method {
		case http.MethodGet:
			if _, _, ok := h.requireOrgMember(deps, w, r, orgID); !ok {
				return
			}
			members, err := deps.AdminStore.ListTeamMembers(ctx, teamID)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeAdminJSON(w, http.StatusOK, map[string]interface{}{
				"team":    adminTeamView{ID: team.ID.String(), OrgID: team.OrgID.String(), Name: team.Name, Description: team.Description},
				"members": h.teamMemberViews(deps, r, members),
			})
			return
		case http.MethodPut:
			if _, _, ok := h.requireOrgAdmin(deps, w, r, orgID); !ok {
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
			writeAdminJSON(w, http.StatusOK, adminTeamView{ID: team.ID.String(), OrgID: team.OrgID.String(), Name: team.Name, Description: team.Description})
			return
		case http.MethodDelete:
			if _, _, ok := h.requireOrgAdmin(deps, w, r, orgID); !ok {
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
			if _, _, ok := h.requireOrgMember(deps, w, r, orgID); !ok {
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
			if _, _, _, ok := h.requireTeamManager(deps, w, r, orgID, teamID); !ok {
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
			userID, ok := h.resolveMemberUser(deps, w, r, orgID, req.UserID, req.Email)
			if !ok {
				return
			}
			if err := deps.AdminStore.AddTeamMember(ctx, userID, teamID, role); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
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
			_, claims, _, ok := h.requireTeamManager(deps, w, r, orgID, teamID)
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
			current, err := deps.AdminStore.GetTeamMember(ctx, userID, teamID)
			if err != nil {
				http.Error(w, "team membership not found", http.StatusNotFound)
				return
			}
			currentRole := current.Role
			if currentRole == "" {
				currentRole = roleMember
			}
			if role != roleAdmin && currentRole == roleAdmin {
				if n, err := deps.AdminStore.CountTeamAdmins(ctx, teamID); err != nil || n <= 1 {
					if err == nil {
						http.Error(w, "cannot demote the last team admin", http.StatusBadRequest)
						return
					}
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
			}
			if err := deps.AdminStore.SetTeamMemberRole(ctx, userID, teamID, role); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeAdminJSON(w, http.StatusOK, map[string]bool{"ok": true})
			return
		case http.MethodDelete:
			if _, _, _, ok := h.requireTeamManager(deps, w, r, orgID, teamID); !ok {
				return
			}
			current, err := deps.AdminStore.GetTeamMember(ctx, userID, teamID)
			if err != nil {
				http.Error(w, "team membership not found", http.StatusNotFound)
				return
			}
			currentRole := current.Role
			if currentRole == "" {
				currentRole = roleMember
			}
			if currentRole == roleAdmin {
				if n, err := deps.AdminStore.CountTeamAdmins(ctx, teamID); err != nil || n <= 1 {
					if err == nil {
						http.Error(w, "cannot remove the last team admin", http.StatusBadRequest)
						return
					}
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
			}
			if err := deps.AdminStore.RemoveTeamMember(ctx, userID, teamID); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
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

func (h *Handler) resolveMemberUser(deps Dependencies, w http.ResponseWriter, r *http.Request, orgID uuid.UUID, userID, email string) (uuid.UUID, bool) {
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
	if _, err := deps.AdminStore.GetMembership(ctx, id, orgID); err != nil {
		http.Error(w, "user is not an organization member", http.StatusBadRequest)
		return uuid.Nil, false
	}
	return id, true
}

func (h *Handler) adminOrgMembers(deps Dependencies, w http.ResponseWriter, r *http.Request, orgID uuid.UUID, rest []string) {
	ctx := r.Context()
	if len(rest) == 0 || rest[0] == "" {
		switch r.Method {
		case http.MethodGet:
			if _, _, ok := h.requireOrgMember(deps, w, r, orgID); !ok {
				return
			}
			members, err := deps.AdminStore.ListMembershipsByOrg(ctx, orgID)
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
			views := make([]adminOrgMemberView, 0, len(members))
			for _, m := range members {
				views = append(views, adminOrgMemberView{UserID: m.UserID.String(), Email: emailByID[m.UserID.String()], Role: m.Role})
			}
			sort.Slice(views, func(i, j int) bool { return views[i].Email < views[j].Email })
			writeAdminJSON(w, http.StatusOK, map[string]interface{}{"members": views})
			return
		case http.MethodPost:
			org, claims, ok := h.requireOrgAdmin(deps, w, r, orgID)
			if !ok {
				return
			}
			_ = org
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
			if err := deps.AdminStore.UpsertMembership(ctx, &store.OrganizationMember{UserID: userID, OrgID: orgID, Role: role}); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
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
		_, claims, ok := h.requireOrgAdmin(deps, w, r, orgID)
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
		membership, err := deps.AdminStore.GetMembership(ctx, memberID, orgID)
		if err != nil {
			http.Error(w, "membership not found", http.StatusNotFound)
			return
		}
		if role != roleAdmin && membership.Role == roleAdmin {
			if memberID.String() == claims.Subject {
				http.Error(w, "cannot demote your own organization admin role", http.StatusBadRequest)
				return
			}
			if n, err := deps.AdminStore.CountOrgAdmins(ctx, orgID); err != nil || n <= 1 {
				if err == nil {
					http.Error(w, "cannot demote the last organization admin", http.StatusBadRequest)
					return
				}
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		membership.Role = role
		if err := deps.AdminStore.UpsertMembership(ctx, &membership); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeAdminJSON(w, http.StatusOK, map[string]bool{"ok": true})
	case http.MethodDelete:
		_, claims, ok := h.requireOrgAdmin(deps, w, r, orgID)
		if !ok {
			return
		}
		membership, err := deps.AdminStore.GetMembership(ctx, memberID, orgID)
		if err != nil {
			http.Error(w, "membership not found", http.StatusNotFound)
			return
		}
		if membership.Role == roleAdmin {
			if n, err := deps.AdminStore.CountOrgAdmins(ctx, orgID); err != nil || n <= 1 {
				if err == nil {
					http.Error(w, "cannot remove the last organization admin", http.StatusBadRequest)
					return
				}
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		_ = claims
		if err := deps.AdminStore.DeleteMembership(ctx, memberID, orgID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
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
		Email       string `json:"email"`
		Password    string `json:"password"`
		OrgName     string `json:"org_name"`
		DisplayName string `json:"display_name"`
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
	orgName := strings.ToLower(strings.TrimSpace(req.OrgName))
	if orgName == "" {
		orgName = deriveOrgName(email)
	}
	if !validResourceName(orgName) {
		http.Error(w, "invalid organization name", http.StatusBadRequest)
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	u := &store.User{Email: email, PasswordHash: string(hash)}
	org := &store.Organization{Name: orgName, DisplayName: strings.TrimSpace(req.DisplayName)}
	if err := deps.AdminStore.CreateUserWithOrg(ctx, u, org, roleAdmin); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeAdminJSON(w, http.StatusCreated, map[string]interface{}{
		"user":         adminUserView{ID: u.ID.String(), Email: u.Email, Role: roleUser, Source: "database"},
		"organization": adminOrgView{ID: org.ID.String(), Name: org.Name, DisplayName: org.DisplayName, Role: roleAdmin},
	})
}

func deriveOrgName(email string) string {
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
		name = "org"
	}
	if len(name) > 56 {
		name = name[:56]
	}
	return name + "-org"
}

func (h *Handler) ensurePersonalOrg(ctx context.Context, deps Dependencies, userID uuid.UUID, email string) error {
	memberships, err := deps.AdminStore.ListMembershipsByUser(ctx, userID)
	if err != nil {
		return err
	}
	if len(memberships) > 0 {
		return nil
	}
	base := deriveOrgName(email)
	for i := 0; i < 100; i++ {
		name := base
		if i > 0 {
			name = fmt.Sprintf("%.52s-%d", strings.TrimSuffix(base, "-org"), i+1)
		}
		org := &store.Organization{Name: name}
		if err := deps.AdminStore.CreateOrganization(ctx, org); err != nil {
			continue
		}
		return deps.AdminStore.UpsertMembership(ctx, &store.OrganizationMember{UserID: userID, OrgID: org.ID, Role: roleAdmin})
	}
	return errBad("could not allocate a personal organization")
}

func (h *Handler) requestOrgID(r *http.Request, orgIDStr string) string {
	if strings.TrimSpace(orgIDStr) != "" {
		return strings.TrimSpace(orgIDStr)
	}
	return strings.TrimSpace(r.Header.Get("X-Org-ID"))
}

func (h *Handler) resolveWriteOrg(deps Dependencies, w http.ResponseWriter, r *http.Request, claims *adminauth.Claims, orgIDStr string) (store.Organization, bool) {
	ctx := r.Context()
	orgIDStr = h.requestOrgID(r, orgIDStr)
	if orgIDStr != "" {
		orgID, err := uuid.Parse(strings.TrimSpace(orgIDStr))
		if err != nil {
			http.Error(w, "invalid organization id", http.StatusBadRequest)
			return store.Organization{}, false
		}
		if _, _, ok := h.requireOrgAdmin(deps, w, r, orgID); !ok {
			return store.Organization{}, false
		}
		org, err := deps.AdminStore.GetOrganization(ctx, orgID)
		if err != nil {
			http.Error(w, "organization not found", http.StatusNotFound)
			return store.Organization{}, false
		}
		return org, true
	}
	memberships, err := deps.AdminStore.ListMembershipsByUser(ctx, mustParseUUID(claims.Subject))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return store.Organization{}, false
	}
	if claims.IsAdmin && len(memberships) == 0 {
		org, err := deps.AdminStore.GetOrganization(ctx, store.SystemOrgID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return store.Organization{}, false
		}
		return org, true
	}
	adminOf := []store.OrganizationMember{}
	for _, m := range memberships {
		if m.Role != roleAdmin {
			continue
		}
		org, err := deps.AdminStore.GetOrganization(ctx, m.OrgID)
		if err != nil || org.IsSystem {
			continue
		}
		adminOf = append(adminOf, m)
	}
	if len(adminOf) == 1 {
		org, err := deps.AdminStore.GetOrganization(ctx, adminOf[0].OrgID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return store.Organization{}, false
		}
		return org, true
	}
	http.Error(w, "specify organization id", http.StatusBadRequest)
	return store.Organization{}, false
}

func (h *Handler) visibleOrgIDs(deps Dependencies, r *http.Request, claims *adminauth.Claims) (map[string]bool, bool) {
	if claims.IsAdmin {
		return nil, true
	}
	memberships, err := deps.AdminStore.ListMembershipsByUser(r.Context(), mustParseUUID(claims.Subject))
	if err != nil {
		return nil, false
	}
	out := make(map[string]bool, len(memberships))
	for _, m := range memberships {
		out[m.OrgID.String()] = true
	}
	return out, true
}

func (h *Handler) canSeeSystemOrg(deps Dependencies, r *http.Request, claims *adminauth.Claims, filterOrg string) bool {
	if filterOrg != "" && filterOrg != store.SystemOrgID.String() {
		return false
	}
	visible, _ := h.visibleOrgIDs(deps, r, claims)
	return visible == nil || visible[store.SystemOrgID.String()]
}

func (h *Handler) orgNameMap(deps Dependencies, r *http.Request, claims *adminauth.Claims) map[string]string {
	out := map[string]string{}
	var orgs []store.Organization
	var err error
	if claims.IsAdmin {
		orgs, err = deps.AdminStore.ListOrganizations(r.Context())
	} else {
		orgs, err = h.callerOrgs(deps, r, claims)
	}
	if err != nil {
		return out
	}
	for _, org := range orgs {
		out[org.ID.String()] = org.Name
	}
	return out
}

func (h *Handler) resolveOrgFilter(deps Dependencies, w http.ResponseWriter, r *http.Request, claims *adminauth.Claims, queryOrg string) (string, bool) {
	queryOrg = h.requestOrgID(r, queryOrg)
	if queryOrg == "" {
		return "", true
	}
	if _, err := uuid.Parse(queryOrg); err != nil {
		http.Error(w, "invalid organization id", http.StatusBadRequest)
		return "", false
	}
	visible, _ := h.visibleOrgIDs(deps, r, claims)
	if visible != nil && !visible[queryOrg] {
		http.Error(w, "organization not found", http.StatusNotFound)
		return "", false
	}
	if _, err := deps.AdminStore.GetOrganization(r.Context(), mustParseUUID(queryOrg)); err != nil {
		http.Error(w, "organization not found", http.StatusNotFound)
		return "", false
	}
	return queryOrg, true
}

func (h *Handler) requireResourceOrg(deps Dependencies, w http.ResponseWriter, r *http.Request, claims *adminauth.Claims, orgID uuid.UUID) bool {
	if h.canWriteOrg(deps, r.Context(), claims, orgID) {
		return true
	}
	if claims.IsAdmin {
		http.Error(w, "organization not found", http.StatusNotFound)
		return false
	}
	org, err := deps.AdminStore.GetOrganization(r.Context(), orgID)
	if err != nil || org.IsSystem {
		http.Error(w, "organization not found", http.StatusNotFound)
		return false
	}
	http.Error(w, "organization admin required", http.StatusForbidden)
	return false
}

func (h *Handler) canWriteOrg(deps Dependencies, ctx context.Context, claims *adminauth.Claims, orgID uuid.UUID) bool {
	if claims.IsAdmin {
		return true
	}
	org, err := deps.AdminStore.GetOrganization(ctx, orgID)
	if err != nil || org.IsSystem {
		return false
	}
	membership, err := deps.AdminStore.GetMembership(ctx, mustParseUUID(claims.Subject), orgID)
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
