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
	return &staticAuthenticator{clients: cfg.Clients}
}

func NewAuthorizer(cfg config.Auth) Authorizer {
	if cfg.Mode == config.AuthModeNone {
		return AuthorizerFunc(func(*Principal, string) bool { return true })
	}
	return &staticAuthorizer{clients: cfg.Clients}
}

func (nopAuthenticator) Authenticate(r *http.Request) (*Principal, error) {
	return nil, nil
}

type staticAuthenticator struct {
	clients map[string]config.Client
}

type staticAuthorizer struct {
	clients map[string]config.Client
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
	for name, c := range s.clients {
		if c.Token == token {
			return &Principal{Name: name, Tenant: c.Tenant}, nil
		}
	}
	return nil, ErrInvalidToken
}

func (s *staticAuthorizer) Allow(principal *Principal, model string) bool {
	if principal == nil {
		return false
	}
	client, ok := s.clients[principal.Name]
	if !ok {
		return false
	}
	if len(client.AllowedModels) == 0 {
		return true
	}
	for _, allowed := range client.AllowedModels {
		if allowed == model {
			return true
		}
	}
	return false
}

var (
	ErrNoToken       = errors.New("missing Authorization header")
	ErrInvalidScheme = errors.New("unsupported auth scheme")
	ErrInvalidToken  = errors.New("invalid client token")
)
