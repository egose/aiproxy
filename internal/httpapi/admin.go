package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/egose/aiproxy/internal/adminauth"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/store"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

func ioLimitReader(r *http.Request) io.Reader {
	return io.LimitReader(r.Body, 1<<20)
}

const adminPathPrefix = "/_internal/admin/"

func (h *Handler) handleAdmin(deps Dependencies, w http.ResponseWriter, r *http.Request) bool {
	if !strings.HasPrefix(r.URL.Path, adminPathPrefix) && r.URL.Path != "/_internal/admin" {
		return false
	}
	if deps.AdminStore == nil {
		if r.URL.Path == adminPathPrefix+"status" && r.Method == http.MethodGet {
			writeAdminJSON(w, http.StatusOK, map[string]interface{}{
				"multi_tenancy_enabled": deps.MultiTenancy.Enabled,
				"web_ui_enabled":        deps.WebUI.Enabled,
				"db_ok":                 false,
				"oidc_enabled":          false,
			})
			return true
		}
		http.Error(w, "multi_tenancy not enabled", http.StatusNotFound)
		return true
	}
	path := strings.TrimPrefix(r.URL.Path, adminPathPrefix)
	path = strings.Trim(path, "/")
	segments := []string{}
	if path != "" {
		segments = strings.Split(path, "/")
	}
	switch {
	case len(segments) == 1 && segments[0] == "status" && r.Method == http.MethodGet:
		dbOK := deps.AdminStore.Ping(r.Context()) == nil
		oidcDisplay, oidcEnabledFlag := "", false
		if cfg, err := deps.AdminStore.GetOIDCConfig(r.Context()); err == nil && oidcEnabled(cfg) {
			oidcEnabledFlag = true
			oidcDisplay = cfg.DisplayName
		}
		writeAdminJSON(w, http.StatusOK, map[string]interface{}{
			"multi_tenancy_enabled": deps.MultiTenancy.Enabled,
			"web_ui_enabled":        deps.WebUI.Enabled,
			"db_ok":                 dbOK,
			"oidc_enabled":          oidcEnabledFlag,
			"oidc_display_name":     oidcDisplay,
			"registration_enabled":  deps.MultiTenancy.AllowPublicRegistration,
		})
		return true
	case len(segments) == 1 && segments[0] == "login" && r.Method == http.MethodPost:
		h.adminLogin(deps, w, r)
		return true
	case len(segments) == 1 && segments[0] == "refresh" && r.Method == http.MethodPost:
		h.adminRefresh(deps, w, r)
		return true
	case len(segments) == 1 && segments[0] == "logout" && r.Method == http.MethodPost:
		h.adminLogout(deps, w, r)
		return true
	case len(segments) == 1 && segments[0] == "me" && r.Method == http.MethodGet:
		h.adminMe(deps, w, r)
		return true
	case len(segments) == 1 && segments[0] == "register" && r.Method == http.MethodPost:
		h.adminRegister(deps, w, r)
		return true
	case len(segments) >= 1 && segments[0] == "orgs":
		h.adminOrgs(deps, w, r, segments[1:])
		return true
	case len(segments) >= 1 && segments[0] == "providers":
		h.adminProviders(deps, w, r, segments[1:])
		return true
	case len(segments) == 1 && segments[0] == "provider-types" && r.Method == http.MethodGet:
		h.adminProviderTypes(deps, w, r)
		return true
	case len(segments) >= 1 && segments[0] == "aliases":
		h.adminAliases(deps, w, r, segments[1:])
		return true
	case len(segments) >= 1 && (segments[0] == "keys" || segments[0] == "api-keys"):
		h.adminKeys(deps, w, r, segments[1:])
		return true
	case len(segments) >= 1 && segments[0] == "users":
		h.adminUsers(deps, w, r, segments[1:])
		return true
	case len(segments) >= 1 && segments[0] == "invites":
		h.adminInvites(deps, w, r, segments[1:])
		return true
	case len(segments) == 1 && segments[0] == "oidc-config":
		h.adminOIDCConfig(deps, w, r)
		return true
	case len(segments) == 2 && segments[0] == "oidc" && segments[1] == "start" && r.Method == http.MethodGet:
		h.adminOIDCStart(deps, w, r)
		return true
	case len(segments) == 2 && segments[0] == "oidc" && segments[1] == "callback" && r.Method == http.MethodGet:
		h.adminOIDCCallback(deps, w, r)
		return true
	}
	http.Error(w, "unknown admin endpoint", http.StatusNotFound)
	return true
}

func writeAdminJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func adminBearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, "Bearer ") {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
}

func (h *Handler) adminClaims(deps Dependencies, r *http.Request) (*adminauth.Claims, bool) {
	claims, err := adminauth.VerifyAccess(adminBearer(r))
	if err != nil {
		return nil, false
	}
	return claims, true
}

func (h *Handler) requireAdmin(deps Dependencies, w http.ResponseWriter, r *http.Request) (*adminauth.Claims, bool) {
	claims, ok := h.adminClaims(deps, r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return nil, false
	}
	if !claims.IsAdmin {
		http.Error(w, "forbidden", http.StatusForbidden)
		return nil, false
	}
	return claims, true
}

func (h *Handler) adminLogin(deps Dependencies, w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(ioLimitReader(r)).Decode(&req); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	u, err := deps.AdminStore.GetUserByEmail(r.Context(), strings.TrimSpace(req.Email))
	if err != nil || u.Disabled {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(req.Password)); err != nil {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}
	access, exp, err := adminauth.IssueAccess(u.ID.String(), u.Email, u.IsAdmin)
	if err != nil {
		http.Error(w, "issue token: "+err.Error(), http.StatusInternalServerError)
		return
	}
	refresh, err := adminauth.NewRefreshToken()
	if err != nil {
		http.Error(w, "issue token: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if err := deps.AdminStore.CreateRefreshToken(r.Context(), &store.RefreshToken{
		UserID:    u.ID,
		TokenHash: store.TokenHash(refresh),
		ExpiresAt: time.Now().Add(adminauth.RefreshTTL),
	}); err != nil {
		http.Error(w, "issue token: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeAdminJSON(w, http.StatusOK, map[string]interface{}{
		"access_token":  access,
		"refresh_token": refresh,
		"expires_in":    int(time.Until(exp).Seconds()),
	})
}

func (h *Handler) adminRefresh(deps Dependencies, w http.ResponseWriter, r *http.Request) {
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(ioLimitReader(r)).Decode(&req); err != nil || req.RefreshToken == "" {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	rt, err := deps.AdminStore.GetRefreshToken(r.Context(), store.TokenHash(req.RefreshToken))
	if err != nil || rt.RevokedAt != nil || time.Now().After(rt.ExpiresAt) {
		http.Error(w, "invalid refresh token", http.StatusUnauthorized)
		return
	}
	var u store.User
	users, err := deps.AdminStore.ListUsers(r.Context())
	if err != nil {
		http.Error(w, "refresh failed", http.StatusInternalServerError)
		return
	}
	found := false
	for _, cand := range users {
		if cand.ID == rt.UserID {
			u = cand
			found = true
		}
	}
	if !found || u.Disabled {
		http.Error(w, "invalid refresh token", http.StatusUnauthorized)
		return
	}
	_ = deps.AdminStore.RevokeRefreshToken(r.Context(), rt.ID, time.Now())
	access, exp, err := adminauth.IssueAccess(u.ID.String(), u.Email, u.IsAdmin)
	if err != nil {
		http.Error(w, "issue token: "+err.Error(), http.StatusInternalServerError)
		return
	}
	refresh, err := adminauth.NewRefreshToken()
	if err != nil {
		http.Error(w, "issue token: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if err := deps.AdminStore.CreateRefreshToken(r.Context(), &store.RefreshToken{
		UserID:    u.ID,
		TokenHash: store.TokenHash(refresh),
		ExpiresAt: time.Now().Add(adminauth.RefreshTTL),
	}); err != nil {
		http.Error(w, "issue token: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeAdminJSON(w, http.StatusOK, map[string]interface{}{
		"access_token":  access,
		"refresh_token": refresh,
		"expires_in":    int(time.Until(exp).Seconds()),
	})
}

func (h *Handler) adminLogout(deps Dependencies, w http.ResponseWriter, r *http.Request) {
	claims, ok := h.adminClaims(deps, r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	_ = claims
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	_ = json.NewDecoder(ioLimitReader(r)).Decode(&req)
	if req.RefreshToken != "" {
		if rt, err := deps.AdminStore.GetRefreshToken(r.Context(), store.TokenHash(req.RefreshToken)); err == nil {
			_ = deps.AdminStore.RevokeRefreshToken(r.Context(), rt.ID, time.Now())
		}
	}
	writeAdminJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *Handler) adminMe(deps Dependencies, w http.ResponseWriter, r *http.Request) {
	claims, ok := h.adminClaims(deps, r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	writeAdminJSON(w, http.StatusOK, map[string]interface{}{
		"user_id":  claims.Subject,
		"email":    claims.Email,
		"is_admin": claims.IsAdmin,
	})
}

func newInboundToken() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "sk-admin-" + hex.EncodeToString(buf), nil
}

var _ = uuid.New
var _ = config.ProviderTypeOpenAI
