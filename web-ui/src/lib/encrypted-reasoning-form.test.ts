import { describe, expect, it } from 'vitest';
import { adminAliasSchema, adminModelSchema, adminProviderSchema, encryptedReasoningEffectiveSchema } from '../types';
import {
  normalizeReasoningPattern,
  reasoningDeclaration,
  reasoningDeclarationError,
  reasoningDraft,
  reasoningDraftSchema,
} from './encrypted-reasoning-form';

describe('sparse policy decoding and form intent', () => {
  it.each([
    ['provider', (value: object) => adminProviderSchema.parse({ name: 'p', type: 'openai', ...value })],
    ['model', (value: object) => adminModelSchema.parse({ name: 'm', ...value })],
    ['alias', (value: object) => adminAliasSchema.parse({ name: 'a', algorithm: 'round_robin', ...value })],
  ] as const)('%s distinguishes absent, null, reset and sparse members without defaults', (_, decode) => {
    expect(decode({})).not.toHaveProperty('encrypted_reasoning');
    for (const local of [null, {}, { passthrough: false }, { on_caller_mismatch: '' }, { match_messages: [] }]) {
      expect(decode({ encrypted_reasoning: local }).encrypted_reasoning).toEqual(local);
    }
  });

  it('requires complete normalized effective values but never supplies local declaration defaults', () => {
    expect(encryptedReasoningEffectiveSchema.safeParse({}).success).toBe(false);
    expect(encryptedReasoningEffectiveSchema.safeParse({ passthrough: true }).success).toBe(false);
    const effective = { passthrough: false, on_caller_mismatch: 'strip_and_retry', match_messages: ['base message'] };
    const view = adminProviderSchema.parse({
      name: 'p',
      type: 'openai',
      encrypted_reasoning: {},
      encrypted_reasoning_effective: effective,
      encrypted_reasoning_source: 'root',
      encrypted_reasoning_inherited: false,
      models_inherited: true,
    });
    expect(view.encrypted_reasoning).toEqual({});
    expect(view.encrypted_reasoning_effective).toEqual(effective);
    expect(view.models_inherited).toBe(true);
  });

  it.each([
    {},
    { passthrough: false },
    { on_caller_mismatch: '' },
    { match_messages: [] },
    { passthrough: true, on_caller_mismatch: 'fail', match_messages: [] },
    { match_messages: ['  RAW Pattern ', 'a\nmultiline pattern'] },
  ])('round trips exact local members %j', (local) => {
    const draft = reasoningDraft(local);
    expect(reasoningDeclaration(draft)).toEqual(local);
    expect(reasoningDraftSchema.safeParse(draft).success).toBe(true);
  });

  it('keeps reset distinct from an all-default declaration and removes only on explicit inherit', () => {
    expect(reasoningDraft({}).mode).toBe('reset');
    const explicit = reasoningDraft({ passthrough: true, on_caller_mismatch: 'fail', match_messages: [] });
    expect(explicit.mode).toBe('custom');
    expect(reasoningDeclaration({ ...explicit, mode: 'reset' })).toEqual({});
    expect(reasoningDeclaration({ ...explicit, mode: 'inherit' })).toBeNull();
    expect(reasoningDraft(undefined).mode).toBe('inherit');
    expect(reasoningDraft(null).mode).toBe('inherit');
  });
});

describe('Go-compatible pattern validation', () => {
  it('uses Unicode simple per-rune lowercasing and Go TrimSpace rather than JS contextual casing/trim', () => {
    expect(normalizeReasoningPattern('\u0085ΟΣİ \u0085')).toBe('οσi');
    expect(normalizeReasoningPattern('\ufeffABCD\ufeff')).toBe('\ufeffabcd\ufeff');
    expect(reasoningDeclarationError({ match_messages: ['ΟΣİabcd', 'οσiabcd'] })).toMatch(/unique/);
    expect(reasoningDeclarationError({ match_messages: ['\u0085ABCD', 'abcd'] })).toMatch(/unique/);
  });

  it.each([
    { match_messages: ['abc'] },
    { match_messages: ['abcd', ' ABCD '] },
    { match_messages: ['abcd', '', 'efgh'] },
    { match_messages: ['\u0085 \u3000'] },
    { match_messages: ['a'.repeat(257)] },
    { match_messages: ['😀'.repeat(65)] },
    { match_messages: Array.from({ length: 17 }, (_, i) => `message ${i}`) },
    { on_caller_mismatch: ' fail ' },
  ])('rejects invalid declaration %j', (local) => {
    expect(reasoningDeclarationError(local)).toBeTruthy();
    expect(reasoningDraftSchema.safeParse(reasoningDraft(local)).success).toBe(false);
  });

  it.each([
    { match_messages: [] },
    { match_messages: ['😀'] },
    { match_messages: ['😀'.repeat(64)] },
    { match_messages: ['a'.repeat(256)] },
    { match_messages: ['İ'.repeat(256)] },
    { match_messages: Array.from({ length: 16 }, (_, i) => `message ${i}`) },
    { on_caller_mismatch: '' },
  ])('accepts normalized UTF-8/count boundaries %j', (local) => {
    expect(reasoningDeclarationError(local)).toBeUndefined();
  });
});
