import { ZodError } from 'zod';
import axios from 'axios';

import { setDashboardToken } from '../store';
import { assertAuthGeneration, authLifecycle } from '../auth-lifecycle';
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

export type DashboardCredential = 'session' | 'token';

function dataClient(credential: DashboardCredential | undefined) {
  if (credential === 'session') return adminClient;
  if (credential === 'token') return dashboardClient;
  throw new Error('Dashboard access required.');
}

export async function fetchSnapshot(credential: DashboardCredential | undefined): Promise<Snapshot> {
  const res = await dataClient(credential).get<unknown>(snapshotPath);
  const envelope = res.data as SnapshotEnvelope;
  return snapshotSchema.parse({ ...envelope, last_seq: envelope.last_seq ?? 0 });
}

export async function connectDashboardToken(token: string) {
  const generation = authLifecycle.generation;
  const res = await axios.get<unknown>(snapshotPath, {
    timeout: 10_000,
    headers: { Authorization: `Bearer ${token}` },
  });
  assertAuthGeneration(generation);
  const envelope = res.data as SnapshotEnvelope;
  snapshotSchema.parse({ ...envelope, last_seq: envelope.last_seq ?? 0 });
  setDashboardToken(token);
}

export async function fetchPayloads(
  credential: DashboardCredential | undefined,
  limit = 100,
  errorsOnly = false,
): Promise<PayloadList> {
  const res = await dataClient(credential).get<unknown>(payloadsPath, { params: { limit, errors_only: errorsOnly } });
  return payloadListSchema.parse(res.data);
}

export async function fetchPayload(credential: DashboardCredential | undefined, requestId: string): Promise<unknown> {
  const res = await dataClient(credential).get<unknown>(`${payloadsPath}/${requestId}`);
  return res.data;
}

export async function fetchBlocks(credential: DashboardCredential | undefined): Promise<BlockList> {
  const res = await dataClient(credential).get<unknown>(blocksPath);
  return blockListSchema.parse(res.data);
}

export async function fetchBlock(credential: DashboardCredential | undefined, blockId: string): Promise<BlockCapture> {
  const res = await dataClient(credential).get<unknown>(`${blocksPath}/${blockId}`);
  return blockCaptureSchema.parse(res.data);
}

export type BlockDecisionAction = 'allow' | 'redact' | 'deny';

export async function decideBlock(
  credential: DashboardCredential | undefined,
  blockId: string,
  action: BlockDecisionAction,
  findingShas: string[],
) {
  const res = await dataClient(credential).post(`${blocksPath}/${blockId}/decision`, {
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

export function errorMessage(err: unknown, mode?: DashboardCredential): string {
  if (err instanceof ZodError) {
    const details = err.issues.map((issue) => `${issue.path.join('.') || '(root)'}: ${issue.message}`).join('; ');
    return `Unexpected dashboard response (${details}).`;
  }
  if (typeof err !== 'object' || err === null) return 'Request failed.';
  if ('response' in err) {
    const resp = (err as { response?: { status?: number; data?: unknown } }).response;
    if (
      resp?.status === 403 &&
      typeof resp.data === 'string' &&
      resp.data.trim() === 'dashboard operator access required'
    ) {
      return mode === 'session'
        ? 'Operator access required. Global dashboard data and decisions are available only to system administrators.'
        : 'Operator access required. Global dashboard data and decisions are available only to system administrators or dashboard-token holders.';
    }
    if (resp?.status === 401) {
      return mode === 'token'
        ? 'Authentication failed. Check your dashboard token.'
        : 'Authentication failed. Sign in again.';
    }
    if (typeof resp?.data === 'string' && resp.data) return resp.data;
    if (resp?.status === 404) return 'Dashboard not configured on this server.';
    if (resp?.status === 429) return 'Rate limited, retry shortly.';
    if (resp?.status) return `Request failed with status ${resp.status}.`;
  }
  if ('message' in err && typeof (err as { message?: unknown }).message === 'string') {
    return (err as { message: string }).message;
  }
  return 'Request failed.';
}
