package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/store"
	"github.com/google/uuid"
)

func TestNoneAuthenticator(t *testing.T) {
	a := NewAuthenticator(config.Auth{Mode: config.AuthModeNone})
	p, err := a.Authenticate(httptest.NewRequest(http.MethodGet, "/", nil))
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if p != nil {
		t.Errorf("expected nil principal for none mode")
	}
}

func TestBearerStaticAcceptsKnownToken(t *testing.T) {
	a := NewAuthenticator(config.Auth{
		Mode: config.AuthModeBearerStatic,
		Clients: map[string]config.Client{
			"ci": {Name: "ci", Token: "tok", Tenant: "team-a"},
		},
	})
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Authorization", "Bearer tok")
	p, err := a.Authenticate(r)
	if err != nil {
		t.Fatalf("auth: %v", err)
	}
	if p.Name != "ci" {
		t.Errorf("principal = %+v", p)
	}
	if p.Tenant != "team-a" {
		t.Errorf("tenant = %q", p.Tenant)
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
	a := NewAuthorizer(config.Auth{Mode: config.AuthModeBearerStatic, Clients: map[string]config.Client{
		"ci": {Name: "ci", Token: "tok", AllowedModels: []string{"openai/gpt-4o-mini", "alias/chat_default"}},
	}})
	if !a.Allow(&Principal{Name: "ci"}, "openai/gpt-4o-mini") {
		t.Fatal("expected direct model to be allowed")
	}
	if !a.Allow(&Principal{Name: "ci"}, "alias/chat_default") {
		t.Fatal("expected alias model to be allowed")
	}
	if a.Allow(&Principal{Name: "ci"}, "openai/gpt-4.1") {
		t.Fatal("expected unrelated model to be denied")
	}
}

func TestBearerStaticAuthorizerAllowsClientWithoutAllowedModels(t *testing.T) {
	a := NewAuthorizer(config.Auth{Mode: config.AuthModeBearerStatic, Clients: map[string]config.Client{
		"ci": {Name: "ci", Token: "tok"},
	}})
	if !a.Allow(&Principal{Name: "ci"}, "openai/gpt-4o-mini") {
		t.Fatal("expected unrestricted client to be allowed")
	}
}

func TestBearerStaticAuthorizerRejectsUnknownPrincipal(t *testing.T) {
	a := NewAuthorizer(config.Auth{Mode: config.AuthModeBearerStatic, Clients: map[string]config.Client{
		"ci": {Name: "ci", Token: "tok"},
	}})
	if a.Allow(&Principal{Name: "unknown"}, "openai/gpt-4o-mini") {
		t.Fatal("expected unknown principal to be denied")
	}
}

func TestDynamicClientsAuthenticateByHash(t *testing.T) {
	cfg := config.Auth{
		Mode: config.AuthModeBearerStatic,
		Clients: map[string]config.Client{
			"ci": {Name: "ci", Token: "tok"},
		},
	}
	raw := "dynamic-secret-token"
	dyn := []DynamicClient{{TokenHash: store.TokenHash(raw), Name: "db-key", Tenant: "team-b", AllowedModels: []string{"openai/gpt-4o-mini"}}}
	a := NewAuthenticatorWithClients(cfg, dyn)
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Authorization", "Bearer "+raw)
	p, err := a.Authenticate(r)
	if err != nil {
		t.Fatalf("auth: %v", err)
	}
	if p.Name != "db-key" || p.Tenant != "team-b" {
		t.Errorf("principal = %+v", p)
	}
	r2 := httptest.NewRequest(http.MethodGet, "/", nil)
	r2.Header.Set("Authorization", "Bearer tok")
	if _, err := a.Authenticate(r2); err != nil {
		t.Errorf("static token stopped working: %v", err)
	}
	r3 := httptest.NewRequest(http.MethodGet, "/", nil)
	r3.Header.Set("Authorization", "Bearer nope")
	if _, err := a.Authenticate(r3); err == nil {
		t.Errorf("bad token accepted")
	}
	z := NewAuthorizerWithClients(cfg, dyn)
	if !z.Allow(&Principal{Name: "db-key"}, "openai/gpt-4o-mini") {
		t.Errorf("allowed model rejected")
	}
	if z.Allow(&Principal{Name: "db-key"}, "other/model") {
		t.Errorf("unlisted model allowed")
	}
	if !z.Allow(&Principal{Name: "ci"}, "anything/at-all") {
		t.Errorf("unrestricted static client rejected")
	}
}

func TestDynamicClientExpirationWithoutReload(t *testing.T) {
	deadline := time.Date(2030, 1, 2, 3, 4, 5, 123456000, time.UTC)
	now := deadline.Add(-time.Nanosecond)
	want := Principal{Name: "expiring", Tenant: "tenant", KeyID: uuid.New(), WorkspaceID: uuid.New(), OwnerUserID: uuid.New(), OwnerTeamID: uuid.New()}
	dyn := []DynamicClient{
		{TokenHash: store.TokenHash("expiring"), ExpiresAt: deadline, Name: want.Name, Tenant: want.Tenant,
			KeyID: want.KeyID, WorkspaceID: want.WorkspaceID, OwnerUserID: want.OwnerUserID, OwnerTeamID: want.OwnerTeamID},
		{TokenHash: store.TokenHash("permanent"), Name: "permanent"},
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
	dyn := []DynamicClient{{TokenHash: store.TokenHash("expired"), Name: "expired", ExpiresAt: time.Unix(1, 0)}}
	cfg := config.Auth{Mode: config.AuthModeBearerStatic}
	for _, a := range []Authenticator{NewAuthenticatorWithClients(cfg, dyn), NewAuthenticatorWithClientsAndClock(cfg, dyn, nil)} {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("Authorization", "Bearer expired")
		if p, err := a.Authenticate(r); p != nil || err != ErrInvalidToken {
			t.Errorf("principal = %+v, error = %v; want nil, ErrInvalidToken", p, err)
		}
	}
}
