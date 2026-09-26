import { z } from 'zod';

// Go marshals nil slices and maps as null, so every collection coming from
// the snapshot endpoints must tolerate null — not just undefined.
const arrayOf = <S extends z.ZodTypeAny>(schema: S) => z.array(schema).catch([]);

const boolMap = z.record(z.string(), z.boolean()).catch({});

export const modelPriceSchema = z.object({
  name: z.string(),
  input_per_million: z.number().nullable().optional(),
  output_per_million: z.number().nullable().optional(),
  cached_per_million: z.number().nullable().optional(),
  cache_write_per_million: z.number().nullable().optional(),
});

export const providerSchema = z.object({
  type: z.string(),
  name: z.string(),
  display_name: z.string().optional(),
  base_url: z.string().optional(),
  models: z.array(modelPriceSchema),
});

export const aliasTargetSchema = z.object({
  provider: z.string(),
  model: z.string(),
});

export const aliasSchema = z.object({
  name: z.string(),
  algorithm: z.string(),
  retry_status_codes: z.array(z.number()).optional(),
  targets: z.array(aliasTargetSchema),
});

export const usageSchema = z.object({
  Tenant: z.string().optional().default(''),
  Client: z.string().optional().default(''),
  Model: z.string(),
  Operation: z.string().optional().default(''),
  StatusCode: z.number().optional(),
  Count: z.number().optional().default(0),
  PromptTokens: z.number().optional().default(0),
  CompletionTokens: z.number().optional().default(0),
  TotalTokens: z.number().optional().default(0),
});

export const recentSchema = z.object({
  Timestamp: z.string().optional(),
  Tenant: z.string().optional(),
  Client: z.string().optional(),
  Model: z.string().optional().default(''),
  Operation: z.string().optional(),
  StatusCode: z.number().optional(),
  Provider: z.string().optional(),
  UpstreamModel: z.string().optional(),
  PromptTokens: z.number().optional(),
  CompletionTokens: z.number().optional(),
  TotalTokens: z.number().optional(),
  Duration: z.number().optional(),
});

export const cooldownSchema = z.object({
  alias: z.string(),
  provider: z.string(),
  model: z.string(),
  remaining_ms: z.number(),
});

export const healthcheckSchema = z.object({
  provider: z.string(),
  configured: z.boolean(),
  checked: z.boolean(),
  healthy: z.boolean(),
  status_code: z.number().optional(),
  message: z.string().optional(),
  path: z.string().optional(),
  last_checked: z.string().optional(),
});

export const logEntrySchema = z.object({
  seq: z.number().optional(),
  time: z.string().optional(),
  level: z.string().optional(),
  message: z.string().optional(),
  attrs: z.string().optional(),
});

export const snapshotSchema = z.object({
  version: z.string(),
  address: z.string(),
  auth_mode: z.string(),
  start_time: z.string(),
  now: z.string(),
  providers: arrayOf(providerSchema),
  disabled_providers: arrayOf(providerSchema),
  aliases: arrayOf(aliasSchema),
  health: boolMap,
  cooldowns: arrayOf(cooldownSchema),
  healthchecks: arrayOf(healthcheckSchema),
  usage: arrayOf(usageSchema),
  recent: arrayOf(recentSchema),
  logs: arrayOf(logEntrySchema),
  last_seq: z.number(),
  payload_enabled: z.boolean().optional().default(false),
});

export const payloadSummarySchema = z.object({
  request_id: z.string(),
  ts: z.string().optional(),
  method: z.string().optional(),
  path: z.string().optional(),
  status: z.number().optional(),
  public_model: z.string().optional(),
  provider: z.string().optional(),
  error: z.string().optional(),
});

export const payloadListSchema = z.object({
  enabled: z.boolean(),
  payloads: arrayOf(payloadSummarySchema),
});

export const blockSummarySchema = z.object({
  block_id: z.string(),
  ts: z.string(),
  operation: z.string().optional(),
  public_model: z.string().optional(),
  rule_ids: z.array(z.string()),
  finding_count: z.number(),
});

export const blockListSchema = z.object({
  enabled: z.boolean(),
  blocks: arrayOf(blockSummarySchema),
});

export const blockFindingSchema = z.object({
  rule_id: z.string(),
  description: z.string().optional(),
  secret: z.string(),
  secret_sha256: z.string().optional(),
  match: z.string().optional(),
  line: z.string().optional(),
});

export const blockCaptureSchema = z.object({
  block_id: z.string(),
  ts: z.string(),
  operation: z.string().optional(),
  public_model: z.string().optional(),
  rule_ids: z.array(z.string()),
  findings: z.array(blockFindingSchema),
});

export const tokenFormSchema = z.object({
  token: z.string().min(1, 'Paste the dashboard bearer token.'),
});

export const adminStatusSchema = z.object({
  multi_tenancy_enabled: z.boolean(),
  web_ui_enabled: z.boolean(),
  db_ok: z.boolean(),
  oidc_enabled: z.boolean().optional().default(false),
  oidc_display_name: z.string().optional().default(''),
  registration_enabled: z.boolean().optional().default(false),
});

const sourceSchema = z.enum(['config', 'database']).catch('config');

export const adminModelSchema = z.object({
  name: z.string(),
  display_name: z.string().optional().default(''),
  upstream_name: z.string().optional().default(''),
  protocol: z.string().optional().default(''),
  capabilities: z.array(z.string()).catch([]),
  pricing: z.record(z.string(), z.number()).optional().default({}),
});

export const adminHealthcheckSchema = z.object({
  path: z.string(),
  method: z.string().optional().default(''),
  expected_status: z.number().optional().default(0),
  expected_body: z.string().optional().default(''),
  interval: z.string().optional().default(''),
  timeout: z.string().optional().default(''),
  failure_threshold: z.number().optional().default(0),
  success_threshold: z.number().optional().default(0),
  send_authorization: z.boolean().optional().default(false),
});

export const adminProviderSchema = z.object({
  name: z.string(),
  type: z.string(),
  display_name: z.string().optional().default(''),
  base_url: z.string().optional().default(''),
  upstream_header_timeout: z.string().optional().default(''),
  user_agent: z.string().optional().default(''),
  forward_user_agent: z.boolean().optional().default(false),
  forward_headers: z.array(z.string()).catch([]),
  extends: z.string().optional().default(''),
  api_key_ref_path: z.string().optional().default(''),
  api_key_ref_key: z.string().optional().default(''),
  copilot_credential_path: z.string().optional().default(''),
  copilot_credential_name: z.string().optional().default(''),
  enabled: z.boolean().catch(true),
  source: sourceSchema,
  org_id: z.string().optional().default(''),
  org_name: z.string().optional().default(''),
  has_credential: z.boolean().catch(false),
  healthcheck: adminHealthcheckSchema.optional(),
  models: z.array(adminModelSchema).catch([]),
});

export const adminProvidersSchema = z.object({
  providers: arrayOf(adminProviderSchema),
});

export const adminAliasTargetSchema = z.object({
  provider: z.string(),
  model: z.string(),
});

export const adminAliasSchema = z.object({
  name: z.string(),
  algorithm: z.string(),
  retry_status_codes: z.array(z.number()).catch([]),
  session_affinity: z.object({ headers: z.array(z.string()).catch([]) }).optional(),
  encrypted_reasoning: z
    .object({
      passthrough: z.boolean().catch(true),
      on_caller_mismatch: z.string().optional().default(''),
      match_messages: z.array(z.string()).catch([]),
    })
    .optional(),
  source: sourceSchema,
  org_id: z.string().optional().default(''),
  org_name: z.string().optional().default(''),
  targets: z.array(adminAliasTargetSchema).catch([]),
});

export const adminAliasesSchema = z.object({
  aliases: arrayOf(adminAliasSchema),
});

export const adminKeyTPMSchema = z.object({
  model: z.string(),
  ceiling: z.number().catch(0),
  effective: z.number().catch(0),
  used_tokens: z.number().catch(0),
});

export const adminKeyUsageSchema = z.object({
  requests: z.number().catch(0),
  tokens: z.number().catch(0),
  spend_micros: z.number().catch(0),
});

export const adminKeyQuotaSchema = z.object({
  budget_micros: z.number().catch(0),
  spend_micros: z.number().catch(0),
  tpm: z.array(adminKeyTPMSchema).catch([]),
});

export const adminKeySchema = z.object({
  id: z.string().optional().default(''),
  name: z.string(),
  token_prefix: z.string().optional().default(''),
  tenant: z.string().optional().default(''),
  allowed_models: z.array(z.string()).catch([]),
  enabled: z.boolean().catch(true),
  source: sourceSchema,
  org_id: z.string().optional().default(''),
  org_name: z.string().optional().default(''),
  description: z.string().optional().default(''),
  expires_at: z.string().optional().default(''),
  user_ids: z.array(z.string()).optional().default([]),
  team_ids: z.array(z.string()).optional().default([]),
  owner_user_id: z.string().optional().default(''),
  owner_team_id: z.string().optional().default(''),
  owner_name: z.string().optional().default(''),
  can_manage: z.boolean().catch(false),
  usage: adminKeyUsageSchema.optional().default({ requests: 0, tokens: 0, spend_micros: 0 }),
  quota: adminKeyQuotaSchema.optional(),
});

export const adminKeysSchema = z.object({
  keys: arrayOf(adminKeySchema),
});

export const adminUserSchema = z.object({
  id: z.string(),
  email: z.string(),
  role: z.enum(['admin', 'user']).catch('user'),
  is_admin: z.boolean().catch(false),
  disabled: z.boolean().catch(false),
  source: sourceSchema.optional().default('database'),
  orgs: z.array(z.object({ id: z.string(), name: z.string(), role: z.string() })).catch([]),
});

export const adminUsersSchema = z.object({
  users: arrayOf(adminUserSchema),
});

export const adminInviteSchema = z.object({
  id: z.string(),
  email: z.string(),
  role: z.enum(['admin', 'user']).catch('user'),
  org_id: z.string().optional().default(''),
  org_name: z.string().optional().default(''),
  org_role: z.enum(['admin', 'member']).catch('member'),
  expires_at: z.string(),
});

export const adminInvitesSchema = z.object({
  invites: arrayOf(adminInviteSchema),
});

export const adminOrgSchema = z.object({
  id: z.string(),
  name: z.string(),
  display_name: z.string().optional().default(''),
  is_system: z.boolean().catch(false),
  role: z.enum(['admin', 'member']).optional().default('member'),
});

export const adminOrgsSchema = z.object({
  organizations: arrayOf(adminOrgSchema),
});

export const adminTeamSchema = z.object({
  id: z.string(),
  org_id: z.string(),
  name: z.string(),
  description: z.string().optional().default(''),
  members: z.number().optional().default(0),
  my_role: z.string().optional().default(''),
});

export const adminTeamsSchema = z.object({
  teams: arrayOf(adminTeamSchema),
});

export const adminOrgMemberSchema = z.object({
  user_id: z.string(),
  email: z.string(),
  role: z.enum(['admin', 'member']).catch('member'),
});

export const adminTeamMemberSchema = z.object({
  user_id: z.string(),
  email: z.string().optional().default(''),
  role: z.enum(['admin', 'member']).catch('member'),
});

export const adminQuotaTPMSchema = z.object({
  model: z.string(),
  ceiling: z.number().catch(0),
  effective: z.number().catch(0),
  used_tokens: z.number().catch(0),
});

export const adminQuotaSchema = z.object({
  budget_micros: z.number().catch(0),
  spend_micros: z.number().catch(0),
  tpm: z.array(adminQuotaTPMSchema).catch([]),
});

export const adminLoginSchema = z.object({
  email: z.string().min(1, 'Enter the admin email.'),
  password: z.string().min(1, 'Enter the password.'),
});

export const adminMeSchema = z.object({
  user_id: z.string(),
  email: z.string(),
  is_admin: z.boolean().catch(false),
});

export const adminOIDCConfigSchema = z.object({
  enabled: z.boolean(),
  issuer_url: z.string().optional().default(''),
  client_id: z.string().optional().default(''),
  has_client_secret: z.boolean().catch(false),
  scopes: z.string().optional().default(''),
  username_claim: z.string().optional().default(''),
  admin_claim: z.string().optional().default(''),
  admin_value: z.string().optional().default(''),
  display_name: z.string().optional().default(''),
  effective_enabled: z.boolean().catch(false),
});

export const providerTypeInfoSchema = z.object({
  type: z.string(),
  credential: z.string(),
  requires_base_url: z.boolean().catch(false),
  supports_healthcheck: z.boolean().catch(true),
  model_protocol_required: z.boolean().catch(false),
  protocols: z.array(z.string()).catch([]),
  default_capabilities: z.array(z.string()).catch([]),
  supported_capabilities: z.array(z.string()).catch([]),
});

export const providerTypesSchema = z.object({
  provider_types: arrayOf(providerTypeInfoSchema),
});

export type Snapshot = z.infer<typeof snapshotSchema>;
export type Provider = z.infer<typeof providerSchema>;
export type Alias = z.infer<typeof aliasSchema>;
export type Usage = z.infer<typeof usageSchema>;
export type Recent = z.infer<typeof recentSchema>;
export type PayloadSummary = z.infer<typeof payloadSummarySchema>;
export type PayloadList = z.infer<typeof payloadListSchema>;
export type BlockSummary = z.infer<typeof blockSummarySchema>;
export type BlockList = z.infer<typeof blockListSchema>;
export type BlockCapture = z.infer<typeof blockCaptureSchema>;
export type TokenForm = z.infer<typeof tokenFormSchema>;
export type AdminStatus = z.infer<typeof adminStatusSchema>;
export type AdminModel = z.infer<typeof adminModelSchema>;
export type AdminProvider = z.infer<typeof adminProviderSchema>;
export type AdminAlias = z.infer<typeof adminAliasSchema>;
export type AdminKey = z.infer<typeof adminKeySchema>;
export type AdminKeyQuota = z.infer<typeof adminKeyQuotaSchema>;
export type AdminKeyTPM = z.infer<typeof adminKeyTPMSchema>;
export type AdminUser = z.infer<typeof adminUserSchema>;
export type AdminInvite = z.infer<typeof adminInviteSchema>;
export type AdminOrg = z.infer<typeof adminOrgSchema>;
export type AdminTeam = z.infer<typeof adminTeamSchema>;
export type AdminOrgMember = z.infer<typeof adminOrgMemberSchema>;
export type AdminTeamMember = z.infer<typeof adminTeamMemberSchema>;
export type AdminQuota = z.infer<typeof adminQuotaSchema>;
export type AdminLogin = z.infer<typeof adminLoginSchema>;
export type AdminMe = z.infer<typeof adminMeSchema>;
export type AdminOIDCConfig = z.infer<typeof adminOIDCConfigSchema>;
export type ProviderTypeInfo = z.infer<typeof providerTypeInfoSchema>;
