package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/egose/aiproxy/internal/config"
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
