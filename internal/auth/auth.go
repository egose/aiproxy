package auth

import (
	"errors"
	"net/http"
	"strings"

	"github.com/egose/aiproxy/internal/config"
)

type Principal struct {
	Name   string
	Tenant string
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
	tokens := make(map[string]Principal, len(cfg.Clients))
	for name, c := range cfg.Clients {
		tokens[c.Token] = Principal{Name: name, Tenant: c.Tenant}
	}
	return &staticAuthenticator{tokens: tokens}
}

func NewAuthorizer(cfg config.Auth) Authorizer {
	if cfg.Mode == config.AuthModeNone {
		return AuthorizerFunc(func(*Principal, string) bool { return true })
	}
	allowed := make(map[string]map[string]struct{}, len(cfg.Clients))
	clients := make(map[string]struct{}, len(cfg.Clients))
	for name, c := range cfg.Clients {
		clients[name] = struct{}{}
		if len(c.AllowedModels) == 0 {
			continue
		}
		models := make(map[string]struct{}, len(c.AllowedModels))
		for _, model := range c.AllowedModels {
			models[model] = struct{}{}
		}
		allowed[name] = models
	}
	return &staticAuthorizer{clients: clients, allowed: allowed}
}

func (nopAuthenticator) Authenticate(r *http.Request) (*Principal, error) {
	return nil, nil
}

type staticAuthenticator struct {
	tokens map[string]Principal
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
