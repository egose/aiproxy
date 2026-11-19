package httpapi

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/accounting"
	"github.com/egose/aiproxy/internal/adminauth"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/observability"
	"github.com/egose/aiproxy/internal/providerhealth"
	"github.com/egose/aiproxy/internal/store"
	"github.com/uptrace/bun"
	"golang.org/x/crypto/bcrypt"
)

func b64url(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}

func signIDToken(t *testing.T, key *rsa.PrivateKey, issuer, clientID, nonce, email string) string {
	t.Helper()
	header := b64url([]byte(`{"alg":"RS256","kid":"testkey","typ":"JWT"}`))
	payload, err := json.Marshal(map[string]interface{}{
		"iss":                issuer,
		"sub":                "user-1",
		"aud":                clientID,
		"exp":                time.Now().Add(time.Hour).Unix(),
		"iat":                time.Now().Unix(),
		"nonce":              nonce,
		"email":              email,
		"email_verified":     true,
		"preferred_username": email,
	})
	if err != nil {
		t.Fatal(err)
	}
	unsigned := header + "." + b64url(payload)
	sum := sha256.Sum256([]byte(unsigned))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if err != nil {
		t.Fatal(err)
	}
	return unsigned + "." + b64url(sig)
}

func TestAdminOIDCFullCodeFlow(t *testing.T) {
	dbURL := os.Getenv("AIPROXY_TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("AIPROXY_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	st, err := store.Open(ctx, dbURL)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err := st.MigrateUp(ctx); err != nil {
		t.Fatal(err)
	}

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	n := b64url(key.PublicKey.N.Bytes())
	e := b64url([]byte{byte(key.PublicKey.E >> 16), byte(key.PublicKey.E >> 8), byte(key.PublicKey.E)})
	var issuer string
	var lastNonce string
	email := fmt.Sprintf("sso-user-%d@example.com", time.Now().UnixNano())
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"issuer":                 issuer,
			"authorization_endpoint": issuer + "/auth",
			"token_endpoint":         issuer + "/token",
			"jwks_uri":               issuer + "/jwks",
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"keys": []interface{}{
			map[string]interface{}{"kty": "RSA", "kid": "testkey", "use": "sig", "alg": "RS256", "n": n, "e": e},
		}})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		if r.FormValue("code") != "valid-code" {
			http.Error(w, "bad code", http.StatusBadRequest)
			return
		}
		id := signIDToken(t, key, issuer, "test-client", lastNonce, email)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "opaque", "token_type": "Bearer", "id_token": id,
		})
	})
	oidcSrv := httptest.NewServer(mux)
	defer oidcSrv.Close()
	issuer = oidcSrv.URL

	if err := os.Setenv("AIPROXY_JWT_SECRET", "test-jwt-secret-1234567890"); err != nil {
		t.Fatal(err)
	}
	if err := os.Setenv("AIPROXY_DB_ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef"); err != nil {
		t.Fatal(err)
	}
	secretEnc, err := store.EncryptSecret([]byte("test-secret"))
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertOIDCConfig(ctx, &store.OIDCConfig{
		Enabled: true, IssuerURL: issuer, ClientID: "test-client",
		ClientSecretEncrypted: secretEnc, Scopes: "openid email profile",
		UsernameClaim: "email", DisplayName: "Test SSO",
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = st.UpsertOIDCConfig(context.Background(), &store.OIDCConfig{Enabled: false})
	})

	rt := &config.Runtime{MultiTenancy: config.MultiTenancy{Enabled: true}}
	h := NewHandler(Dependencies{Catalog: rt.Catalog, MultiTenancy: rt.MultiTenancy, AdminStore: st, AdminAuthConfig: rt.Auth})
	workspace := &store.Workspace{Name: fmt.Sprintf("oidc-invited-%d", time.Now().UnixNano()), Kind: "organization"}
	if err := st.CreateWorkspace(ctx, workspace); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.DeleteWorkspace(ctx, workspace.ID) })
	inv := &store.Invite{Email: email, TokenHash: store.TokenHash(email), WorkspaceID: &workspace.ID,
		WorkspaceRole: "member", ExpiresAt: time.Now().Add(time.Hour)}
	if err := st.CreateInvite(ctx, inv); err != nil {
		t.Fatal(err)
	}
	code, _ := callAdmin(t, h, "", http.MethodPost, "/_internal/admin/invites/accept", map[string]any{"token": email, "password": "invited-password"})
	if code != http.StatusCreated {
		t.Fatalf("invite acceptance = %d", code)
	}
	u, err := st.GetUserByEmail(ctx, email)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.DeleteUser(ctx, u.ID) })
	hook := &oidcUserLookupHook{}
	st.DB.AddQueryHook(hook)
	unredeemedEmail := "unknown-" + u.Email
	pendingInvite := &store.Invite{Email: unredeemedEmail, TokenHash: store.TokenHash(unredeemedEmail), WorkspaceID: &workspace.ID, WorkspaceRole: "member", ExpiresAt: time.Now().Add(time.Hour)}
	if err := st.CreateInvite(ctx, pendingInvite); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.DeleteInvite(ctx, pendingInvite.ID) })
	for _, tc := range []struct {
		name                                                 string
		unknown, lookupFailure, disabled, publicRegistration bool
	}{
		{name: "invited"}, {name: "returning", publicRegistration: true},
		{name: "unknown", unknown: true}, {name: "unknown-public-registration", unknown: true, publicRegistration: true},
		{name: "lookup-failure", lookupFailure: true}, {name: "disabled", disabled: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			email = u.Email
			if tc.unknown {
				email = "unknown-" + u.Email
			}
			u.Disabled = tc.disabled
			if err := st.UpdateUser(ctx, &u); err != nil {
				t.Fatal(err)
			}
			deps := h.SnapshotDependencies()
			deps.MultiTenancy.AllowPublicRegistration = tc.publicRegistration
			h.UpdateDependencies(deps)
			hook.fail = tc.lookupFailure
			beforeUsers, err := st.DB.NewSelect().Model((*store.User)(nil)).Count(ctx)
			if err != nil {
				t.Fatal(err)
			}
			beforeWorkspaces, err := st.DB.NewSelect().Model((*store.Workspace)(nil)).Count(ctx)
			if err != nil {
				t.Fatal(err)
			}
			beforeSessions, err := st.DB.NewSelect().Model((*store.RefreshToken)(nil)).Count(ctx)
			if err != nil {
				t.Fatal(err)
			}

			startReq := httptest.NewRequest(http.MethodGet, "/_internal/admin/oidc/start", nil)
			startReq.Host = "example.com"
			startW := httptest.NewRecorder()
			h.ServeHTTP(startW, startReq)
			if startW.Code != http.StatusFound {
				t.Fatalf("start status = %d", startW.Code)
			}
			loc, err := url.Parse(startW.Header().Get("Location"))
			if err != nil {
				t.Fatal(err)
			}
			state := loc.Query().Get("state")
			if state == "" {
				t.Fatalf("no state in %s", loc.String())
			}
			oidcStates.Lock()
			stored, ok := oidcStates.entries[state]
			oidcStates.Unlock()
			if !ok {
				t.Fatalf("state not stored")
			}
			lastNonce = stored.nonce

			cbReq := httptest.NewRequest(http.MethodGet, "/_internal/admin/oidc/callback?state="+state+"&code=valid-code", nil)
			cbReq.Host = "example.com"
			cbW := httptest.NewRecorder()
			h.ServeHTTP(cbW, cbReq)
			if cbW.Code != http.StatusFound {
				t.Fatalf("callback status = %d, body = %s", cbW.Code, cbW.Body.String())
			}
			fragLoc := cbW.Header().Get("Location")
			if !strings.HasPrefix(fragLoc, "http://example.com/admin/oidc/callback#") {
				t.Fatalf("callback redirect = %s", fragLoc)
			}
			frag, err := url.ParseQuery(strings.TrimPrefix(fragLoc[strings.Index(fragLoc, "#")+1:], ""))
			if err != nil {
				t.Fatal(err)
			}
			hook.fail = false
			afterUsers, err := st.DB.NewSelect().Model((*store.User)(nil)).Count(ctx)
			if err != nil || afterUsers != beforeUsers {
				t.Fatalf("SSO created accounts: %d -> %d (%v)", beforeUsers, afterUsers, err)
			}
			afterWorkspaces, err := st.DB.NewSelect().Model((*store.Workspace)(nil)).Count(ctx)
			if err != nil || afterWorkspaces != beforeWorkspaces {
				t.Fatalf("SSO created workspaces: %d -> %d (%v)", beforeWorkspaces, afterWorkspaces, err)
			}
			if tc.unknown || tc.lookupFailure || tc.disabled {
				afterSessions, err := st.DB.NewSelect().Model((*store.RefreshToken)(nil)).Count(ctx)
				if err != nil || afterSessions != beforeSessions {
					t.Fatal("denied identity persisted a refresh session")
				}
				if frag.Get("access_token") != "" || frag.Get("refresh_token") != "" || frag.Get("error") == "" {
					t.Fatal("denied identity received session or no error")
				}
				if tc.lookupFailure && frag.Get("error") != "cannot look up account" {
					t.Fatalf("lookup failure = %s", frag.Get("error"))
				}
				if tc.unknown {
					if _, err := st.GetUserByEmail(ctx, email); !errors.Is(err, sql.ErrNoRows) {
						t.Fatalf("unknown identity persisted: %v", err)
					}
				}
			} else {
				if frag.Get("access_token") == "" || frag.Get("refresh_token") == "" {
					t.Fatal("missing session")
				}
				claims, err := adminauth.VerifyAccess(frag.Get("access_token"))
				if err != nil || claims.Subject != u.ID.String() {
					t.Fatalf("wrong session identity: %v", err)
				}
			}
			memberships, err := st.ListMembershipsByUser(ctx, u.ID)
			if err != nil || len(memberships) != 1 || memberships[0].WorkspaceID != workspace.ID {
				t.Fatalf("SSO changed invited workspace: %v %v", memberships, err)
			}
		})
	}
}

type oidcUserLookupHook struct{ fail bool }

func (h *oidcUserLookupHook) BeforeQuery(ctx context.Context, event *bun.QueryEvent) context.Context {
	if h.fail && strings.Contains(event.Query, `FROM "users"`) && strings.Contains(event.Query, "email =") {
		ctx, cancel := context.WithCancel(ctx)
		cancel()
		return ctx
	}
	return ctx
}
func (*oidcUserLookupHook) AfterQuery(context.Context, *bun.QueryEvent) {}

func TestDashboardRejectsOrdinaryJWTWithStore(t *testing.T) {
	dbURL := os.Getenv("AIPROXY_TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("AIPROXY_TEST_DATABASE_URL not set")
	}
	t.Setenv("AIPROXY_JWT_SECRET", "test-jwt-secret-1234567890")
	ctx := context.Background()
	st, err := store.Open(ctx, dbURL)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err := st.MigrateUp(ctx); err != nil {
		t.Fatal(err)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte("pw"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatal(err)
	}
	u := &store.User{Email: fmt.Sprintf("dash-jwt-%d@example.com", time.Now().UnixNano()), PasswordHash: string(hash)}
	if err := st.CreateUser(ctx, u); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if existing, err := st.GetUserByEmail(context.Background(), u.Email); err == nil {
			_ = st.UpdateUser(context.Background(), &store.User{ID: existing.ID, Email: existing.Email, PasswordHash: existing.PasswordHash, Disabled: true})
		}
	})
	access, _, err := adminauth.IssueAccess(u.ID.String(), u.Email, false)
	if err != nil {
		t.Fatal(err)
	}
	rt := &config.Runtime{MultiTenancy: config.MultiTenancy{Enabled: true}}
	usage := accounting.NewAggregator()
	health := providerhealth.New(nil, config.ProviderHealth{})
	health.SetProviders(rt.Catalog)
	logs := observability.NewLogBuffer(10)
	deps := newDashboardDeps(rt, time.Now(), usage, health, logs)
	deps.MultiTenancy = rt.MultiTenancy
	deps.AdminStore = st
	deps.AdminAuthConfig = rt.Auth
	h := NewHandler(deps)

	req := httptest.NewRequest(http.MethodGet, "/_internal/dashboard/snapshot", nil)
	req.Header.Set("Authorization", "Bearer "+access)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("snapshot with ordinary JWT = %d, want 403", w.Code)
	}

	bad := httptest.NewRequest(http.MethodGet, "/_internal/dashboard/snapshot", nil)
	bad.Header.Set("Authorization", "Bearer garbage")
	bw := httptest.NewRecorder()
	h.ServeHTTP(bw, bad)
	if bw.Code != http.StatusUnauthorized {
		t.Fatalf("snapshot with bad token = %d, want 401", bw.Code)
	}
}
