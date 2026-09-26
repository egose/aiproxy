package auth

import (
	"errors"
	"net/http"
	"strings"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/store"
	"github.com/google/uuid"
)

type Principal struct {
	Name        string
	Tenant      string
	KeyID       uuid.UUID
	OrgID       uuid.UUID
	OwnerUserID uuid.UUID
	OwnerTeamID uuid.UUID
}

type DynamicClient struct {
	TokenHash     string
	Name          string
	Tenant        string
	AllowedModels []string
	KeyID         uuid.UUID
	OrgID         uuid.UUID
	OwnerUserID   uuid.UUID
	OwnerTeamID   uuid.UUID
}

type Authenticator interface {
	Authenticate(r *http.Request) (*Principal, error)
}

type Authorizer interface {
	Allow(principal *Principal, model string) bool
}

type AuthorizerFunc func(principal *Principal, model string) bool

func (f AuthorizerFunc) Allow(principal *Principal, model string) bool {
	return f(principal, model)
}

type nopAuthenticator struct{}

func NewAuthenticator(cfg config.Auth) Authenticator {
	if cfg.Mode == config.AuthModeNone {
		return nopAuthenticator{}
	}
	return &staticAuthenticator{tokens: staticTokens(cfg), hashed: nil}
}

func NewAuthenticatorWithClients(cfg config.Auth, dyn []DynamicClient) Authenticator {
	if cfg.Mode == config.AuthModeNone {
		return nopAuthenticator{}
	}
	hashed := make(map[string]Principal, len(dyn))
	for _, d := range dyn {
		if d.TokenHash == "" {
			continue
		}
		hashed[d.TokenHash] = Principal{Name: d.Name, Tenant: d.Tenant, KeyID: d.KeyID, OrgID: d.OrgID, OwnerUserID: d.OwnerUserID, OwnerTeamID: d.OwnerTeamID}
	}
	return &staticAuthenticator{tokens: staticTokens(cfg), hashed: hashed}
}

func staticTokens(cfg config.Auth) map[string]Principal {
	tokens := make(map[string]Principal, len(cfg.Clients))
	for name, c := range cfg.Clients {
		tokens[c.Token] = Principal{Name: name, Tenant: c.Tenant}
	}
	return tokens
}

func NewAuthorizer(cfg config.Auth) Authorizer {
	if cfg.Mode == config.AuthModeNone {
		return AuthorizerFunc(func(*Principal, string) bool { return true })
	}
	clients, allowed := clientAllowLists(cfg, nil)
	return &staticAuthorizer{clients: clients, allowed: allowed}
}

func NewAuthorizerWithClients(cfg config.Auth, dyn []DynamicClient) Authorizer {
	if cfg.Mode == config.AuthModeNone {
		return AuthorizerFunc(func(*Principal, string) bool { return true })
	}
	clients, allowed := clientAllowLists(cfg, dyn)
	return &staticAuthorizer{clients: clients, allowed: allowed}
}

func clientAllowLists(cfg config.Auth, dyn []DynamicClient) (map[string]struct{}, map[string]map[string]struct{}) {
	allowed := make(map[string]map[string]struct{}, len(cfg.Clients)+len(dyn))
	clients := make(map[string]struct{}, len(cfg.Clients)+len(dyn))
	add := func(name string, models []string) {
		clients[name] = struct{}{}
		if len(models) == 0 {
			return
		}
		set := make(map[string]struct{}, len(models))
		for _, model := range models {
			set[model] = struct{}{}
		}
		allowed[name] = set
	}
	for name, c := range cfg.Clients {
		add(name, c.AllowedModels)
	}
	for _, d := range dyn {
		add(d.Name, d.AllowedModels)
	}
	return clients, allowed
}

func (nopAuthenticator) Authenticate(r *http.Request) (*Principal, error) {
	return nil, nil
}

type staticAuthenticator struct {
	tokens map[string]Principal
	hashed map[string]Principal
}

type staticAuthorizer struct {
	clients map[string]struct{}
	allowed map[string]map[string]struct{}
}

func (s *staticAuthenticator) Authenticate(r *http.Request) (*Principal, error) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return nil, ErrNoToken
	}
	const prefix = "Bearer "
	if !strings.HasPrefix(authHeader, prefix) {
		return nil, ErrInvalidScheme
	}
	token := strings.TrimSpace(strings.TrimPrefix(authHeader, prefix))
	if token == "" {
		return nil, ErrInvalidToken
	}
	principal, ok := s.tokens[token]
	if ok {
		matched := principal
		return &matched, nil
	}
	if len(s.hashed) > 0 {
		if principal, ok := s.hashed[store.TokenHash(token)]; ok {
			matched := principal
			return &matched, nil
		}
	}
	return nil, ErrInvalidToken
}

func (s *staticAuthorizer) Allow(principal *Principal, model string) bool {
	if principal == nil {
		return false
	}
	if _, ok := s.clients[principal.Name]; !ok {
		return false
	}
	allowed, ok := s.allowed[principal.Name]
	if !ok {
		return true
	}
	_, ok = allowed[model]
	return ok
}

var (
	ErrNoToken       = errors.New("missing Authorization header")
	ErrInvalidScheme = errors.New("unsupported auth scheme")
	ErrInvalidToken  = errors.New("invalid client token")
)
