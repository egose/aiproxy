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

export type Snapshot = z.infer<typeof snapshotSchema>;
export type Provider = z.infer<typeof providerSchema>;
export type Alias = z.infer<typeof aliasSchema>;
export type Usage = z.infer<typeof usageSchema>;
export type Recent = z.infer<typeof recentSchema>;
export type PayloadSummary = z.infer<typeof payloadSummarySchema>;
export type BlockSummary = z.infer<typeof blockSummarySchema>;
export type BlockCapture = z.infer<typeof blockCaptureSchema>;
export type TokenForm = z.infer<typeof tokenFormSchema>;
