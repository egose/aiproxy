import { describe, expect, it } from 'vitest';

import { blockCaptureSchema, blockListSchema, payloadListSchema, snapshotSchema } from './types';

// Field names mirror the Go structs verbatim: accounting.Summary/Event have
// no JSON tags (Capitalized keys), observability logs use time/message,
// payloadlog and dashrpc types use snake_case. If the backend renames a
// field, these tests fail fast instead of shipping silent empty tables.
const snapshotFixture = {
  version: 'test',
  address: ':8080',
  auth_mode: 'none',
  start_time: '2026-09-20T00:00:00Z',
  now: '2026-09-20T00:01:00Z',
  providers: [{ type: 'openai', name: 'openai', models: [{ name: 'gpt-4o-mini' }] }],
  aliases: [{ name: 'chat', algorithm: 'round_robin', targets: [{ provider: 'openai', model: 'gpt-4o-mini' }] }],
  health: { openai: true },
  cooldowns: [{ alias: 'chat', provider: 'openai', model: 'gpt-4o-mini', remaining_ms: 1500 }],
  healthchecks: [{ provider: 'openai', configured: false, checked: false, healthy: false }],
  usage: [
    {
      Tenant: '',
      Client: '',
      Model: 'openai/gpt-4o-mini',
      Operation: 'chat_completions',
      StatusCode: 200,
      Count: 3,
      PromptTokens: 10,
      CompletionTokens: 20,
      TotalTokens: 30,
    },
  ],
  recent: [
    {
      Timestamp: '2026-09-20T00:00:30Z',
      Model: 'openai/gpt-4o-mini',
      Operation: 'chat_completions',
      StatusCode: 200,
      Provider: 'openai',
      TotalTokens: 30,
    },
  ],
  logs: [{ time: '2026-09-20T00:00:01Z', level: 'INFO', message: 'starting', seq: 1 }],
  last_seq: 1,
  payload_enabled: false,
};

describe('dashboard contracts', () => {
  it('parses a representative snapshot', () => {
    const snap = snapshotSchema.parse(snapshotFixture);
    expect(snap.usage[0]?.Model).toBe('openai/gpt-4o-mini');
    expect(snap.usage[0]?.Count).toBe(3);
    expect(snap.recent[0]?.StatusCode).toBe(200);
    expect(snap.logs[0]?.message).toBe('starting');
  });

  it('tolerates null collections from Go nil slices', () => {
    const snap = snapshotSchema.parse({
      ...snapshotFixture,
      disabled_providers: null,
      aliases: null,
      health: null,
      cooldowns: null,
      healthchecks: null,
      usage: null,
      recent: null,
      logs: null,
    });
    expect(snap.disabled_providers).toEqual([]);
    expect(snap.aliases).toEqual([]);
    expect(snap.health).toEqual({});
    expect(snap.usage).toEqual([]);
    expect(snap.recent).toEqual([]);
    expect(snap.providers).toHaveLength(1);
  });

  it('parses payload and block lists', () => {
    const payloads = payloadListSchema.parse({
      enabled: true,
      payloads: [
        { request_id: 'r1', ts: '2026-09-20T00:00:00Z', method: 'POST', path: '/v1/chat/completions', status: 200 },
      ],
    });
    expect(payloads.payloads[0]?.request_id).toBe('r1');

    const blocks = blockListSchema.parse({
      enabled: true,
      blocks: [{ block_id: 'blk_abc', ts: '2026-09-20T00:00:00Z', rule_ids: ['r1'], finding_count: 1 }],
    });
    expect(blocks.blocks[0]?.finding_count).toBe(1);

    const capture = blockCaptureSchema.parse({
      ...blocks.blocks[0],
      findings: [{ rule_id: 'r1', secret: 's', secret_sha256: 'a'.repeat(64) }],
    });
    expect(capture.findings).toHaveLength(1);
  });
});
