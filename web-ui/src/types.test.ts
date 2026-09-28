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
  it('preserves bounded recent metadata flags and opaque identities without changing old rows', () => {
    const event = {
      ...snapshotFixture.recent[0],
      RequestID: 'prefix',
      RecentSequence: '18446744073709551615',
      ProviderID: '42',
      Truncated: {
        RequestID: true,
        PublicModel: true,
        Tenant: true,
        Client: true,
        Model: true,
        Operation: true,
        Provider: true,
        UpstreamModel: true,
      },
    };
    expect(snapshotSchema.parse({ ...snapshotFixture, recent: [event] }).recent[0]).toEqual(event);
    const old = snapshotSchema.parse(snapshotFixture).recent[0];
    expect(old).toEqual(snapshotFixture.recent[0]);
    expect(old?.Truncated).toBeUndefined();
    expect(old?.RecentSequence).toBeUndefined();
    expect(old?.ProviderID).toBeUndefined();
  });
  it('preserves optional completion/log correlation IDs and cache tokens', () => {
    const event = {
      ...snapshotFixture.recent[0],
      RequestID: 'req-one',
      PublicModel: 'alias/chat',
      Tenant: 'team',
      Client: 'cli',
      UpstreamModel: 'resolved',
      Duration: 100,
      CachedTokens: 2,
      CacheCreationTokens: 3,
      CacheReadTokens: 4,
    };
    const log = { ...snapshotFixture.logs[0], request_id: 'req-one' };
    const snap = snapshotSchema.parse({ ...snapshotFixture, recent: [event], logs: [log] });
    expect(snap.recent[0]).toEqual(event);
    expect(snap.logs[0]).toEqual(log);
    const old = snapshotSchema.parse(snapshotFixture);
    expect(old.recent[0]?.RequestID).toBeUndefined();
    expect(old.logs[0]?.request_id).toBeUndefined();
  });
  it('preserves diagnostics and distinguishes absent metadata from disabled configuration', () => {
    const details = {
      display_name: 'Model',
      upstream_name: 'upstream',
      protocol: 'messages',
      capabilities: ['chat', 'responses'],
    };
    const diagnostics = {
      header_timeout: '30s',
      probe: {
        path: '/health',
        method: 'GET',
        expected_status: 200,
        interval: '10s',
        timeout: '2s',
        failure_threshold: 3,
        success_threshold: 1,
      },
    };
    const affinity = { enabled: true, headers: ['x-session-id'] };
    const snap = snapshotSchema.parse({
      ...snapshotFixture,
      providers: [{ ...snapshotFixture.providers[0], diagnostics, models: [{ name: 'm', details }] }],
      aliases: [{ ...snapshotFixture.aliases[0], session_affinity: affinity }],
      healthchecks: [{ ...snapshotFixture.healthchecks[0], last_checked: snapshotFixture.now }],
    });
    expect(snap.providers[0]?.diagnostics).toEqual(diagnostics);
    expect(snap.providers[0]?.models[0]?.details).toEqual(details);
    expect(snap.aliases[0]?.session_affinity).toEqual(affinity);
    expect(snap.healthchecks[0]?.last_checked).toBe(snapshotFixture.now);
    const old = snapshotSchema.parse(snapshotFixture);
    expect(old.providers[0]?.diagnostics).toBeUndefined();
    expect(old.providers[0]?.models[0]?.details).toBeUndefined();
    expect(old.aliases[0]?.session_affinity).toBeUndefined();
    for (const metadata of [null, { enabled: false, headers: null }]) {
      const parsed = snapshotSchema.parse({
        ...snapshotFixture,
        aliases: [{ ...snapshotFixture.aliases[0], session_affinity: metadata }],
      });
      expect(parsed.aliases[0]?.session_affinity).toEqual(metadata === null ? null : { enabled: false, headers: [] });
    }
  });

  it('parses a representative snapshot', () => {
    const snap = snapshotSchema.parse(snapshotFixture);
    expect(snap.usage[0]?.Model).toBe('openai/gpt-4o-mini');
    expect(snap.usage[0]?.Count).toBe(3);
    expect(snap.recent[0]?.StatusCode).toBe(200);
    expect(snap.logs[0]?.message).toBe('starting');
    expect(snap.rates).toBeUndefined();
    expect(snap.billing).toBeUndefined();
  });

  it('preserves additive rate windows and retained public attribution', () => {
    const counts = { requests: 1000, errors: 100, throttled: 50, tokens: 100000 };
    const rates = {
      window_end: snapshotFixture.now,
      bucket_seconds: 1,
      minute: counts,
      five_minutes: { ...counts, requests: 5000 },
      minutes: Array.from({ length: 15 }, () => counts),
    };
    const billing = {
      as_of: snapshotFixture.now,
      retention_seconds: 86400,
      bucket_seconds: 60,
      usage: snapshotFixture.usage,
      upstream: [
        {
          ...snapshotFixture.usage[0],
          PublicModel: 'alias/chat',
          Provider: 'openai',
          Model: 'gpt-4o-mini',
          CachedTokens: 3,
          CacheCreationTokens: 1,
          CacheReadTokens: 2,
        },
      ],
    };
    const snap = snapshotSchema.parse({ ...snapshotFixture, rates, billing });
    expect(snap.rates).toEqual(rates);
    expect(snap.billing?.upstream[0]).toMatchObject(billing.upstream[0]);
    expect(snap.billing?.retention_seconds).toBe(86400);
    const empty = snapshotSchema.parse({
      ...snapshotFixture,
      rates: null,
      billing: { ...billing, usage: null, upstream: null },
    });
    expect(empty.rates).toBeNull();
    expect(empty.billing?.upstream).toEqual([]);
    expect(() => snapshotSchema.parse({ ...snapshotFixture, rates: { ...rates, minute: {} } })).toThrow();
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
