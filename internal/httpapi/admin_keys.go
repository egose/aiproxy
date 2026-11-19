package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
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
	WorkspaceID   string            `json:"workspace_id,omitempty"`
	WorkspaceName string            `json:"workspace_name,omitempty"`
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
	WorkspaceID   string            `json:"workspace_id,omitempty"`
	WorkspaceName string            `json:"workspace_name,omitempty"`
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
			visibility, ok := h.resolveWorkspaceFilter(deps, w, r, claims)
			if !ok {
				return
			}
			views, err := h.mergedKeyViews(ctx, deps, claims, visibility)
			if err != nil {
				http.Error(w, "could not list keys", http.StatusInternalServerError)
				return
			}
			writeAdminJSON(w, http.StatusOK, map[string]interface{}{"keys": views})
			return
		case http.MethodPost:
			var req struct {
				Name          string   `json:"name"`
				Tenant        string   `json:"tenant"`
				AllowedModels []string `json:"allowed_models"`
				WorkspaceID   string   `json:"workspace_id"`
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
			workspace, ok := h.resolveKeyWriteWorkspace(deps, w, r, claims, req.WorkspaceID)
			if !ok {
				return
			}
			owner, ok := h.resolveKeyOwner(deps, w, r, claims, workspace, req.OwnerType, req.OwnerID)
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
			boundUsers, boundTeams, ok := h.resolveKeyBindings(deps, w, r, workspace, userIDs, teamIDs)
			if !ok {
				return
			}
			view, token, err := h.createDBKey(ctx, deps, workspace, req.Name, req.Tenant, req.AllowedModels, req.Description, expires, owner, mustParseUUID(claims.Subject), boundUsers, boundTeams)
			if err != nil {
				var storage catalogStorageError
				switch {
				case errors.Is(err, store.ErrKeyManagerRequired):
					http.Error(w, store.ErrKeyManagerRequired.Error(), http.StatusForbidden)
				case errors.Is(err, store.ErrInvalidKeyBinding):
					http.Error(w, store.ErrInvalidKeyBinding.Error(), http.StatusBadRequest)
				case errors.Is(err, store.ErrKeyNameExists):
					http.Error(w, store.ErrKeyNameExists.Error(), http.StatusConflict)
				case errors.As(err, &storage):
					http.Error(w, "could not issue key; read current keys before retrying", http.StatusInternalServerError)
				default:
					http.Error(w, err.Error(), http.StatusBadRequest)
				}
				return
			}
			keyID := view.ID
			markKeySaved(w, keyID)
			if !activateKeyIssuance(deps, w, keyID) {
				return
			}
			view, err = h.fullKeyView(ctx, deps, claims, mustParseUUID(keyID))
			if err != nil {
				http.Error(w, "saved but key response view unavailable; "+keyIssuanceRecovery(keyID), http.StatusInternalServerError)
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
		if _, ok := resolveKeyWriteTarget(deps, w, r, claims, id); !ok {
			return
		}
		token, err := h.rotateDBKey(ctx, deps, id)
		if err != nil {
			http.Error(w, "could not rotate key", http.StatusInternalServerError)
			return
		}
		markKeySaved(w, id.String())
		if !activateKeyIssuance(deps, w, id.String()) {
			return
		}
		writeAdminJSON(w, http.StatusOK, map[string]interface{}{"token": token})
	case "revoke":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if _, ok := resolveKeyWriteTarget(deps, w, r, claims, id); !ok {
			return
		}
		if err := deps.AdminStore.SetInboundKeyEnabled(ctx, id, false); err != nil {
			http.Error(w, "could not revoke key", http.StatusInternalServerError)
			return
		}
		if !h.activateChange(deps, w) {
			return
		}
		writeAdminJSON(w, http.StatusOK, map[string]bool{"ok": true})
	case "":
		if r.Method == http.MethodGet {
			h.getAdminKey(deps, w, r, claims, id)
			return
		}
		switch r.Method {
		case http.MethodPut:
			target, ok := resolveKeyWriteTarget(deps, w, r, claims, id)
			if !ok {
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
			workspace, err := deps.AdminStore.GetWorkspace(ctx, target.WorkspaceID)
			if errors.Is(err, sql.ErrNoRows) {
				http.Error(w, "workspace not found", http.StatusNotFound)
				return
			}
			if err != nil {
				http.Error(w, "could not load workspace", http.StatusInternalServerError)
				return
			}
			patch := store.InboundKeyPolicyPatch{Description: req.Description, Tenant: req.Tenant, AllowedModels: req.AllowedModels, SetExpiry: req.ExpiresAt != nil, ReplaceBindings: req.UserIDs != nil || req.TeamIDs != nil}
			if req.Description != nil {
				*patch.Description = strings.TrimSpace(*req.Description)
			}
			if req.ExpiresAt != nil {
				expires, ok := h.parseKeyExpiry(w, *req.ExpiresAt)
				if !ok {
					return
				}
				patch.ExpiresAt = expires
			}
			if req.Tenant != nil {
				*patch.Tenant = strings.TrimSpace(*req.Tenant)
			}
			if req.UserIDs != nil || req.TeamIDs != nil {
				boundUsers, boundTeams, ok := h.resolveKeyBindings(deps, w, r, workspace, req.UserIDs, req.TeamIDs)
				if !ok {
					return
				}
				patch.UserIDs, patch.TeamIDs = boundUsers, boundTeams
			}
			if err := deps.AdminStore.UpdateInboundKeyPolicy(ctx, target.ID, mustParseUUID(claims.Subject), patch); err != nil {
				switch {
				case errors.Is(err, store.ErrKeyManagerRequired):
					http.Error(w, err.Error(), http.StatusForbidden)
				case errors.Is(err, store.ErrInvalidKeyBinding):
					http.Error(w, err.Error(), http.StatusBadRequest)
				case errors.Is(err, sql.ErrNoRows):
					http.Error(w, "key or workspace not found", http.StatusNotFound)
				default:
					http.Error(w, "could not update key", http.StatusInternalServerError)
				}
				return
			}
			if !h.activateChange(deps, w) {
				return
			}
			view, err := h.fullKeyView(ctx, deps, claims, target.ID)
			if err != nil {
				http.Error(w, "saved but key response view unavailable", http.StatusInternalServerError)
				return
			}
			writeAdminJSON(w, http.StatusOK, map[string]interface{}{"key": view})
		case http.MethodDelete:
			if _, ok := resolveKeyWriteTarget(deps, w, r, claims, id); !ok {
				return
			}
			if err := deps.AdminStore.DeleteInboundKey(ctx, id); err != nil {
				http.Error(w, "could not delete key", http.StatusInternalServerError)
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

func keyManagerInScope(visibility workspaceVisibility, caller uuid.UUID, k store.InboundKey) bool {
	return visibility.global || visibility.roles[k.WorkspaceID] == roleAdmin || (k.OwnerUserID != nil && *k.OwnerUserID == caller)
}

func resolveKeyWriteTarget(deps Dependencies, w http.ResponseWriter, r *http.Request, claims *adminauth.Claims, id uuid.UUID) (store.InboundKey, bool) {
	workspaceIDs, err := parseWriteWorkspaceIDs(r, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return store.InboundKey{}, false
	}
	ctx := r.Context()
	visibility, err := resolveWorkspaceVisibility(ctx, deps, claims)
	if err != nil {
		http.Error(w, workspaceVisibilityError, http.StatusInternalServerError)
		return store.InboundKey{}, false
	}
	if !visibility.allowsWriteSelectors(workspaceIDs) {
		http.Error(w, "key not found", http.StatusNotFound)
		return store.InboundKey{}, false
	}
	k, ok := loadAdminKey(ctx, deps, w, id)
	if !ok {
		return store.InboundKey{}, false
	}
	if len(workspaceIDs) > 0 && k.WorkspaceID != workspaceIDs[0] {
		http.Error(w, "key not found", http.StatusNotFound)
		return store.InboundKey{}, false
	}
	access, err := resolveKeyDetailAccess(ctx, deps, visibility, mustParseUUID(claims.Subject), k)
	switch {
	case err != nil:
		http.Error(w, "could not load key", http.StatusInternalServerError)
	case access == keyManageable:
		return k, true
	case access == keyReadOnly:
		http.Error(w, "key manager required", http.StatusForbidden)
	default:
		http.Error(w, "key not found", http.StatusNotFound)
	}
	return store.InboundKey{}, false
}

func (h *Handler) mergedKeyViews(ctx context.Context, deps Dependencies, claims *adminauth.Claims, visibility workspaceVisibility) ([]interface{}, error) {
	views := []interface{}{}
	if visibility.empty() {
		return views, nil
	}
	if visibility.allows(store.SystemWorkspaceID) {
		for name := range deps.AdminAuthConfig.Clients {
			views = append(views, adminKeyView{Name: name, Tenant: deps.AdminAuthConfig.Clients[name].Tenant, AllowedModels: deps.AdminAuthConfig.Clients[name].AllowedModels, Enabled: true, Source: "config"})
		}
	}
	sharedKeys := map[uuid.UUID]bool{}
	callerTeams := map[uuid.UUID]bool{}
	caller := mustParseUUID(claims.Subject)
	if !visibility.global {
		shared, err := deps.AdminStore.ListInboundKeysVisibleToMember(ctx, caller)
		if err != nil {
			return nil, err
		}
		for _, k := range shared {
			sharedKeys[k.ID] = true
		}
		adminTeams, err := deps.AdminStore.ListTeamAdminTeamIDs(ctx, caller)
		if err != nil {
			return nil, err
		}
		for _, tid := range adminTeams {
			callerTeams[tid] = true
		}
	}
	canManage := func(k store.InboundKey) bool {
		return keyManagerInScope(visibility, caller, k) || (k.OwnerTeamID != nil && callerTeams[*k.OwnerTeamID])
	}
	rows, err := deps.AdminStore.ListInboundKeys(ctx)
	if err != nil {
		return nil, err
	}
	rows = slices.DeleteFunc(rows, func(k store.InboundKey) bool {
		return !visibility.allows(k.WorkspaceID) || (!canManage(k) && !sharedKeys[k.ID])
	})
	names := workspaceNameMap(ctx, deps, visibility)
	emails, teamNames := h.keyOwnerNames(deps, ctx, rows)
	usageByKey := map[string]adminKeyUsageView{}
	usageWorkspaces := map[string]bool{}
	for _, k := range rows {
		usageWorkspaces[k.WorkspaceID.String()] = true
	}
	for workspaceStr := range usageWorkspaces {
		if used, err := deps.AdminStore.KeyUsageByWorkspace(ctx, mustParseUUID(workspaceStr)); err == nil {
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
		if view, err := h.quotaView(deps, ctx, k.WorkspaceID, scope); err == nil {
			return &view
		}
		return nil
	}
	for _, k := range rows {
		if canManage(k) {
			full, err := h.buildFullKeyView(ctx, deps, names, emails, teamNames, k, true, usageByKey[k.ID.String()], keyQuota(k, true))
			if err != nil {
				return nil, err
			}
			views = append(views, full)
			continue
		}
		views = append(views, h.buildMemberKeyView(names, emails, teamNames, k, usageByKey[k.ID.String()], keyQuota(k, false)))
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

func (h *Handler) resolveKeyBindings(deps Dependencies, w http.ResponseWriter, r *http.Request, workspace store.Workspace, userIDs, teamIDs []string) ([]uuid.UUID, []uuid.UUID, bool) {
	ctx := r.Context()
	boundUsers := make([]uuid.UUID, 0, len(userIDs))
	for _, raw := range userIDs {
		uid, err := uuid.Parse(strings.TrimSpace(raw))
		if err != nil {
			http.Error(w, "invalid user id", http.StatusBadRequest)
			return nil, nil, false
		}
		if _, err := deps.AdminStore.GetMembership(ctx, uid, workspace.ID); err != nil {
			http.Error(w, "user is not an workspace member", http.StatusBadRequest)
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
		if team.WorkspaceID != workspace.ID {
			http.Error(w, "team belongs to a different workspace", http.StatusBadRequest)
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
		WorkspaceID: k.WorkspaceID.String(), WorkspaceName: names[k.WorkspaceID.String()],
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
		WorkspaceID: k.WorkspaceID.String(), WorkspaceName: names[k.WorkspaceID.String()],
		Description: k.Description, ExpiresAt: formatKeyExpiry(k.ExpiresAt),
		OwnerUserID: ownerUserID, OwnerTeamID: ownerTeamID, OwnerName: ownerName,
		Usage: usage, Quota: quota,
	}
}

func (h *Handler) canManageKey(deps Dependencies, ctx context.Context, claims *adminauth.Claims, k store.InboundKey) bool {
	visibility, err := resolveWorkspaceVisibility(ctx, deps, claims)
	if err != nil {
		return false
	}
	access, err := resolveKeyDetailAccess(ctx, deps, visibility, mustParseUUID(claims.Subject), k)
	return err == nil && access == keyManageable
}

func (h *Handler) resolveKeyWriteWorkspace(deps Dependencies, w http.ResponseWriter, r *http.Request, claims *adminauth.Claims, workspaceIDStr string) (store.Workspace, bool) {
	ctx := r.Context()
	workspaceIDStr = h.requestWorkspaceID(r, workspaceIDStr)
	if workspaceIDStr != "" {
		workspaceID, err := uuid.Parse(workspaceIDStr)
		if err != nil {
			http.Error(w, "invalid workspace id", http.StatusBadRequest)
			return store.Workspace{}, false
		}
		if h.canWriteWorkspace(deps, ctx, claims, workspaceID) {
			workspace, err := deps.AdminStore.GetWorkspace(ctx, workspaceID)
			if err != nil {
				http.Error(w, "workspace not found", http.StatusNotFound)
				return store.Workspace{}, false
			}
			return workspace, true
		}
		if _, err := deps.AdminStore.GetMembership(ctx, mustParseUUID(claims.Subject), workspaceID); err != nil {
			http.Error(w, "workspace not found", http.StatusNotFound)
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
		http.Error(w, workspaceVisibilityError, http.StatusInternalServerError)
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
	if len(memberships) == 1 {
		workspace, err := deps.AdminStore.GetWorkspace(ctx, memberships[0].WorkspaceID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return store.Workspace{}, false
		}
		return workspace, true
	}
	http.Error(w, "specify workspace id", http.StatusBadRequest)
	return store.Workspace{}, false
}

type keyOwner struct {
	userID *uuid.UUID
	teamID *uuid.UUID
}

func (h *Handler) resolveKeyOwner(deps Dependencies, w http.ResponseWriter, r *http.Request, claims *adminauth.Claims, workspace store.Workspace, ownerType, ownerID string) (keyOwner, bool) {
	ctx := r.Context()
	caller := mustParseUUID(claims.Subject)
	admin := h.canWriteWorkspace(deps, ctx, claims, workspace.ID)
	ownerType = strings.ToLower(strings.TrimSpace(ownerType))
	if ownerType == "" {
		if admin {
			return keyOwner{}, true
		}
		uid := caller
		if _, err := deps.AdminStore.GetMembership(ctx, uid, workspace.ID); err != nil {
			http.Error(w, "workspace not found", http.StatusNotFound)
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
		if _, err := deps.AdminStore.GetMembership(ctx, uid, workspace.ID); err != nil {
			http.Error(w, "owner is not an workspace member", http.StatusBadRequest)
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
		if err != nil || team.WorkspaceID != workspace.ID {
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

func (h *Handler) getAdminKey(deps Dependencies, w http.ResponseWriter, r *http.Request, claims *adminauth.Claims, id uuid.UUID) {
	visibility, ok := h.resolveWorkspaceFilter(deps, w, r, claims)
	if !ok {
		return
	}
	ctx := r.Context()
	if visibility.empty() {
		http.Error(w, "key not found", http.StatusNotFound)
		return
	}
	k, ok := loadAdminKey(ctx, deps, w, id)
	if !ok {
		return
	}
	access, err := resolveKeyDetailAccess(ctx, deps, visibility, mustParseUUID(claims.Subject), k)
	if err != nil {
		http.Error(w, "could not load key", http.StatusInternalServerError)
		return
	}
	if access == keyHidden {
		http.Error(w, "key not found", http.StatusNotFound)
		return
	}
	view, err := h.keyDetailView(ctx, deps, visibility, k, access == keyManageable)
	if err != nil {
		http.Error(w, "could not load key", http.StatusInternalServerError)
		return
	}
	writeAdminJSON(w, http.StatusOK, map[string]interface{}{"key": view})
}

func loadAdminKey(ctx context.Context, deps Dependencies, w http.ResponseWriter, id uuid.UUID) (store.InboundKey, bool) {
	k, err := deps.AdminStore.GetInboundKey(ctx, id)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		http.Error(w, "key not found", http.StatusNotFound)
	case err != nil:
		http.Error(w, "could not load key", http.StatusInternalServerError)
	default:
		return k, true
	}
	return store.InboundKey{}, false
}

type keyDetailAccess int

const (
	keyHidden keyDetailAccess = iota
	keyReadOnly
	keyManageable
)

func resolveKeyDetailAccess(ctx context.Context, deps Dependencies, visibility workspaceVisibility, caller uuid.UUID, k store.InboundKey) (keyDetailAccess, error) {
	if !visibility.allows(k.WorkspaceID) || (!visibility.global && k.WorkspaceID == store.SystemWorkspaceID) {
		return keyHidden, nil
	}
	if keyManagerInScope(visibility, caller, k) {
		return keyManageable, nil
	}
	if k.OwnerTeamID != nil {
		team, err := deps.AdminStore.GetTeam(ctx, *k.OwnerTeamID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return keyHidden, err
		}
		if err == nil && team.WorkspaceID == k.WorkspaceID {
			member, err := deps.AdminStore.GetTeamMember(ctx, caller, team.ID)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return keyHidden, err
			}
			if err == nil && member.Role == roleAdmin {
				return keyManageable, nil
			}
		}
	}
	shared, err := deps.AdminStore.InboundKeySharedWithMember(ctx, k.ID, caller)
	if err != nil {
		return keyHidden, err
	}
	if shared {
		return keyReadOnly, nil
	}
	return keyHidden, nil
}

func (h *Handler) keyDetailView(ctx context.Context, deps Dependencies, visibility workspaceVisibility, k store.InboundKey, manageable bool) (interface{}, error) {
	names := workspaceNameMap(ctx, deps, visibility)
	emails, teamNames := h.keyOwnerNames(deps, ctx, []store.InboundKey{k})
	usage := adminKeyUsageView{}
	if used, err := deps.AdminStore.KeyUsageByWorkspace(ctx, k.WorkspaceID); err == nil {
		for _, u := range used {
			if u.KeyID == k.ID {
				usage = adminKeyUsageView{Requests: u.Requests, Tokens: u.Tokens, Spend: u.Spend}
			}
		}
	}
	var quota *adminQuotaView
	if scope, ok := keyOwnerScope(k); ok && (manageable || scope.typ != quotaScopeUser) {
		if view, err := h.quotaView(deps, ctx, k.WorkspaceID, scope); err == nil {
			quota = &view
		}
	}
	if manageable {
		return h.buildFullKeyView(ctx, deps, names, emails, teamNames, k, true, usage, quota)
	}
	return h.buildMemberKeyView(names, emails, teamNames, k, usage, quota), nil
}

func (h *Handler) fullKeyView(ctx context.Context, deps Dependencies, claims *adminauth.Claims, id uuid.UUID) (adminKeyView, error) {
	visibility, err := resolveWorkspaceVisibility(ctx, deps, claims)
	if err != nil {
		return adminKeyView{}, err
	}
	names := workspaceNameMap(ctx, deps, visibility)
	k, err := deps.AdminStore.GetInboundKey(ctx, id)
	if err != nil {
		return adminKeyView{}, err
	}
	if !visibility.allows(k.WorkspaceID) {
		return adminKeyView{}, errBad("key not found")
	}
	emails, teamNames := h.keyOwnerNames(deps, ctx, []store.InboundKey{k})
	var quota *adminQuotaView
	if scope, ok := keyOwnerScope(k); ok {
		if view, err := h.quotaView(deps, ctx, k.WorkspaceID, scope); err == nil {
			quota = &view
		}
	}
	return h.buildFullKeyView(ctx, deps, names, emails, teamNames, k, true, adminKeyUsageView{}, quota)
}

func (h *Handler) createDBKey(ctx context.Context, deps Dependencies, workspace store.Workspace, name, tenant string, allowed []string, description string, expires *time.Time, owner keyOwner, actorID uuid.UUID, userIDs, teamIDs []uuid.UUID) (adminKeyView, string, error) {
	name = strings.TrimSpace(name)
	if !validResourceName(name) {
		return adminKeyView{}, "", errBad("invalid key name")
	}
	token, err := newInboundToken()
	if err != nil {
		return adminKeyView{}, "", err
	}
	k := &store.InboundKey{Name: name, TokenHash: store.TokenHash(token), TokenPrefix: token[:12], Tenant: tenant, AllowedModels: allowed, Enabled: true, WorkspaceID: workspace.ID, Description: strings.TrimSpace(description), ExpiresAt: expires, OwnerUserID: owner.userID, OwnerTeamID: owner.teamID}
	if k.AllowedModels == nil {
		k.AllowedModels = []string{}
	}
	if err := deps.AdminStore.IssueInboundKey(ctx, k, actorID, userIDs, teamIDs); err != nil {
		return adminKeyView{}, "", catalogStorageError{err}
	}
	return adminKeyView{ID: k.ID.String(), Name: k.Name, TokenPrefix: k.TokenPrefix, Tenant: k.Tenant, AllowedModels: k.AllowedModels, Enabled: true, Source: "database", WorkspaceID: workspace.ID.String(), WorkspaceName: workspace.Name, Description: k.Description, ExpiresAt: formatKeyExpiry(k.ExpiresAt), CanManage: true}, token, nil
}

func markKeySaved(w http.ResponseWriter, id string) {
	w.Header().Set("X-Aiproxy-Key-Saved", "true")
	w.Header().Set("X-Aiproxy-Key-ID", id)
	w.Header().Set("Location", "/_internal/admin/keys/"+id)
	w.Header().Set("Cache-Control", "no-store")
}

func keyIssuanceRecovery(id string) string {
	return "key " + id + "; read this key and rotate it to obtain a new bearer; do not repeat creation"
}

func activateKeyIssuance(deps Dependencies, w http.ResponseWriter, id string) bool {
	if deps.RequestReload != nil {
		if err := deps.RequestReload(); err != nil {
			if deps.Logger != nil {
				deps.Logger.Error("key activation failed", "error", err)
			}
			http.Error(w, "saved but activation failed; "+keyIssuanceRecovery(id), http.StatusInternalServerError)
			return false
		}
	}
	return true
}

func (h *Handler) rotateDBKey(ctx context.Context, deps Dependencies, id uuid.UUID) (string, error) {
	token, err := newInboundToken()
	if err != nil {
		return "", err
	}
	if err := deps.AdminStore.RotateInboundKey(ctx, id, store.TokenHash(token), token[:12]); err != nil {
		return "", err
	}
	return token, nil
}
