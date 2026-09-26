package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/egose/aiproxy/internal/adminauth"
	"github.com/egose/aiproxy/internal/store"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/oauth2"
)

const oidcCallbackSuffix = "/_internal/admin/oidc/callback"

type oidcState struct {
	nonce     string
	expiresAt time.Time
}

var oidcStates = struct {
	sync.Mutex
	entries map[string]oidcState
}{entries: map[string]oidcState{}}

var oidcProviders = struct {
	sync.Mutex
	entries map[string]*oidc.Provider
}{entries: map[string]*oidc.Provider{}}

func newRandomHex(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func oidcDiscovery(ctx context.Context, issuer string) (*oidc.Provider, error) {
	oidcProviders.Lock()
	if p, ok := oidcProviders.entries[issuer]; ok {
		oidcProviders.Unlock()
		return p, nil
	}
	oidcProviders.Unlock()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	p, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, err
	}
	oidcProviders.Lock()
	oidcProviders.entries[issuer] = p
	oidcProviders.Unlock()
	return p, nil
}

func oidcScopes(cfg store.OIDCConfig) []string {
	scopes := strings.Fields(cfg.Scopes)
	if len(scopes) == 0 {
		return []string{oidc.ScopeOpenID, "email", "profile"}
	}
	return scopes
}

func (h *Handler) oidcConfig(ctx context.Context, deps Dependencies) (store.OIDCConfig, error) {
	return deps.AdminStore.GetOIDCConfig(ctx)
}

func oidcEnabled(cfg store.OIDCConfig) bool {
	return cfg.Enabled && strings.TrimSpace(cfg.IssuerURL) != "" && strings.TrimSpace(cfg.ClientID) != ""
}

func oidcBaseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if forwarded := r.Header.Get("X-Forwarded-Proto"); forwarded != "" {
		scheme = strings.ToLower(strings.TrimSpace(strings.Split(forwarded, ",")[0]))
	}
	host := r.Host
	if forwarded := r.Header.Get("X-Forwarded-Host"); forwarded != "" {
		host = strings.TrimSpace(strings.Split(forwarded, ",")[0])
	}
	return scheme + "://" + host
}

func oidcRedirectURL(r *http.Request) string {
	return oidcBaseURL(r) + oidcCallbackSuffix
}

func uiOIDCCallbackURL(r *http.Request, fragment string) string {
	return oidcBaseURL(r) + "/admin/oidc/callback#" + fragment
}

func (h *Handler) adminOIDCConfig(deps Dependencies, w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	switch r.Method {
	case http.MethodGet:
		if _, ok := h.adminClaims(deps, r); !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		cfg, err := h.oidcConfig(ctx, deps)
		if err != nil {
			cfg = store.OIDCConfig{Scopes: "openid email profile", UsernameClaim: "email", DisplayName: "Single Sign-On"}
		}
		writeAdminJSON(w, http.StatusOK, oidcConfigView(cfg))
	case http.MethodPut:
		if _, ok := h.requireAdmin(deps, w, r); !ok {
			return
		}
		var req struct {
			Enabled       *bool   `json:"enabled"`
			IssuerURL     *string `json:"issuer_url"`
			ClientID      *string `json:"client_id"`
			ClientSecret  string  `json:"client_secret"`
			Scopes        *string `json:"scopes"`
			UsernameClaim *string `json:"username_claim"`
			AdminClaim    *string `json:"admin_claim"`
			AdminValue    *string `json:"admin_value"`
			DisplayName   *string `json:"display_name"`
		}
		if err := json.NewDecoder(ioLimitReader(r)).Decode(&req); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		cfg, err := h.oidcConfig(ctx, deps)
		if err != nil {
			cfg = store.OIDCConfig{}
		}
		if req.Enabled != nil {
			cfg.Enabled = *req.Enabled
		}
		if req.IssuerURL != nil {
			cfg.IssuerURL = strings.TrimSpace(*req.IssuerURL)
		}
		if req.ClientID != nil {
			cfg.ClientID = strings.TrimSpace(*req.ClientID)
		}
		if req.ClientSecret != "" {
			enc, err := store.EncryptSecret([]byte(req.ClientSecret))
			if err != nil {
				http.Error(w, "encrypt secret: "+err.Error(), http.StatusInternalServerError)
				return
			}
			cfg.ClientSecretEncrypted = enc // pragma: allowlist secret
		}
		if req.Scopes != nil {
			cfg.Scopes = strings.TrimSpace(*req.Scopes)
		}
		if req.UsernameClaim != nil {
			cfg.UsernameClaim = strings.TrimSpace(*req.UsernameClaim)
		}
		if req.AdminClaim != nil {
			cfg.AdminClaim = strings.TrimSpace(*req.AdminClaim)
		}
		if req.AdminValue != nil {
			cfg.AdminValue = strings.TrimSpace(*req.AdminValue)
		}
		if req.DisplayName != nil {
			cfg.DisplayName = strings.TrimSpace(*req.DisplayName)
		}
		if cfg.Scopes == "" {
			cfg.Scopes = "openid email profile"
		}
		if cfg.UsernameClaim == "" {
			cfg.UsernameClaim = "email"
		}
		if cfg.DisplayName == "" {
			cfg.DisplayName = "Single Sign-On"
		}
		if cfg.Enabled {
			if cfg.IssuerURL == "" || cfg.ClientID == "" {
				http.Error(w, "issuer_url and client_id are required when enabled", http.StatusBadRequest)
				return
			}
			if _, err := url.ParseRequestURI(cfg.IssuerURL); err != nil {
				http.Error(w, "invalid issuer_url", http.StatusBadRequest)
				return
			}
			if _, err := oidcDiscovery(ctx, cfg.IssuerURL); err != nil {
				http.Error(w, "OIDC discovery failed: "+err.Error(), http.StatusBadRequest)
				return
			}
		}
		if err := deps.AdminStore.UpsertOIDCConfig(ctx, &cfg); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeAdminJSON(w, http.StatusOK, oidcConfigView(cfg))
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func oidcConfigView(cfg store.OIDCConfig) map[string]interface{} {
	return map[string]interface{}{
		"enabled":           cfg.Enabled,
		"issuer_url":        cfg.IssuerURL,
		"client_id":         cfg.ClientID,
		"has_client_secret": len(cfg.ClientSecretEncrypted) > 0,
		"scopes":            cfg.Scopes,
		"username_claim":    cfg.UsernameClaim,
		"admin_claim":       cfg.AdminClaim,
		"admin_value":       cfg.AdminValue,
		"display_name":      cfg.DisplayName,
		"effective_enabled": oidcEnabled(cfg),
	}
}

func (h *Handler) adminOIDCStart(deps Dependencies, w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	cfg, err := h.oidcConfig(ctx, deps)
	if err != nil || !oidcEnabled(cfg) {
		http.Error(w, "single sign-on is not configured", http.StatusNotFound)
		return
	}
	provider, err := oidcDiscovery(ctx, cfg.IssuerURL)
	if err != nil {
		http.Error(w, "OIDC discovery failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	state, err := newRandomHex(32)
	if err != nil {
		http.Error(w, "state: "+err.Error(), http.StatusInternalServerError)
		return
	}
	nonce, err := newRandomHex(32)
	if err != nil {
		http.Error(w, "nonce: "+err.Error(), http.StatusInternalServerError)
		return
	}
	oidcStates.Lock()
	now := time.Now()
	for s, st := range oidcStates.entries {
		if now.After(st.expiresAt) {
			delete(oidcStates.entries, s)
		}
	}
	oidcStates.entries[state] = oidcState{nonce: nonce, expiresAt: now.Add(10 * time.Minute)}
	oidcStates.Unlock()
	oauthCfg := oauth2.Config{
		ClientID:    cfg.ClientID,
		Endpoint:    provider.Endpoint(),
		RedirectURL: oidcRedirectURL(r),
		Scopes:      oidcScopes(cfg),
	}
	http.Redirect(w, r, oauthCfg.AuthCodeURL(state, oidc.Nonce(nonce)), http.StatusFound)
}

func (h *Handler) adminOIDCCallback(deps Dependencies, w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	fail := func(msg string) {
		http.Redirect(w, r, uiOIDCCallbackURL(r, "error="+url.QueryEscape(msg)), http.StatusFound)
	}
	if errParam := r.URL.Query().Get("error"); errParam != "" {
		desc := r.URL.Query().Get("error_description")
		if desc == "" {
			desc = errParam
		}
		fail(desc)
		return
	}
	state := r.URL.Query().Get("state")
	code := r.URL.Query().Get("code")
	if state == "" || code == "" {
		fail("missing state or code")
		return
	}
	oidcStates.Lock()
	st, ok := oidcStates.entries[state]
	if ok {
		delete(oidcStates.entries, state)
	}
	oidcStates.Unlock()
	if !ok || time.Now().After(st.expiresAt) {
		fail("invalid or expired state")
		return
	}
	cfg, err := h.oidcConfig(ctx, deps)
	if err != nil || !oidcEnabled(cfg) {
		fail("single sign-on is not configured")
		return
	}
	provider, err := oidcDiscovery(ctx, cfg.IssuerURL)
	if err != nil {
		fail("OIDC discovery failed")
		return
	}
	secret, err := store.DecryptSecret(cfg.ClientSecretEncrypted)
	if err != nil {
		fail("server cannot decrypt the OIDC client secret")
		return
	}
	oauthCfg := oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: string(secret),
		Endpoint:     provider.Endpoint(),
		RedirectURL:  oidcRedirectURL(r),
		Scopes:       oidcScopes(cfg),
	}
	token, err := oauthCfg.Exchange(ctx, code)
	if err != nil {
		fail("code exchange failed")
		return
	}
	rawID, ok := token.Extra("id_token").(string)
	if !ok || rawID == "" {
		fail("no id_token in response")
		return
	}
	verifier := provider.Verifier(&oidc.Config{ClientID: cfg.ClientID})
	idToken, err := verifier.Verify(ctx, rawID)
	if err != nil {
		fail("id_token verification failed")
		return
	}
	var claims map[string]interface{}
	if err := idToken.Claims(&claims); err != nil {
		fail("cannot read id_token claims")
		return
	}
	if idToken.Nonce != st.nonce {
		fail("nonce mismatch")
		return
	}
	email := oidcClaimString(claims, cfg.UsernameClaim)
	if email == "" {
		email = oidcClaimString(claims, "email")
	}
	if email == "" {
		email = oidcClaimString(claims, "preferred_username")
	}
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" {
		fail("no email claim in id_token")
		return
	}
	isAdmin := oidcAdminMatch(claims, cfg)
	u, err := deps.AdminStore.GetUserByEmail(ctx, email)
	if err != nil {
		randomPassword, herr := newRandomHex(32)
		if herr != nil {
			fail("cannot provision user")
			return
		}
		hash, herr := bcrypt.GenerateFromPassword([]byte(randomPassword), bcrypt.DefaultCost)
		if herr != nil {
			fail("cannot provision user")
			return
		}
		nu := &store.User{Email: email, PasswordHash: string(hash), IsAdmin: isAdmin}
		if cerr := deps.AdminStore.CreateUser(ctx, nu); cerr != nil {
			fail("cannot provision user")
			return
		}
		u = *nu
	}
	if err := h.ensurePersonalOrg(ctx, deps, u.ID, u.Email); err != nil {
		fail("cannot provision organization")
		return
	} else {
		if u.Disabled {
			fail("account is disabled")
			return
		}
		if strings.TrimSpace(cfg.AdminClaim) != "" && u.IsAdmin != isAdmin {
			u.IsAdmin = isAdmin
			_ = deps.AdminStore.UpdateUser(ctx, &u)
		}
	}
	access, exp, err := adminauth.IssueAccess(u.ID.String(), u.Email, u.IsAdmin)
	if err != nil {
		fail("cannot issue session")
		return
	}
	refresh, err := adminauth.NewRefreshToken()
	if err != nil {
		fail("cannot issue session")
		return
	}
	if err := deps.AdminStore.CreateRefreshToken(ctx, &store.RefreshToken{
		UserID:    u.ID,
		TokenHash: store.TokenHash(refresh),
		ExpiresAt: time.Now().Add(adminauth.RefreshTTL),
	}); err != nil {
		fail("cannot issue session")
		return
	}
	fragment := url.Values{
		"access_token":  {access},
		"refresh_token": {refresh},
		"expires_in":    {fmt.Sprintf("%d", int(time.Until(exp).Seconds()))},
	}.Encode()
	http.Redirect(w, r, uiOIDCCallbackURL(r, fragment), http.StatusFound)
}

func oidcClaimString(claims map[string]interface{}, name string) string {
	v, ok := claims[name]
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}

func oidcAdminMatch(claims map[string]interface{}, cfg store.OIDCConfig) bool {
	claim := strings.TrimSpace(cfg.AdminClaim)
	want := strings.TrimSpace(cfg.AdminValue)
	if claim == "" || want == "" {
		return false
	}
	v, ok := claims[claim]
	if !ok {
		return false
	}
	switch tv := v.(type) {
	case string:
		return tv == want
	case bool:
		return (want == "true") == tv
	case []interface{}:
		for _, item := range tv {
			if s, _ := item.(string); s == want {
				return true
			}
		}
	}
	return false
}
