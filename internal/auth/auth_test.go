package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/identity"
	"github.com/egose/aiproxy/internal/store"
	"github.com/google/uuid"
)

func TestNoneAuthenticator(t *testing.T) {
	cfg := config.Auth{Mode: config.AuthModeNone}
	dyn := []DynamicClient{{
		TokenHash: store.TokenHash("db-fixture"), KeyID: uuid.New(),
		Workspace: identity.InWorkspace(store.SystemWorkspaceID),
	}}
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Authorization", "Bearer db-fixture")
	for _, a := range []Authenticator{NewAuthenticator(cfg), NewAuthenticatorWithClients(cfg, dyn)} {
		if p, err := a.Authenticate(r); err != nil || p != nil {
			t.Errorf("none mode returned principal = %+v, error = %v", p, err)
		}
	}
	for _, a := range []Authorizer{NewAuthorizer(cfg), NewAuthorizerWithClients(cfg, dyn)} {
		if !a.Allow(nil, "fixture/model") {
			t.Error("none mode denied anonymous model access")
		}
	}
}

func TestBearerStaticAcceptsKnownToken(t *testing.T) {
	a := NewAuthenticator(config.Auth{
		Mode: config.AuthModeBearerStatic,
		Clients: map[string]config.Client{
			"ci": {Name: "ci", Token: "tok", Tenant: "team-a"},
		},
	})
	p := authenticatedPrincipal(t, a, "tok")
	if p.Name != "ci" {
		t.Errorf("principal = %+v", p)
	}
	if p.Tenant != "team-a" {
		t.Errorf("tenant = %q", p.Tenant)
	}
	if p.CredentialID() != identity.StaticClientID("ci") {
		t.Error("file-only client lost its configured identity")
	}
}

func TestBearerStaticRejectsMissingHeader(t *testing.T) {
	a := NewAuthenticator(config.Auth{Mode: config.AuthModeBearerStatic, Clients: map[string]config.Client{"ci": {Token: "tok"}}})
	if _, err := a.Authenticate(httptest.NewRequest(http.MethodGet, "/", nil)); err == nil {
		t.Errorf("expected missing-header error")
	}
}

func TestBearerStaticRejectsWrongScheme(t *testing.T) {
	a := NewAuthenticator(config.Auth{Mode: config.AuthModeBearerStatic, Clients: map[string]config.Client{"ci": {Token: "tok"}}})
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Authorization", "Basic dXNlcjpwdw==")
	if _, err := a.Authenticate(r); err == nil {
		t.Errorf("expected wrong-scheme error")
	}
}

func TestBearerStaticRejectsBadToken(t *testing.T) {
	a := NewAuthenticator(config.Auth{Mode: config.AuthModeBearerStatic, Clients: map[string]config.Client{"ci": {Token: "tok"}}})
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Authorization", "Bearer wrong")
	if _, err := a.Authenticate(r); err == nil {
		t.Errorf("expected bad-token error")
	}
}

func TestBearerStaticAuthorizerAllowsConfiguredModels(t *testing.T) {
	cfg := config.Auth{Mode: config.AuthModeBearerStatic, Clients: map[string]config.Client{
		"ci": {Name: "ci", Token: "tok", AllowedModels: []string{"openai/gpt-4o-mini", "alias/chat_default"}},
	}}
	p := authenticatedPrincipal(t, NewAuthenticator(cfg), "tok")
	a := NewAuthorizer(cfg)
	if !a.Allow(p, "openai/gpt-4o-mini") {
		t.Fatal("expected direct model to be allowed")
	}
	if !a.Allow(p, "alias/chat_default") {
		t.Fatal("expected alias model to be allowed")
	}
	if a.Allow(p, "openai/gpt-4.1") {
		t.Fatal("expected unrelated model to be denied")
	}
}

func TestBearerStaticAuthorizerAllowsClientWithoutAllowedModels(t *testing.T) {
	cfg := config.Auth{Mode: config.AuthModeBearerStatic, Clients: map[string]config.Client{
		"ci": {Name: "ci", Token: "tok"},
	}}
	p := authenticatedPrincipal(t, NewAuthenticator(cfg), "tok")
	if !NewAuthorizer(cfg).Allow(p, "openai/gpt-4o-mini") {
		t.Fatal("expected unrestricted client to be allowed")
	}
}

func TestBearerStaticAuthorizerRejectsUnknownPrincipal(t *testing.T) {
	cfg := config.Auth{Mode: config.AuthModeBearerStatic, Clients: map[string]config.Client{
		"ci": {Name: "ci", Token: "tok"},
	}}
	unknown := config.Auth{Mode: config.AuthModeBearerStatic, Clients: map[string]config.Client{"unknown": {Token: "unknown"}}}
	a := NewAuthorizer(cfg)
	for _, p := range []*Principal{nil, {Name: "ci"}, authenticatedPrincipal(t, NewAuthenticator(unknown), "unknown")} {
		if a.Allow(p, "openai/gpt-4o-mini") {
			t.Fatal("expected anonymous, label-only, and unregistered principals to be denied")
		}
	}
}

func TestStaticAndDatabaseCredentialsHaveIndependentModelPolicies(t *testing.T) {
	cfg := config.Auth{
		Mode: config.AuthModeBearerStatic,
		Clients: map[string]config.Client{
			"shared": {Token: "file-fixture", Tenant: "label", AllowedModels: []string{"fixture/static"}},
		},
	}
	keyID, workspaceID := uuid.New(), uuid.New()
	dyn := []DynamicClient{{
		TokenHash: store.TokenHash("db-fixture"), Name: "shared", Tenant: "label",
		AllowedModels: []string{"alias/managed"}, KeyID: keyID, Workspace: identity.InWorkspace(workspaceID),
	}}
	a := NewAuthenticatorWithClients(cfg, dyn)
	file := authenticatedPrincipal(t, a, "file-fixture")
	db := authenticatedPrincipal(t, a, "db-fixture")
	if db.Name != "shared" || db.Tenant != "label" || file.Name != db.Name || file.Tenant != db.Tenant {
		t.Fatal("authentication changed the reporting labels")
	}
	if id, workspace, ok := db.DatabaseKey(); id != keyID || workspace != workspaceID || !ok {
		t.Fatalf("managed principal = (%v, %v, %v)", id, workspace, ok)
	}

	relabeled := *db
	relabeled.Name, relabeled.Tenant = "new-name", "new-label"
	for _, policy := range []struct {
		file, db           []string
		fileOther, dbOther bool
	}{
		{file: cfg.Clients["shared"].AllowedModels, db: dyn[0].AllowedModels},
		{file: cfg.Clients["shared"].AllowedModels, dbOther: true},
		{fileOther: true, db: dyn[0].AllowedModels},
	} {
		client := cfg.Clients["shared"]
		client.AllowedModels = policy.file
		cfg.Clients["shared"] = client
		dyn[0].AllowedModels = policy.db
		z := NewAuthorizerWithClients(cfg, dyn)
		if !z.Allow(file, "fixture/static") || z.Allow(file, "alias/managed") != policy.fileOther ||
			!z.Allow(db, "alias/managed") || z.Allow(db, "fixture/static") != policy.dbOther {
			t.Fatalf("file/DB allowlists did not stay independent: %+v", policy)
		}
		if !z.Allow(&relabeled, "alias/managed") || z.Allow(&relabeled, "fixture/static") != policy.dbOther {
			t.Fatal("reporting label edit changed authorization")
		}
	}
}

func TestModelPassthroughGrantsAreSeparateAndSnapshotLocal(t *testing.T) {
	grants := []string{"p/family"}
	cfg := config.Auth{Mode: config.AuthModeBearerStatic, Clients: map[string]config.Client{
		"shared":    {Token: "file-token", AllowedModels: []string{"p/literal"}, AllowedModelPassthroughs: grants},
		"ungranted": {Token: "ungranted-token", AllowedModels: []string{"p/family", "p/requested"}},
		"unlimited": {Token: "unlimited-token"},
	}}
	keys := []DynamicClient{{
		TokenHash: store.TokenHash("database-token"), Name: "shared", AllowedModels: []string{"p/family", "p/requested"},
		KeyID: uuid.New(), Workspace: identity.InWorkspace(store.SystemWorkspaceID),
	}}
	a := NewAuthenticatorWithClients(cfg, keys)
	z := NewAuthorizerWithClients(cfg, keys)
	granted := authenticatedPrincipal(t, a, "file-token")
	if !z.AllowModelPassthrough(granted, "p", "family") || !z.Allow(granted, "p/literal") ||
		z.Allow(granted, "p/family") || z.Allow(granted, "p/requested") || z.AllowModelPassthrough(granted, "other", "family") {
		t.Fatal("literal allowlist and provider-local passthrough grant are not independent")
	}
	for _, token := range []string{"database-token", "ungranted-token", "unlimited-token"} {
		if z.AllowModelPassthrough(authenticatedPrincipal(t, a, token), "p", "family") {
			t.Fatalf("%s received a rule grant", token)
		}
	}
	if z.AllowModelPassthrough(nil, "p", "family") || z.AllowModelPassthrough(&Principal{Name: "shared"}, "p", "family") {
		t.Fatal("anonymous or label-only principal received a static grant")
	}

	grants[0] = "p/new-family"
	refreshed := NewAuthorizerWithClients(cfg, keys)
	if !z.AllowModelPassthrough(granted, "p", "family") || z.AllowModelPassthrough(granted, "p", "new-family") ||
		refreshed.AllowModelPassthrough(granted, "p", "family") || !refreshed.AllowModelPassthrough(granted, "p", "new-family") {
		t.Fatal("rebuilding policy changed a previous snapshot")
	}

	nop := NewAuthorizer(config.Auth{Mode: config.AuthModeNone})
	if !nop.AllowModelPassthrough(nil, "p", "family") || nop.AllowModelPassthrough(granted, "p", "family") ||
		nop.AllowModelPassthrough(authenticatedPrincipal(t, a, "database-token"), "p", "family") ||
		NewAuthorizer(config.Auth{}).AllowModelPassthrough(nil, "p", "family") ||
		AuthorizerFunc(func(*Principal, string) bool { return true }).AllowModelPassthrough(nil, "p", "family") {
		t.Fatal("only configured no-auth policy can opt in an anonymous caller")
	}
}

func TestWorkspaceScopeDistinguishesSystemFromAbsent(t *testing.T) {
	system := identity.InWorkspace(store.SystemWorkspaceID)
	systemKey := DynamicClient{TokenHash: store.TokenHash("system"), KeyID: uuid.New(), Workspace: system}
	dyn := []DynamicClient{
		systemKey,
		{TokenHash: store.TokenHash("unscoped"), KeyID: uuid.New()},
		{TokenHash: store.TokenHash("no-id"), Workspace: system},
	}
	cfg := config.Auth{Mode: config.AuthModeBearerStatic, Clients: map[string]config.Client{"static": {Token: "static"}}}
	a := NewAuthenticatorWithClients(cfg, dyn)
	p := authenticatedPrincipal(t, a, "system")
	if keyID, workspaceID, ok := p.DatabaseKey(); !ok || keyID != systemKey.KeyID || workspaceID != store.SystemWorkspaceID ||
		p.WorkspaceScope() != system {
		t.Fatalf("authenticated System key lost scope: (%v, %v, %v)", keyID, workspaceID, ok)
	}
	file := authenticatedPrincipal(t, a, "static")
	if _, _, ok := file.DatabaseKey(); ok || file.WorkspaceScope() != system {
		t.Fatal("static client must belong to System without becoming a database key")
	}
	for _, token := range []string{"unscoped", "no-id", "unknown"} {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("Authorization", "Bearer "+token)
		if principal, err := a.Authenticate(r); principal != nil || err != ErrInvalidToken {
			t.Errorf("%s: principal = %+v, error = %v; want nil, ErrInvalidToken", token, principal, err)
		}
	}
	for _, principal := range []*Principal{nil, {Name: "system", Tenant: "system"}} {
		if _, scoped := principal.WorkspaceScope().ID(); scoped || principal.CredentialID().Valid() || principal.WorkspaceScope() == system {
			t.Error("absent or label-only principal acquired a System identity")
		}
	}
}

func TestDynamicClientExpirationWithoutReload(t *testing.T) {
	deadline := time.Date(2030, 1, 2, 3, 4, 5, 123456000, time.UTC)
	now := deadline.Add(-time.Nanosecond)
	keyID := uuid.New()
	want := Principal{Name: "expiring", Tenant: "tenant", OwnerUserID: uuid.New(), OwnerTeamID: uuid.New(),
		credential: identity.DatabaseKeyID(keyID), workspace: identity.InWorkspace(uuid.New())}
	dyn := []DynamicClient{
		{TokenHash: store.TokenHash("expiring"), ExpiresAt: deadline, Name: want.Name, Tenant: want.Tenant,
			KeyID: keyID, Workspace: want.workspace, OwnerUserID: want.OwnerUserID, OwnerTeamID: want.OwnerTeamID},
		{TokenHash: store.TokenHash("permanent"), Name: "permanent", KeyID: uuid.New(), Workspace: want.workspace},
	}
	cfg := config.Auth{Mode: config.AuthModeBearerStatic, Clients: map[string]config.Client{"static": {Token: "static"}}}
	a := NewAuthenticatorWithClientsAndClock(cfg, dyn, func() time.Time { return now })
	dyn[0].ExpiresAt = time.Time{}
	dyn[0].Name = "mutated"
	cfg.Clients["static"] = config.Client{Token: "mutated"}
	for _, offset := range []time.Duration{-time.Nanosecond, 0, time.Nanosecond, 24 * time.Hour} {
		t.Run(offset.String(), func(t *testing.T) {
			now = deadline.Add(offset)
			for _, token := range []string{"expiring", "permanent", "static", "unknown"} {
				r := httptest.NewRequest(http.MethodGet, "/", nil)
				r.Header.Set("Authorization", "Bearer "+token)
				p, err := a.Authenticate(r)
				if token == "unknown" || token == "expiring" && offset >= 0 {
					if p != nil || err != ErrInvalidToken {
						t.Errorf("%s: principal = %+v, error = %v; want nil, ErrInvalidToken", token, p, err)
					}
					continue
				}
				if err != nil || p == nil || p.Name != token {
					t.Fatalf("%s: principal = %+v, error = %v", token, p, err)
				}
				if token == "expiring" && *p != want {
					t.Errorf("principal = %+v, want %+v", *p, want)
				}
				p.Name = "mutated returned principal"
			}
		})
	}
}

func TestDynamicClientExpirationDefaultClock(t *testing.T) {
	dyn := []DynamicClient{{TokenHash: store.TokenHash("expired"), Name: "expired", ExpiresAt: time.Unix(1, 0),
		KeyID: uuid.New(), Workspace: identity.InWorkspace(store.SystemWorkspaceID)}}
	cfg := config.Auth{Mode: config.AuthModeBearerStatic}
	for _, a := range []Authenticator{NewAuthenticatorWithClients(cfg, dyn), NewAuthenticatorWithClientsAndClock(cfg, dyn, nil)} {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("Authorization", "Bearer expired")
		if p, err := a.Authenticate(r); p != nil || err != ErrInvalidToken {
			t.Errorf("principal = %+v, error = %v; want nil, ErrInvalidToken", p, err)
		}
	}
}

func authenticatedPrincipal(t *testing.T, a Authenticator, token string) *Principal {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	p, err := a.Authenticate(r)
	if err != nil || p == nil {
		t.Fatalf("auth = %+v, %v; want an authenticated principal", p, err)
	}
	return p
}
