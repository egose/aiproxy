package store

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type User struct {
	bun.BaseModel `bun:"table:users,alias:u"`
	ID            uuid.UUID `bun:"id,pk,type:uuid,default:gen_random_uuid()"`
	Email         string    `bun:"email,unique,notnull"`
	PasswordHash  string    `bun:"password_hash,notnull"`
	IsAdmin       bool      `bun:"is_admin,notnull,default:false"`
	Disabled      bool      `bun:"disabled,notnull,default:false"`
	CreatedAt     time.Time `bun:"created_at,notnull,default:now()"`
	UpdatedAt     time.Time `bun:"updated_at,notnull,default:now()"`
}

type RefreshToken struct {
	bun.BaseModel `bun:"table:refresh_tokens,alias:rt"`
	ID            uuid.UUID  `bun:"id,pk,type:uuid,default:gen_random_uuid()"`
	UserID        uuid.UUID  `bun:"user_id,notnull,type:uuid"`
	TokenHash     string     `bun:"token_hash,unique,notnull"`
	ExpiresAt     time.Time  `bun:"expires_at,notnull"`
	RevokedAt     *time.Time `bun:"revoked_at"`
	CreatedAt     time.Time  `bun:"created_at,notnull,default:now()"`
}

type DBProvider struct {
	bun.BaseModel         `bun:"table:db_providers,alias:p"`
	ID                    uuid.UUID       `bun:"id,pk,type:uuid,default:gen_random_uuid()"`
	Name                  string          `bun:"name,unique,notnull"`
	Type                  string          `bun:"type,notnull"`
	DisplayName           string          `bun:"display_name,notnull,default:''"`
	BaseURL               string          `bun:"base_url,notnull,default:''"`
	UpstreamTimeoutMs     *int64          `bun:"upstream_header_timeout_ms"`
	UserAgent             string          `bun:"user_agent,notnull,default:''"`
	ForwardUserAgent      bool            `bun:"forward_user_agent,notnull,default:false"`
	ForwardHeaders        []string        `bun:"forward_headers,type:jsonb,nullzero"`
	APIKeyEncrypted       []byte          `bun:"api_key_encrypted"`
	APIKeyRefPath         string          `bun:"api_key_ref_path,notnull,default:''"`
	APIKeyRefKey          string          `bun:"api_key_ref_key,notnull,default:''"`
	CopilotCredentialPath string          `bun:"copilot_credential_path,notnull,default:''"`
	CopilotCredentialName string          `bun:"copilot_credential_name,notnull,default:''"`
	Extends               string          `bun:"extends,notnull,default:''"`
	Enabled               bool            `bun:"enabled,notnull"`
	OrgID                 uuid.UUID       `bun:"org_id,notnull,type:uuid"`
	Healthcheck           json.RawMessage `bun:"healthcheck,type:jsonb,nullzero"`
	CreatedAt             time.Time       `bun:"created_at,notnull,default:now()"`
	UpdatedAt             time.Time       `bun:"updated_at,notnull,default:now()"`
}

type DBProviderModel struct {
	bun.BaseModel `bun:"table:db_provider_models,alias:m"`
	ID            uuid.UUID       `bun:"id,pk,type:uuid,default:gen_random_uuid()"`
	ProviderID    uuid.UUID       `bun:"provider_id,notnull,type:uuid"`
	Name          string          `bun:"name,notnull"`
	DisplayName   string          `bun:"display_name,notnull,default:''"`
	UpstreamName  string          `bun:"upstream_name,notnull,default:''"`
	Protocol      string          `bun:"protocol,notnull,default:''"`
	Capabilities  []string        `bun:"capabilities,type:jsonb,nullzero"`
	Pricing       json.RawMessage `bun:"pricing,type:jsonb,nullzero"`
}

type DBAlias struct {
	bun.BaseModel      `bun:"table:db_aliases,alias:a"`
	ID                 uuid.UUID       `bun:"id,pk,type:uuid,default:gen_random_uuid()"`
	Name               string          `bun:"name,unique,notnull"`
	Algorithm          string          `bun:"algorithm,notnull"`
	RetryStatusCodes   []int           `bun:"retry_status_codes,type:jsonb,nullzero"`
	SessionAffinity    json.RawMessage `bun:"session_affinity,type:jsonb,nullzero"`
	EncryptedReasoning json.RawMessage `bun:"encrypted_reasoning,type:jsonb,nullzero"`
	OrgID              uuid.UUID       `bun:"org_id,notnull,type:uuid"`
	CreatedAt          time.Time       `bun:"created_at,notnull,default:now()"`
	UpdatedAt          time.Time       `bun:"updated_at,notnull,default:now()"`
}

type DBAliasTarget struct {
	bun.BaseModel `bun:"table:db_alias_targets,alias:t"`
	ID            uuid.UUID `bun:"id,pk,type:uuid,default:gen_random_uuid()"`
	AliasID       uuid.UUID `bun:"alias_id,notnull,type:uuid"`
	Provider      string    `bun:"provider,notnull"`
	Model         string    `bun:"model,notnull"`
}

type InboundKey struct {
	bun.BaseModel `bun:"table:inbound_keys,alias:k"`
	ID            uuid.UUID  `bun:"id,pk,type:uuid,default:gen_random_uuid()"`
	Name          string     `bun:"name,unique,notnull"`
	TokenHash     string     `bun:"token_hash,unique,notnull"`
	TokenPrefix   string     `bun:"token_prefix,notnull"`
	Tenant        string     `bun:"tenant,notnull,default:''"`
	AllowedModels []string   `bun:"allowed_models,type:jsonb,nullzero"`
	Enabled       bool       `bun:"enabled,notnull"`
	OrgID         uuid.UUID  `bun:"org_id,notnull,type:uuid"`
	Description   string     `bun:"description,notnull,default:''"`
	ExpiresAt     *time.Time `bun:"expires_at"`
	OwnerUserID   *uuid.UUID `bun:"owner_user_id,type:uuid"`
	OwnerTeamID   *uuid.UUID `bun:"owner_team_id,type:uuid"`
	CreatedAt     time.Time  `bun:"created_at,notnull,default:now()"`
	UpdatedAt     time.Time  `bun:"updated_at,notnull,default:now()"`
}

type KeyUser struct {
	bun.BaseModel `bun:"table:key_users,alias:ku"`
	KeyID         uuid.UUID `bun:"key_id,pk,type:uuid"`
	UserID        uuid.UUID `bun:"user_id,pk,type:uuid"`
	CreatedAt     time.Time `bun:"created_at,notnull,default:now()"`
}

type KeyTeam struct {
	bun.BaseModel `bun:"table:key_teams,alias:kt"`
	KeyID         uuid.UUID `bun:"key_id,pk,type:uuid"`
	TeamID        uuid.UUID `bun:"team_id,pk,type:uuid"`
	CreatedAt     time.Time `bun:"created_at,notnull,default:now()"`
}

type OIDCConfig struct {
	bun.BaseModel         `bun:"table:oidc_configs,alias:o"`
	ID                    string    `bun:"id,pk"`
	Enabled               bool      `bun:"enabled,notnull,default:false"`
	IssuerURL             string    `bun:"issuer_url,notnull,default:''"`
	ClientID              string    `bun:"client_id,notnull,default:''"`
	ClientSecretEncrypted []byte    `bun:"client_secret_encrypted"`
	Scopes                string    `bun:"scopes,notnull,default:'openid email profile'"`
	UsernameClaim         string    `bun:"username_claim,notnull,default:'email'"`
	AdminClaim            string    `bun:"admin_claim,notnull,default:''"`
	AdminValue            string    `bun:"admin_value,notnull,default:''"`
	DisplayName           string    `bun:"display_name,notnull,default:'Single Sign-On'"`
	CreatedAt             time.Time `bun:"created_at,notnull,default:now()"`
	UpdatedAt             time.Time `bun:"updated_at,notnull,default:now()"`
}

type Invite struct {
	bun.BaseModel `bun:"table:invites,alias:i"`
	ID            uuid.UUID  `bun:"id,pk,type:uuid,default:gen_random_uuid()"`
	Email         string     `bun:"email,unique,notnull"`
	IsAdmin       bool       `bun:"is_admin,notnull,default:false"`
	TokenHash     string     `bun:"token_hash,unique,notnull"`
	ExpiresAt     time.Time  `bun:"expires_at,notnull"`
	AcceptedAt    *time.Time `bun:"accepted_at"`
	CreatedBy     *uuid.UUID `bun:"created_by,type:uuid"`
	OrgID         *uuid.UUID `bun:"org_id,type:uuid"`
	OrgRole       string     `bun:"org_role,notnull,default:'member'"`
	CreatedAt     time.Time  `bun:"created_at,notnull,default:now()"`
}

type Organization struct {
	bun.BaseModel `bun:"table:organizations,alias:o"`
	ID            uuid.UUID `bun:"id,pk,type:uuid"`
	Name          string    `bun:"name,unique,notnull"`
	DisplayName   string    `bun:"display_name,notnull,default:''"`
	IsSystem      bool      `bun:"is_system,notnull,default:false"`
	CreatedAt     time.Time `bun:"created_at,notnull,default:now()"`
	UpdatedAt     time.Time `bun:"updated_at,notnull,default:now()"`
}

type OrganizationMember struct {
	bun.BaseModel `bun:"table:organization_members,alias:om"`
	UserID        uuid.UUID `bun:"user_id,pk,type:uuid"`
	OrgID         uuid.UUID `bun:"org_id,pk,type:uuid"`
	Role          string    `bun:"role,notnull,default:'member'"`
	CreatedAt     time.Time `bun:"created_at,notnull,default:now()"`
}

type OrganizationTeam struct {
	bun.BaseModel `bun:"table:organization_teams,alias:ot"`
	ID            uuid.UUID `bun:"id,pk,type:uuid,default:gen_random_uuid()"`
	OrgID         uuid.UUID `bun:"org_id,notnull,type:uuid"`
	Name          string    `bun:"name,notnull"`
	Description   string    `bun:"description,notnull,default:''"`
	CreatedAt     time.Time `bun:"created_at,notnull,default:now()"`
	UpdatedAt     time.Time `bun:"updated_at,notnull,default:now()"`
}

type TeamMember struct {
	bun.BaseModel `bun:"table:team_members,alias:tm"`
	UserID        uuid.UUID `bun:"user_id,pk,type:uuid"`
	TeamID        uuid.UUID `bun:"team_id,pk,type:uuid"`
	Role          string    `bun:"role,notnull,default:'member'"`
	CreatedAt     time.Time `bun:"created_at,notnull,default:now()"`
}

type ScopeQuota struct {
	bun.BaseModel     `bun:"table:scope_quotas,alias:sq"`
	OrgID             uuid.UUID `bun:"org_id,pk,type:uuid"`
	ScopeType         string    `bun:"scope_type,pk"`
	ScopeID           uuid.UUID `bun:"scope_id,pk,type:uuid"`
	Model             string    `bun:"model,pk,default:''"`
	BudgetMicros      int64     `bun:"budget_micros,notnull,default:0"`
	TPMCeiling        int64     `bun:"tpm_ceiling,notnull,default:0"`
	TPMEffective      int64     `bun:"tpm_effective,notnull,default:0"`
	SpentOffsetMicros int64     `bun:"spent_offset_micros,notnull,default:0"`
	CreatedAt         time.Time `bun:"created_at,notnull,default:now()"`
	UpdatedAt         time.Time `bun:"updated_at,notnull,default:now()"`
}

type SpendEntry struct {
	bun.BaseModel `bun:"table:spend_ledger,alias:sl"`
	ID            uuid.UUID  `bun:"id,pk,type:uuid,default:gen_random_uuid()"`
	OrgID         uuid.UUID  `bun:"org_id,notnull,type:uuid"`
	KeyID         uuid.UUID  `bun:"key_id,notnull,type:uuid"`
	UserID        *uuid.UUID `bun:"user_id,type:uuid"`
	TeamID        *uuid.UUID `bun:"team_id,type:uuid"`
	Model         string     `bun:"model,notnull,default:''"`
	Tokens        int64      `bun:"tokens,notnull,default:0"`
	CostMicros    int64      `bun:"cost_micros,notnull,default:0"`
	CreatedAt     time.Time  `bun:"created_at,notnull,default:now()"`
}

type KeyUsage struct {
	KeyID    uuid.UUID
	Requests int64
	Tokens   int64
	Spend    int64
}
