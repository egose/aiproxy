import { ZodError } from 'zod';

import { adminStore } from '../store';
import { adminClient } from './admin';
import { dashboardClient } from './client';
import {
  blockCaptureSchema,
  blockListSchema,
  payloadListSchema,
  snapshotSchema,
  type BlockCapture,
  type BlockList,
  type PayloadList,
  type Snapshot,
} from '../types';

export const snapshotPath = '/_internal/dashboard/snapshot';
export const logsPath = '/_internal/dashboard/logs';
export const payloadsPath = '/_internal/dashboard/payloads';
export const blocksPath = '/_internal/dashboard/blocks';

export interface SnapshotEnvelope {
  last_seq: number;
  version: string;
  [key: string]: unknown;
}

// When signed in (multi-tenancy), the server accepts the admin JWT on the
// dashboard APIs, so no dashboard token is needed. Otherwise fall back to
// the dashboard bearer token.
function dataClient() {
  return adminStore.accessToken ? adminClient : dashboardClient;
}

export async function fetchSnapshot(): Promise<Snapshot> {
  const res = await dataClient().get<unknown>(snapshotPath);
  const envelope = res.data as SnapshotEnvelope;
  return snapshotSchema.parse({ ...envelope, last_seq: envelope.last_seq ?? 0 });
}

export async function fetchPayloads(limit = 100, errorsOnly = false): Promise<PayloadList> {
  const res = await dataClient().get<unknown>(payloadsPath, { params: { limit, errors_only: errorsOnly } });
  return payloadListSchema.parse(res.data);
}

export async function fetchPayload(requestId: string): Promise<unknown> {
  const res = await dataClient().get<unknown>(`${payloadsPath}/${requestId}`);
  return res.data;
}

export async function fetchBlocks(): Promise<BlockList> {
  const res = await dataClient().get<unknown>(blocksPath);
  return blockListSchema.parse(res.data);
}

export async function fetchBlock(blockId: string): Promise<BlockCapture> {
  const res = await dataClient().get<unknown>(`${blocksPath}/${blockId}`);
  return blockCaptureSchema.parse(res.data);
}

export type BlockDecisionAction = 'allow' | 'redact' | 'deny';

export async function decideBlock(blockId: string, action: BlockDecisionAction, findingShas: string[]) {
  const res = await dataClient().post(`${blocksPath}/${blockId}/decision`, {
    action,
    finding_shas: findingShas,
  });
  return res.data as { ok: boolean; action: string; count: number };
}

export function isUnauthorized(err: unknown): boolean {
  if (typeof err !== 'object' || err === null || !('response' in err)) return false;
  const status = (err as { response?: { status?: number } }).response?.status;
  return status === 401;
}

export function errorMessage(err: unknown): string {
  if (err instanceof ZodError) {
    const details = err.issues.map((issue) => `${issue.path.join('.') || '(root)'}: ${issue.message}`).join('; ');
    return `Unexpected dashboard response (${details}).`;
  }
  if (typeof err !== 'object' || err === null) return 'Request failed.';
  if ('response' in err) {
    const resp = (err as { response?: { status?: number; data?: unknown } }).response;
    if (typeof resp?.data === 'string' && resp.data) return resp.data;
    if (resp?.status === 401) return 'Unauthorized: dashboard token mismatch.';
    if (resp?.status === 404) return 'Dashboard not configured on this server.';
    if (resp?.status === 429) return 'Rate limited, retry shortly.';
    if (resp?.status) return `Request failed with status ${resp.status}.`;
  }
  if ('message' in err && typeof (err as { message?: unknown }).message === 'string') {
    return (err as { message: string }).message;
  }
  return 'Request failed.';
}
