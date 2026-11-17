import { z } from 'zod';
import { encryptedReasoningDeclarationSchema, type EncryptedReasoningDeclaration } from '../types';

export function normalizeReasoningPattern(value: string): string {
  const trimmed = value.replace(
    /^[\u0009-\u000d\u0020\u0085\u00a0\u1680\u2000-\u200a\u2028\u2029\u202f\u205f\u3000]+|[\u0009-\u000d\u0020\u0085\u00a0\u1680\u2000-\u200a\u2028\u2029\u202f\u205f\u3000]+$/gu,
    '',
  );
  return Array.from(trimmed, (rune) => (rune === '\u0130' ? 'i' : rune.toLowerCase())).join('');
}

export function reasoningDeclarationError(value: EncryptedReasoningDeclaration): string | undefined {
  if (value.on_caller_mismatch !== undefined && !['', 'fail', 'strip_and_retry'].includes(value.on_caller_mismatch)) {
    return 'Mismatch handling must be fail, strip_and_retry, or explicitly empty.';
  }
  const patterns = value.match_messages ?? [];
  if (patterns.length > 16) return 'Use at most 16 match messages.';
  const seen = new Set<string>();
  for (const raw of patterns) {
    const pattern = normalizeReasoningPattern(raw);
    const bytes = new TextEncoder().encode(pattern).length;
    if (bytes < 4 || bytes > 256)
      return 'Each match message must be 4–256 UTF-8 bytes after normalization; blank lines are invalid.';
    if (seen.has(pattern)) return 'Match messages must be unique after trimming and lowercasing.';
    seen.add(pattern);
  }
}

export const reasoningDraftSchema = z
  .object({
    mode: z.enum(['inherit', 'reset', 'custom']),
    declaration: encryptedReasoningDeclarationSchema,
  })
  .superRefine((value, ctx) => {
    if (value.mode !== 'custom') return;
    const message = reasoningDeclarationError(value.declaration);
    if (message) ctx.addIssue({ code: 'custom', message });
  });

export type ReasoningDraft = z.infer<typeof reasoningDraftSchema>;

export function reasoningDraft(value?: EncryptedReasoningDeclaration | null): ReasoningDraft {
  return {
    mode: value == null ? 'inherit' : Object.keys(value).length === 0 ? 'reset' : 'custom',
    declaration: value == null ? {} : structuredClone(value),
  };
}

export function reasoningDeclaration(value: ReasoningDraft): EncryptedReasoningDeclaration | null {
  if (value.mode === 'inherit') return null;
  if (value.mode === 'reset') return {};
  return structuredClone(value.declaration);
}
