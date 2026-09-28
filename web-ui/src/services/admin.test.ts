import { beforeEach, describe, expect, it, vi } from 'vitest';

const mocks = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
  put: vi.fn(),
  delete: vi.fn(),
  plainGet: vi.fn(),
  plainPost: vi.fn(),
}));

vi.mock('axios', () => ({
  default: {
    create: () => ({
      get: mocks.get,
      post: mocks.post,
      put: mocks.put,
      delete: mocks.delete,
      interceptors: { request: { use: vi.fn() }, response: { use: vi.fn() } },
    }),
    get: mocks.plainGet,
    post: mocks.plainPost,
  },
}));

import {
  cancelCopilotDeviceFlow,
  copilotDeviceFlowTimeout,
  createAdminAlias,
  createAdminKey,
  createAdminProvider,
  createAdminUser,
  fetchAdminAliases,
  fetchAdminKeys,
  fetchAdminProviders,
  fetchAdminUsers,
  fetchCopilotDeviceFlow,
  pollCopilotDeviceFlow,
  startCopilotDeviceFlow,
} from './admin';

beforeEach(() => {
  vi.clearAllMocks();
});

describe('admin service envelopes', () => {
  it('unwraps provider/alias/key/user lists to arrays', async () => {
    mocks.get.mockImplementation((url: string) => {
      if (url.endsWith('/providers')) return Promise.resolve({ data: { providers: [{ name: 'p', type: 'openai' }] } });
      if (url.endsWith('/aliases'))
        return Promise.resolve({ data: { aliases: [{ name: 'a', algorithm: 'round_robin' }] } });
      if (url.endsWith('/keys')) return Promise.resolve({ data: { keys: [{ name: 'k' }] } });
      if (url.endsWith('/users')) return Promise.resolve({ data: { users: [{ id: '1', email: 'a@b.c' }] } });
      throw new Error(`unexpected GET ${url}`);
    });

    const providers = await fetchAdminProviders();
    expect(Array.isArray(providers)).toBe(true);
    expect(providers).toHaveLength(1);

    const aliases = await fetchAdminAliases();
    expect(Array.isArray(aliases)).toBe(true);

    const keys = await fetchAdminKeys();
    expect(Array.isArray(keys)).toBe(true);

    const users = await fetchAdminUsers();
    expect(Array.isArray(users)).toBe(true);
  });

  it('returns single created items, not envelopes', async () => {
    mocks.post.mockImplementation((url: string) => {
      if (url.endsWith('/providers')) return Promise.resolve({ data: { name: 'p', type: 'openai' } });
      if (url.endsWith('/aliases')) return Promise.resolve({ data: { name: 'a', algorithm: 'round_robin' } });
      if (url.endsWith('/keys')) return Promise.resolve({ data: { key: { name: 'k' }, token: 'sekret' } });
      if (url.endsWith('/users')) return Promise.resolve({ data: { id: '1', email: 'a@b.c' } });
      throw new Error(`unexpected POST ${url}`);
    });

    expect(await createAdminProvider({ name: 'p' })).toMatchObject({ name: 'p' });
    expect(await createAdminAlias({ name: 'a' })).toMatchObject({ name: 'a' });
    const created = await createAdminKey({ name: 'k' });
    expect(created.key).toMatchObject({ name: 'k' });
    expect(created.token).toBe('sekret');
    expect(await createAdminUser({ email: 'a@b.c' })).toMatchObject({ email: 'a@b.c' });
  });

  it('starts/polls device flows with a 25s timeout and sanitized status only', async () => {
    expect(copilotDeviceFlowTimeout).toBe(25_000);
    const pending = {
      id: 'flow-1',
      workspace_id: 'ws-1',
      status: 'pending',
      user_code: 'ABCD-1234',
      verification_uri: 'https://github.com/login/device',
      expires_at: '2026-09-27T11:00:00Z',
      poll_after_ms: 5000,
    };
    mocks.post.mockImplementation((url: string, _body: unknown, config: unknown) => {
      expect((config as { timeout?: number }).timeout).toBe(25_000);
      if (url.endsWith('/copilot-device-flows')) return Promise.resolve({ data: pending });
      if (url.endsWith('/poll')) return Promise.resolve({ data: { ...pending, status: 'ready' } });
      throw new Error(`unexpected POST ${url}`);
    });
    mocks.get.mockResolvedValue({ data: pending });
    mocks.delete.mockResolvedValue({ data: { ...pending, status: 'cancelled' } });
    const started = await startCopilotDeviceFlow({ client_id: 'Iv1.test' });
    expect(started.user_code).toBe('ABCD-1234');
    expect(started).not.toHaveProperty('device_code');
    expect(started).not.toHaveProperty('access_token');
    expect(JSON.stringify(started)).not.toContain('token');
    const polled = await pollCopilotDeviceFlow('flow-1');
    expect(polled.status).toBe('ready');
    expect(await fetchCopilotDeviceFlow('flow-1')).toMatchObject({ id: 'flow-1' });
    expect(await cancelCopilotDeviceFlow('flow-1')).toMatchObject({ status: 'cancelled' });
  });
});
