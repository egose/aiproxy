package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/egose/aiproxy/internal/adminauth"
	"github.com/egose/aiproxy/internal/store"
	"github.com/google/uuid"
)

type adminKeyUsageView struct {
	Requests int64 `json:"requests"`
	Tokens   int64 `json:"tokens"`
	Spend    int64 `json:"spend_micros"`
}

type adminKeyView struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	TokenPrefix   string            `json:"token_prefix"`
	Tenant        string            `json:"tenant"`
	AllowedModels []string          `json:"allowed_models"`
	Enabled       bool              `json:"enabled"`
	Source        string            `json:"source"`
	OrgID         string            `json:"org_id,omitempty"`
	OrgName       string            `json:"org_name,omitempty"`
	Description   string            `json:"description,omitempty"`
	ExpiresAt     string            `json:"expires_at,omitempty"`
	UserIDs       []string          `json:"user_ids,omitempty"`
	TeamIDs       []string          `json:"team_ids,omitempty"`
	OwnerUserID   string            `json:"owner_user_id,omitempty"`
	OwnerTeamID   string            `json:"owner_team_id,omitempty"`
	OwnerName     string            `json:"owner_name,omitempty"`
	CanManage     bool              `json:"can_manage"`
	Usage         adminKeyUsageView `json:"usage"`
	Quota         *adminQuotaView   `json:"quota,omitempty"`
}

type adminKeyMemberView struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	TokenPrefix   string            `json:"token_prefix"`
	AllowedModels []string          `json:"allowed_models"`
	Source        string            `json:"source"`
	OrgID         string            `json:"org_id,omitempty"`
	OrgName       string            `json:"org_name,omitempty"`
	Description   string            `json:"description,omitempty"`
	ExpiresAt     string            `json:"expires_at,omitempty"`
	OwnerUserID   string            `json:"owner_user_id,omitempty"`
	OwnerTeamID   string            `json:"owner_team_id,omitempty"`
	OwnerName     string            `json:"owner_name,omitempty"`
	Usage         adminKeyUsageView `json:"usage"`
	Quota         *adminQuotaView   `json:"quota,omitempty"`
}

func keyOwnerScope(k store.InboundKey) (quotaScope, bool) {
	if k.OwnerUserID != nil {
		return quotaScope{typ: quotaScopeUser, id: *k.OwnerUserID}, true
	}
	if k.OwnerTeamID != nil {
		return quotaScope{typ: quotaScopeTeam, id: *k.OwnerTeamID}, true
	}
	return quotaScope{}, false
}

func (h *Handler) adminKeys(deps Dependencies, w http.ResponseWriter, r *http.Request, rest []string) {
	claims, ok := h.adminClaims(deps, r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	ctx := r.Context()
	if len(rest) == 0 || rest[0] == "" {
		switch r.Method {
		case http.MethodGet:
			filterOrg, ok := h.resolveOrgFilter(deps, w, r, claims, r.URL.Query().Get("org_id"))
			if !ok {
				return
			}
			views, err := h.mergedKeyViews(ctx, deps, r, claims, filterOrg)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeAdminJSON(w, http.StatusOK, map[string]interface{}{"keys": views})
			return
		case http.MethodPost:
			var req struct {
				Name          string   `json:"name"`
				Tenant        string   `json:"tenant"`
				AllowedModels []string `json:"allowed_models"`
				OrgID         string   `json:"org_id"`
				Description   string   `json:"description"`
				ExpiresAt     string   `json:"expires_at"`
				UserIDs       []string `json:"user_ids"`
				TeamIDs       []string `json:"team_ids"`
				OwnerType     string   `json:"owner_type"`
				OwnerID       string   `json:"owner_id"`
			}
			if err := json.NewDecoder(ioLimitReader(r)).Decode(&req); err != nil {
				http.Error(w, "invalid body", http.StatusBadRequest)
				return
			}
			org, ok := h.resolveKeyWriteOrg(deps, w, r, claims, req.OrgID)
			if !ok {
				return
			}
			owner, ok := h.resolveKeyOwner(deps, w, r, claims, org, req.OwnerType, req.OwnerID)
			if !ok {
				return
			}
			userIDs := req.UserIDs
			teamIDs := req.TeamIDs
			if owner.userID != nil {
				userIDs = appendUUIDString(userIDs, owner.userID.String())
			}
			if owner.teamID != nil {
				teamIDs = appendUUIDString(teamIDs, owner.teamID.String())
			}
			expires, ok := h.parseKeyExpiry(w, req.ExpiresAt)
			if !ok {
				return
			}
			boundUsers, boundTeams, ok := h.resolveKeyBindings(deps, w, r, org, userIDs, teamIDs)
			if !ok {
				return
			}
			view, token, err := h.createDBKey(ctx, deps, org, req.Name, req.Tenant, req.AllowedModels, req.Description, expires, owner)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if err := deps.AdminStore.SetKeyBindings(ctx, mustParseUUID(view.ID), boundUsers, boundTeams); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			view, err = h.fullKeyView(ctx, deps, r, claims, mustParseUUID(view.ID))
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if !h.activateChange(deps, w) {
				return
			}
			writeAdminJSON(w, http.StatusCreated, map[string]interface{}{"key": view, "token": token})
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
	switch action {
	case "rotate":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		target, err := h.findInboundKey(ctx, deps, id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if !h.canManageKey(deps, ctx, claims, *target) {
			http.Error(w, "key manager required", http.StatusForbidden)
			return
		}
		token, err := h.rotateDBKey(ctx, deps, target)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if !h.activateChange(deps, w) {
			return
		}
		writeAdminJSON(w, http.StatusOK, map[string]interface{}{"token": token})
	case "revoke":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		target, err := h.findInboundKey(ctx, deps, id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if !h.canManageKey(deps, ctx, claims, *target) {
			http.Error(w, "key manager required", http.StatusForbidden)
			return
		}
		if err := h.setDBKeyEnabled(ctx, deps, target, false); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if !h.activateChange(deps, w) {
			return
		}
		writeAdminJSON(w, http.StatusOK, map[string]bool{"ok": true})
	case "":
		rows, err := deps.AdminStore.ListInboundKeys(ctx)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		var target *store.InboundKey
		for i := range rows {
			if rows[i].ID == id {
				target = &rows[i]
			}
		}
		if target == nil {
			http.Error(w, "key not found", http.StatusNotFound)
			return
		}
		switch r.Method {
		case http.MethodGet:
			view, ok := h.keyDetailView(ctx, deps, r, claims, *target)
			if !ok {
				http.Error(w, "key not found", http.StatusNotFound)
				return
			}
			writeAdminJSON(w, http.StatusOK, map[string]interface{}{"key": view})
		case http.MethodPut:
			if !h.canManageKey(deps, ctx, claims, *target) {
				http.Error(w, "key manager required", http.StatusForbidden)
				return
			}
			var req struct {
				Description   *string  `json:"description"`
				ExpiresAt     *string  `json:"expires_at"`
				Tenant        *string  `json:"tenant"`
				AllowedModels []string `json:"allowed_models"`
				UserIDs       []string `json:"user_ids"`
				TeamIDs       []string `json:"team_ids"`
			}
			if err := json.NewDecoder(ioLimitReader(r)).Decode(&req); err != nil {
				http.Error(w, "invalid body", http.StatusBadRequest)
				return
			}
			org, err := deps.AdminStore.GetOrganization(ctx, target.OrgID)
			if err != nil {
				http.Error(w, "organization not found", http.StatusNotFound)
				return
			}
			if req.Description != nil {
				target.Description = strings.TrimSpace(*req.Description)
			}
			if req.ExpiresAt != nil {
				expires, ok := h.parseKeyExpiry(w, *req.ExpiresAt)
				if !ok {
					return
				}
				target.ExpiresAt = expires
			}
			if req.Tenant != nil {
				target.Tenant = strings.TrimSpace(*req.Tenant)
			}
			if req.AllowedModels != nil {
				target.AllowedModels = req.AllowedModels
			}
			if err := deps.AdminStore.UpdateInboundKey(ctx, target); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if req.UserIDs != nil || req.TeamIDs != nil {
				boundUsers, boundTeams, ok := h.resolveKeyBindings(deps, w, r, org, req.UserIDs, req.TeamIDs)
				if !ok {
					return
				}
				if err := deps.AdminStore.SetKeyBindings(ctx, target.ID, boundUsers, boundTeams); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
			}
			view, err := h.fullKeyView(ctx, deps, r, claims, target.ID)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if !h.activateChange(deps, w) {
				return
			}
			writeAdminJSON(w, http.StatusOK, map[string]interface{}{"key": view})
		case http.MethodDelete:
			if !h.canManageKey(deps, ctx, claims, *target) {
				http.Error(w, "key manager required", http.StatusForbidden)
				return
			}
			if err := deps.AdminStore.DeleteInboundKey(ctx, id); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if !h.activateChange(deps, w) {
				return
			}
			writeAdminJSON(w, http.StatusOK, map[string]bool{"ok": true})
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	default:
		http.Error(w, "unknown action", http.StatusNotFound)
	}
}

func (h *Handler) mergedKeyViews(ctx context.Context, deps Dependencies, r *http.Request, claims *adminauth.Claims, filterOrg string) ([]interface{}, error) {
	visible, _ := h.visibleOrgIDs(deps, r, claims)
	names := h.orgNameMap(deps, r, claims)
	views := []interface{}{}
	if h.canSeeSystemOrg(deps, r, claims, filterOrg) {
		for name := range deps.AdminAuthConfig.Clients {
			views = append(views, adminKeyView{Name: name, Tenant: deps.AdminAuthConfig.Clients[name].Tenant, AllowedModels: deps.AdminAuthConfig.Clients[name].AllowedModels, Enabled: true, Source: "config"})
		}
	}
	adminOrgs := map[string]bool{}
	sharedKeys := map[string]bool{}
	callerTeams := map[string]bool{}
	caller := mustParseUUID(claims.Subject)
	if !claims.IsAdmin {
		memberships, _ := deps.AdminStore.ListMembershipsByUser(ctx, caller)
		for _, m := range memberships {
			if m.Role == roleAdmin {
				adminOrgs[m.OrgID.String()] = true
			}
		}
		shared, _ := deps.AdminStore.ListInboundKeysVisibleToMember(ctx, caller)
		for _, k := range shared {
			sharedKeys[k.ID.String()] = true
		}
		adminTeams, _ := deps.AdminStore.ListTeamAdminTeamIDs(ctx, caller)
		for _, tid := range adminTeams {
			callerTeams[tid.String()] = true
		}
	}
	rows, err := deps.AdminStore.ListInboundKeys(ctx)
	if err != nil {
		return nil, err
	}
	emails, teamNames := h.keyOwnerNames(deps, ctx, rows)
	usageByKey := map[string]adminKeyUsageView{}
	usageOrgs := map[string]bool{}
	for _, k := range rows {
		usageOrgs[k.OrgID.String()] = true
	}
	for orgStr := range usageOrgs {
		if used, err := deps.AdminStore.KeyUsageByOrg(ctx, mustParseUUID(orgStr)); err == nil {
			for _, u := range used {
				usageByKey[u.KeyID.String()] = adminKeyUsageView{Requests: u.Requests, Tokens: u.Tokens, Spend: u.Spend}
			}
		}
	}
	keyQuota := func(k store.InboundKey, manageable bool) *adminQuotaView {
		scope, ok := keyOwnerScope(k)
		if !ok {
			return nil
		}
		if !manageable && scope.typ == quotaScopeUser {
			return nil
		}
		if view, err := h.quotaView(deps, ctx, k.OrgID, scope); err == nil {
			return &view
		}
		return nil
	}
	for _, k := range rows {
		if filterOrg != "" && k.OrgID.String() != filterOrg {
			continue
		}
		if visible != nil && !visible[k.OrgID.String()] {
			continue
		}
		manageable := claims.IsAdmin || adminOrgs[k.OrgID.String()] ||
			(k.OwnerUserID != nil && *k.OwnerUserID == caller) ||
			(k.OwnerTeamID != nil && callerTeams[k.OwnerTeamID.String()])
		if manageable {
			full, err := h.buildFullKeyView(ctx, deps, names, emails, teamNames, k, true, usageByKey[k.ID.String()], keyQuota(k, true))
			if err != nil {
				return nil, err
			}
			views = append(views, full)
			continue
		}
		if sharedKeys[k.ID.String()] {
			views = append(views, h.buildMemberKeyView(names, emails, teamNames, k, usageByKey[k.ID.String()], keyQuota(k, false)))
		}
	}
	sort.Slice(views, func(i, j int) bool { return keyViewName(views[i]) < keyViewName(views[j]) })
	return views, nil
}

func keyViewName(v interface{}) string {
	if full, ok := v.(adminKeyView); ok {
		return full.Name
	}
	if member, ok := v.(adminKeyMemberView); ok {
		return member.Name
	}
	return ""
}

func formatKeyExpiry(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func (h *Handler) parseKeyExpiry(w http.ResponseWriter, raw string) (*time.Time, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, true
	}
	parsed, err := time.Parse(time.RFC3339, trimmed)
	if err != nil {
		http.Error(w, "invalid expires_at, use RFC3339", http.StatusBadRequest)
		return nil, false
	}
	utc := parsed.UTC()
	return &utc, true
}

func (h *Handler) resolveKeyBindings(deps Dependencies, w http.ResponseWriter, r *http.Request, org store.Organization, userIDs, teamIDs []string) ([]uuid.UUID, []uuid.UUID, bool) {
	ctx := r.Context()
	boundUsers := make([]uuid.UUID, 0, len(userIDs))
	for _, raw := range userIDs {
		uid, err := uuid.Parse(strings.TrimSpace(raw))
		if err != nil {
			http.Error(w, "invalid user id", http.StatusBadRequest)
			return nil, nil, false
		}
		if _, err := deps.AdminStore.GetMembership(ctx, uid, org.ID); err != nil {
			http.Error(w, "user is not an organization member", http.StatusBadRequest)
			return nil, nil, false
		}
		boundUsers = append(boundUsers, uid)
	}
	boundTeams := make([]uuid.UUID, 0, len(teamIDs))
	for _, raw := range teamIDs {
		tid, err := uuid.Parse(strings.TrimSpace(raw))
		if err != nil {
			http.Error(w, "invalid team id", http.StatusBadRequest)
			return nil, nil, false
		}
		team, err := deps.AdminStore.GetTeam(ctx, tid)
		if err != nil {
			http.Error(w, "team not found", http.StatusNotFound)
			return nil, nil, false
		}
		if team.OrgID != org.ID {
			http.Error(w, "team belongs to a different organization", http.StatusBadRequest)
			return nil, nil, false
		}
		boundTeams = append(boundTeams, tid)
	}
	return boundUsers, boundTeams, true
}

func (h *Handler) keyOwnerNames(deps Dependencies, ctx context.Context, keys []store.InboundKey) (map[string]string, map[string]string) {
	emails := map[string]string{}
	if users, err := deps.AdminStore.ListUsers(ctx); err == nil {
		for _, u := range users {
			emails[u.ID.String()] = u.Email
		}
	}
	teams := map[string]string{}
	for _, k := range keys {
		if k.OwnerTeamID == nil {
			continue
		}
		id := k.OwnerTeamID.String()
		if _, ok := teams[id]; ok {
			continue
		}
		if t, err := deps.AdminStore.GetTeam(ctx, *k.OwnerTeamID); err == nil {
			teams[id] = t.Name
		}
	}
	return emails, teams
}

func keyOwnerStrings(k store.InboundKey, emails, teamNames map[string]string) (string, string, string) {
	if k.OwnerUserID != nil {
		id := k.OwnerUserID.String()
		name := emails[id]
		if name == "" {
			name = id
		}
		return id, "", name
	}
	if k.OwnerTeamID != nil {
		id := k.OwnerTeamID.String()
		name := teamNames[id]
		if name == "" {
			name = id
		}
		return "", id, name
	}
	return "", "", ""
}

func (h *Handler) buildFullKeyView(ctx context.Context, deps Dependencies, names, emails, teamNames map[string]string, k store.InboundKey, canManage bool, usage adminKeyUsageView, quota *adminQuotaView) (adminKeyView, error) {
	userIDs, err := deps.AdminStore.ListKeyUserIDs(ctx, k.ID)
	if err != nil {
		return adminKeyView{}, err
	}
	teamIDs, err := deps.AdminStore.ListKeyTeamIDs(ctx, k.ID)
	if err != nil {
		return adminKeyView{}, err
	}
	users := make([]string, 0, len(userIDs))
	for _, uid := range userIDs {
		users = append(users, uid.String())
	}
	teams := make([]string, 0, len(teamIDs))
	for _, tid := range teamIDs {
		teams = append(teams, tid.String())
	}
	ownerUserID, ownerTeamID, ownerName := keyOwnerStrings(k, emails, teamNames)
	return adminKeyView{
		ID: k.ID.String(), Name: k.Name, TokenPrefix: k.TokenPrefix, Tenant: k.Tenant,
		AllowedModels: k.AllowedModels, Enabled: k.Enabled, Source: "database",
		OrgID: k.OrgID.String(), OrgName: names[k.OrgID.String()],
		Description: k.Description, ExpiresAt: formatKeyExpiry(k.ExpiresAt),
		UserIDs: users, TeamIDs: teams,
		OwnerUserID: ownerUserID, OwnerTeamID: ownerTeamID, OwnerName: ownerName,
		CanManage: canManage, Usage: usage, Quota: quota,
	}, nil
}

func (h *Handler) buildMemberKeyView(names, emails, teamNames map[string]string, k store.InboundKey, usage adminKeyUsageView, quota *adminQuotaView) adminKeyMemberView {
	ownerUserID, ownerTeamID, ownerName := keyOwnerStrings(k, emails, teamNames)
	return adminKeyMemberView{
		ID: k.ID.String(), Name: k.Name, TokenPrefix: k.TokenPrefix,
		AllowedModels: k.AllowedModels, Source: "database",
		OrgID: k.OrgID.String(), OrgName: names[k.OrgID.String()],
		Description: k.Description, ExpiresAt: formatKeyExpiry(k.ExpiresAt),
		OwnerUserID: ownerUserID, OwnerTeamID: ownerTeamID, OwnerName: ownerName,
		Usage: usage, Quota: quota,
	}
}

func (h *Handler) canManageKey(deps Dependencies, ctx context.Context, claims *adminauth.Claims, k store.InboundKey) bool {
	if claims.IsAdmin {
		return true
	}
	if h.canWriteOrg(deps, ctx, claims, k.OrgID) {
		return true
	}
	caller := mustParseUUID(claims.Subject)
	if k.OwnerUserID != nil && *k.OwnerUserID == caller {
		return true
	}
	if k.OwnerTeamID != nil && h.isTeamAdmin(deps, ctx, claims, *k.OwnerTeamID) {
		return true
	}
	return false
}

func (h *Handler) resolveKeyWriteOrg(deps Dependencies, w http.ResponseWriter, r *http.Request, claims *adminauth.Claims, orgIDStr string) (store.Organization, bool) {
	ctx := r.Context()
	orgIDStr = h.requestOrgID(r, orgIDStr)
	if orgIDStr != "" {
		orgID, err := uuid.Parse(orgIDStr)
		if err != nil {
			http.Error(w, "invalid organization id", http.StatusBadRequest)
			return store.Organization{}, false
		}
		if h.canWriteOrg(deps, ctx, claims, orgID) {
			org, err := deps.AdminStore.GetOrganization(ctx, orgID)
			if err != nil {
				http.Error(w, "organization not found", http.StatusNotFound)
				return store.Organization{}, false
			}
			return org, true
		}
		if _, err := deps.AdminStore.GetMembership(ctx, mustParseUUID(claims.Subject), orgID); err != nil {
			http.Error(w, "organization not found", http.StatusNotFound)
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
	if len(memberships) == 1 {
		org, err := deps.AdminStore.GetOrganization(ctx, memberships[0].OrgID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return store.Organization{}, false
		}
		return org, true
	}
	http.Error(w, "specify organization id", http.StatusBadRequest)
	return store.Organization{}, false
}

type keyOwner struct {
	userID *uuid.UUID
	teamID *uuid.UUID
}

func (h *Handler) resolveKeyOwner(deps Dependencies, w http.ResponseWriter, r *http.Request, claims *adminauth.Claims, org store.Organization, ownerType, ownerID string) (keyOwner, bool) {
	ctx := r.Context()
	caller := mustParseUUID(claims.Subject)
	admin := h.canWriteOrg(deps, ctx, claims, org.ID)
	ownerType = strings.ToLower(strings.TrimSpace(ownerType))
	if ownerType == "" {
		if admin {
			return keyOwner{}, true
		}
		uid := caller
		if _, err := deps.AdminStore.GetMembership(ctx, uid, org.ID); err != nil {
			http.Error(w, "organization not found", http.StatusNotFound)
			return keyOwner{}, false
		}
		return keyOwner{userID: &uid}, true
	}
	switch ownerType {
	case "user":
		uid := caller
		if strings.TrimSpace(ownerID) != "" {
			parsed, err := uuid.Parse(strings.TrimSpace(ownerID))
			if err != nil {
				http.Error(w, "invalid owner user id", http.StatusBadRequest)
				return keyOwner{}, false
			}
			uid = parsed
		}
		if _, err := deps.AdminStore.GetMembership(ctx, uid, org.ID); err != nil {
			http.Error(w, "owner is not an organization member", http.StatusBadRequest)
			return keyOwner{}, false
		}
		if !admin && uid != caller {
			http.Error(w, "cannot create personal keys for other users", http.StatusForbidden)
			return keyOwner{}, false
		}
		return keyOwner{userID: &uid}, true
	case "team":
		tid, err := uuid.Parse(strings.TrimSpace(ownerID))
		if err != nil {
			http.Error(w, "owner team id is required", http.StatusBadRequest)
			return keyOwner{}, false
		}
		team, err := deps.AdminStore.GetTeam(ctx, tid)
		if err != nil || team.OrgID != org.ID {
			http.Error(w, "team not found", http.StatusNotFound)
			return keyOwner{}, false
		}
		if !admin && !h.isTeamAdmin(deps, ctx, claims, tid) {
			http.Error(w, "team admin required", http.StatusForbidden)
			return keyOwner{}, false
		}
		return keyOwner{teamID: &tid}, true
	default:
		http.Error(w, `owner type must be "user" or "team"`, http.StatusBadRequest)
		return keyOwner{}, false
	}
}

func appendUUID(out []uuid.UUID, id uuid.UUID) []uuid.UUID {
	for _, v := range out {
		if v == id {
			return out
		}
	}
	return append(out, id)
}

func appendUUIDString(out []string, id string) []string {
	for _, v := range out {
		if strings.TrimSpace(v) == id {
			return out
		}
	}
	return append(out, id)
}

func (h *Handler) keyDetailView(ctx context.Context, deps Dependencies, r *http.Request, claims *adminauth.Claims, k store.InboundKey) (interface{}, bool) {
	names := h.orgNameMap(deps, r, claims)
	emails, teamNames := h.keyOwnerNames(deps, ctx, []store.InboundKey{k})
	usage := adminKeyUsageView{}
	if used, err := deps.AdminStore.KeyUsageByOrg(ctx, k.OrgID); err == nil {
		for _, u := range used {
			if u.KeyID == k.ID {
				usage = adminKeyUsageView{Requests: u.Requests, Tokens: u.Tokens, Spend: u.Spend}
			}
		}
	}
	keyQuota := func(manageable bool) *adminQuotaView {
		scope, ok := keyOwnerScope(k)
		if !ok {
			return nil
		}
		if !manageable && scope.typ == quotaScopeUser {
			return nil
		}
		if view, err := h.quotaView(deps, ctx, k.OrgID, scope); err == nil {
			return &view
		}
		return nil
	}
	if h.canManageKey(deps, ctx, claims, k) {
		full, err := h.buildFullKeyView(ctx, deps, names, emails, teamNames, k, true, usage, keyQuota(true))
		if err != nil {
			return nil, false
		}
		return full, true
	}
	visible, _ := h.visibleOrgIDs(deps, r, claims)
	if visible != nil && !visible[k.OrgID.String()] {
		return nil, false
	}
	shared, err := deps.AdminStore.ListInboundKeysVisibleToMember(ctx, mustParseUUID(claims.Subject))
	if err != nil {
		return nil, false
	}
	for _, s := range shared {
		if s.ID == k.ID {
			return h.buildMemberKeyView(names, emails, teamNames, k, usage, keyQuota(false)), true
		}
	}
	return nil, false
}

func (h *Handler) fullKeyView(ctx context.Context, deps Dependencies, r *http.Request, claims *adminauth.Claims, id uuid.UUID) (adminKeyView, error) {
	names := h.orgNameMap(deps, r, claims)
	rows, err := deps.AdminStore.ListInboundKeys(ctx)
	if err != nil {
		return adminKeyView{}, err
	}
	emails, teamNames := h.keyOwnerNames(deps, ctx, rows)
	for _, k := range rows {
		if k.ID == id {
			var quota *adminQuotaView
			if scope, ok := keyOwnerScope(k); ok {
				if view, err := h.quotaView(deps, ctx, k.OrgID, scope); err == nil {
					quota = &view
				}
			}
			return h.buildFullKeyView(ctx, deps, names, emails, teamNames, k, true, adminKeyUsageView{}, quota)
		}
	}
	return adminKeyView{}, errBad("key not found")
}

func (h *Handler) createDBKey(ctx context.Context, deps Dependencies, org store.Organization, name, tenant string, allowed []string, description string, expires *time.Time, owner keyOwner) (adminKeyView, string, error) {
	name = strings.TrimSpace(name)
	if !validResourceName(name) {
		return adminKeyView{}, "", errBad("invalid key name")
	}
	token, err := newInboundToken()
	if err != nil {
		return adminKeyView{}, "", err
	}
	k := &store.InboundKey{Name: name, TokenHash: store.TokenHash(token), TokenPrefix: token[:12], Tenant: tenant, AllowedModels: allowed, Enabled: true, OrgID: org.ID, Description: strings.TrimSpace(description), ExpiresAt: expires, OwnerUserID: owner.userID, OwnerTeamID: owner.teamID}
	if k.AllowedModels == nil {
		k.AllowedModels = []string{}
	}
	if err := deps.AdminStore.CreateInboundKey(ctx, k); err != nil {
		return adminKeyView{}, "", err
	}
	return adminKeyView{ID: k.ID.String(), Name: k.Name, TokenPrefix: k.TokenPrefix, Tenant: k.Tenant, AllowedModels: k.AllowedModels, Enabled: true, Source: "database", OrgID: org.ID.String(), OrgName: org.Name, Description: k.Description, ExpiresAt: formatKeyExpiry(k.ExpiresAt), CanManage: true}, token, nil
}

func (h *Handler) findInboundKey(ctx context.Context, deps Dependencies, id uuid.UUID) (*store.InboundKey, error) {
	rows, err := deps.AdminStore.ListInboundKeys(ctx)
	if err != nil {
		return nil, err
	}
	for i := range rows {
		if rows[i].ID == id {
			k := rows[i]
			return &k, nil
		}
	}
	return nil, errBad("key not found")
}

func (h *Handler) rotateDBKey(ctx context.Context, deps Dependencies, k *store.InboundKey) (string, error) {
	token, err := newInboundToken()
	if err != nil {
		return "", err
	}
	k.TokenHash = store.TokenHash(token)
	k.TokenPrefix = token[:12]
	k.Enabled = true
	if err := deps.AdminStore.UpdateInboundKey(ctx, k); err != nil {
		return "", err
	}
	return token, nil
}

func (h *Handler) setDBKeyEnabled(ctx context.Context, deps Dependencies, k *store.InboundKey, enabled bool) error {
	k.Enabled = enabled
	return deps.AdminStore.UpdateInboundKey(ctx, k)
}
