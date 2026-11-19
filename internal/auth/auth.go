package auth

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/identity"
	"github.com/egose/aiproxy/internal/store"
	"github.com/google/uuid"
)

type Principal struct {
	Name        string
	Tenant      string
	OwnerUserID uuid.UUID
	OwnerTeamID uuid.UUID

	credential identity.CredentialID
	workspace  identity.WorkspaceScope
}

func (p *Principal) CredentialID() identity.CredentialID {
	if p == nil {
		return identity.CredentialID{}
	}
	return p.credential
}

func (p *Principal) WorkspaceScope() identity.WorkspaceScope {
	if p == nil {
		return identity.WorkspaceScope{}
	}
	return p.workspace
}

func (p *Principal) DatabaseKey() (keyID, workspaceID uuid.UUID, ok bool) {
	keyID, isKey := p.CredentialID().DatabaseKeyID()
	workspaceID, scoped := p.WorkspaceScope().ID()
	return keyID, workspaceID, isKey && scoped
}

type DynamicClient struct {
	TokenHash     string
	ExpiresAt     time.Time
	Name          string
	Tenant        string
	AllowedModels []string
	KeyID         uuid.UUID
	Workspace     identity.WorkspaceScope
	OwnerUserID   uuid.UUID
	OwnerTeamID   uuid.UUID
}

func (c DynamicClient) credentialID() identity.CredentialID {
	if _, scoped := c.Workspace.ID(); !scoped || c.TokenHash == "" {
		return identity.CredentialID{}
	}
	return identity.DatabaseKeyID(c.KeyID)
}

type Authenticator interface {
	Authenticate(r *http.Request) (*Principal, error)
}

type Authorizer interface {
	Allow(principal *Principal, model string) bool
	AllowModelPassthrough(principal *Principal, provider, rule string) bool
}

type AuthorizerFunc func(principal *Principal, model string) bool

func (f AuthorizerFunc) Allow(principal *Principal, model string) bool {
	return f(principal, model)
}

func (AuthorizerFunc) AllowModelPassthrough(*Principal, string, string) bool {
	return false
}

type nopAuthenticator struct{}
type nopAuthorizer struct{}

func NewAuthenticator(cfg config.Auth) Authenticator {
	return NewAuthenticatorWithClients(cfg, nil)
}

func NewAuthenticatorWithClients(cfg config.Auth, dyn []DynamicClient) Authenticator {
	return NewAuthenticatorWithClientsAndClock(cfg, dyn, time.Now)
}

func NewAuthenticatorWithClientsAndClock(cfg config.Auth, dyn []DynamicClient, now func() time.Time) Authenticator {
	if cfg.Mode == config.AuthModeNone {
		return nopAuthenticator{}
	}
	if now == nil {
		now = time.Now
	}
	hashed := make(map[string]dynamicCredential, len(dyn))
	for _, d := range dyn {
		id := d.credentialID()
		if !id.Valid() {
			continue
		}
		hashed[d.TokenHash] = dynamicCredential{
			principal: Principal{
				Name: d.Name, Tenant: d.Tenant, OwnerUserID: d.OwnerUserID, OwnerTeamID: d.OwnerTeamID,
				credential: id, workspace: d.Workspace,
			},
			expiresAt: d.ExpiresAt,
		}
	}
	return &staticAuthenticator{tokens: staticTokens(cfg), hashed: hashed, now: now}
}

func staticTokens(cfg config.Auth) map[string]Principal {
	tokens := make(map[string]Principal, len(cfg.Clients))
	for name, c := range cfg.Clients {
		tokens[c.Token] = Principal{
			Name: name, Tenant: c.Tenant,
			credential: identity.StaticClientID(name), workspace: identity.InWorkspace(store.SystemWorkspaceID),
		}
	}
	return tokens
}

func NewAuthorizer(cfg config.Auth) Authorizer {
	return NewAuthorizerWithClients(cfg, nil)
}

func NewAuthorizerWithClients(cfg config.Auth, dyn []DynamicClient) Authorizer {
	if cfg.Mode == config.AuthModeNone {
		return nopAuthorizer{}
	}
	allowed := make(map[identity.CredentialID]map[string]struct{}, len(cfg.Clients)+len(dyn))
	passthroughGrants := make(map[identity.CredentialID]map[string]struct{})
	add := func(id identity.CredentialID, models []string) {
		if len(models) == 0 {
			allowed[id] = nil
			return
		}
		set := make(map[string]struct{}, len(models))
		for _, model := range models {
			set[model] = struct{}{}
		}
		allowed[id] = set
	}
	for name, c := range cfg.Clients {
		id := identity.StaticClientID(name)
		add(id, c.AllowedModels)
		if cfg.Mode == config.AuthModeBearerStatic && len(c.AllowedModelPassthroughs) > 0 {
			grants := make(map[string]struct{}, len(c.AllowedModelPassthroughs))
			for _, grant := range c.AllowedModelPassthroughs {
				grants[grant] = struct{}{}
			}
			passthroughGrants[id] = grants
		}
	}
	for _, d := range dyn {
		if id := d.credentialID(); id.Valid() {
			add(id, d.AllowedModels)
		}
	}
	return &staticAuthorizer{allowed: allowed, passthroughGrants: passthroughGrants}
}

func (nopAuthenticator) Authenticate(r *http.Request) (*Principal, error) {
	return nil, nil
}

func (nopAuthorizer) Allow(*Principal, string) bool {
	return true
}

func (nopAuthorizer) AllowModelPassthrough(principal *Principal, _, _ string) bool {
	return principal == nil
}

type staticAuthenticator struct {
	tokens map[string]Principal
	hashed map[string]dynamicCredential
	now    func() time.Time
}

type dynamicCredential struct {
	principal Principal
	expiresAt time.Time
}

type staticAuthorizer struct {
	allowed           map[identity.CredentialID]map[string]struct{}
	passthroughGrants map[identity.CredentialID]map[string]struct{}
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
		if credential, ok := s.hashed[store.TokenHash(token)]; ok {
			if !credential.expiresAt.IsZero() && !s.now().Before(credential.expiresAt) {
				return nil, ErrInvalidToken
			}
			matched := credential.principal
			return &matched, nil
		}
	}
	return nil, ErrInvalidToken
}

func (s *staticAuthorizer) Allow(principal *Principal, model string) bool {
	allowed, ok := s.allowed[principal.CredentialID()]
	if !ok {
		return false
	}
	if allowed == nil {
		return true
	}
	_, ok = allowed[model]
	return ok
}

func (s *staticAuthorizer) AllowModelPassthrough(principal *Principal, provider, rule string) bool {
	_, ok := s.passthroughGrants[principal.CredentialID()][provider+"/"+rule]
	return ok
}

var (
	ErrNoToken       = errors.New("missing Authorization header")
	ErrInvalidScheme = errors.New("unsupported auth scheme")
	ErrInvalidToken  = errors.New("invalid client token")
)
