import { act, cleanup, renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import type { PropsWithChildren } from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import * as hooks from './hooks';
import * as admin from './services/admin';
import * as dashboard from './services/dashboard';
import { authLifecycle } from './auth-lifecycle';
import {
  adminWorkspaceStore,
  clearAdminSession,
  refreshAdminSession,
  setAdminWorkspaceId,
  setAdminSession,
  setDashboardToken,
} from './store';
import { adminStatusSchema } from './types';

vi.mock('./services/admin', async (original) => {
  const module = await original<typeof admin>();
  return Object.fromEntries(
    Object.entries(module).map(([key, value]) => [key, key.startsWith('fetch') ? vi.fn() : value]),
  );
});
vi.mock('./services/dashboard', async (original) => {
  const module = await original<typeof dashboard>();
  return Object.fromEntries(
    Object.entries(module).map(([key, value]) => [key, key.startsWith('fetch') ? vi.fn() : value]),
  );
});

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

const cases = [
  ['me', () => hooks.useAdminMe(), admin.fetchAdminMe],
  ['providers', () => hooks.useAdminProviders(), admin.fetchAdminProviders],
  ['provider-types', () => hooks.useProviderTypes(), admin.fetchProviderTypes],
  ['aliases', () => hooks.useAdminAliases(), admin.fetchAdminAliases],
  ['keys', () => hooks.useAdminKeys(), admin.fetchAdminKeys],
  ['users', () => hooks.useAdminUsers(), admin.fetchAdminUsers],
  ['invites', () => hooks.useAdminInvites(), admin.fetchAdminInvites],
  ['oidc-config', () => hooks.useAdminOIDCConfig(), admin.fetchAdminOIDCConfig],
  ['workspaces', () => hooks.useAdminWorkspaces(), admin.fetchAdminWorkspaces],
  ['teams', () => hooks.useWorkspaceTeams('shared-ws'), admin.fetchWorkspaceTeams],
  ['members', () => hooks.useWorkspaceMembers('shared-ws'), admin.fetchWorkspaceMembers],
  ['quota', () => hooks.useScopeQuota('shared-ws', 'users', 'shared-id'), admin.fetchScopeQuota],
  ['team-members', () => hooks.useTeamMembers('shared-ws', 'shared-team'), admin.fetchTeamMembers],
  ['snapshot', () => hooks.useSnapshot(), dashboard.fetchSnapshot],
  ['payloads', () => hooks.usePayloads(), dashboard.fetchPayloads],
  ['payload', () => hooks.usePayload('shared-request'), dashboard.fetchPayload],
  ['blocks', () => hooks.useBlocks(), dashboard.fetchBlocks],
  ['block', () => hooks.useBlock('shared-block'), dashboard.fetchBlock],
] as const;

let client: QueryClient;
function wrapper({ children }: PropsWithChildren) {
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}

beforeEach(() => {
  vi.resetAllMocks();
  clearAdminSession();
  setDashboardToken('dashboard-secret-A');
  setAdminSession('access-secret-A', 'refresh-secret-A');
  setAdminWorkspaceId('ws-A');
  client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
  vi.mocked(admin.fetchAdminMe).mockResolvedValue({ user_id: 'A', email: 'A@example.com', is_admin: true });
});

afterEach(() => {
  cleanup();
  client.clear();
  clearAdminSession();
  setDashboardToken('');
});

describe('every sensitive query hook', () => {
  it.each(cases)(
    '%s isolates cached and delayed data across replacement, token, workspace and logout transitions',
    async (_name, useHook, fetcher) => {
      const mock = vi.mocked(fetcher);
      const old = { owner: 'A', is_admin: true };
      mock.mockResolvedValue(old as never);
      const { result, unmount } = renderHook(useHook, { wrapper });
      await waitFor(() => expect(result.current.data).toEqual(old));
      const oldKeys = client
        .getQueryCache()
        .getAll()
        .map((query) => query.queryKey);
      const held = deferred<never>();
      mock.mockReturnValueOnce(held.promise);
      let refetch!: Promise<unknown>;
      act(() => {
        refetch = result.current.refetch();
      });
      await waitFor(() => expect(mock).toHaveBeenCalledTimes(2));
      const next = deferred<never>();
      mock.mockReturnValue(next.promise);
      act(() => {
        setAdminSession('access-secret-B', 'refresh-secret-B');
      });
      expect(adminWorkspaceStore.workspaceId).toBe('');
      for (const key of oldKeys) expect(client.getQueryData(key)).toBeUndefined();
      await waitFor(() => expect(result.current.data).toBeUndefined());
      await act(async () => {
        held.resolve({ owner: 'late-A' } as never);
        await refetch;
      });
      expect(result.current.data).toBeUndefined();
      await act(async () => {
        next.resolve({ owner: 'B', is_admin: true } as never);
      });
      await waitFor(() => expect(result.current.data).toEqual({ owner: 'B', is_admin: true }));

      const sameGeneration = authLifecycle.generation;
      const calls = mock.mock.calls.length;
      act(() => {
        refreshAdminSession(authLifecycle.session, 'refreshed-secret-B', 'rotated-secret-B');
      });
      expect(authLifecycle.generation).toBe(sameGeneration);
      expect(result.current.data).toEqual({ owner: 'B', is_admin: true });
      expect(mock).toHaveBeenCalledTimes(calls);

      for (const transition of [
        () => setDashboardToken('dashboard-secret-B'),
        () => setAdminWorkspaceId('ws-B'),
        () => clearAdminSession(),
      ]) {
        const keys = client
          .getQueryCache()
          .getAll()
          .filter((q) => q.meta?.sensitive)
          .map((q) => q.queryKey);
        const pending = deferred<never>();
        mock.mockReturnValue(pending.promise);
        act(transition);
        for (const key of keys) expect(client.getQueryData(key)).toBeUndefined();
        await waitFor(() => expect(result.current.data).toBeUndefined());
      }
      const serialized = JSON.stringify(
        client
          .getQueryCache()
          .getAll()
          .map((query) => query.queryKey),
      );
      for (const token of [
        'access-secret',
        'refresh-secret',
        'dashboard-secret',
        'refreshed-secret',
        'rotated-secret',
      ]) {
        expect(serialized).not.toContain(token);
      }
      unmount();
      setAdminSession('C', 'C-refresh');
      expect(
        client
          .getQueryCache()
          .getAll()
          .filter((q) => q.meta?.sensitive),
      ).toEqual([]);
    },
  );

  it('retains public status while removing inactive sensitive cache entries', async () => {
    const status = adminStatusSchema.parse({ multi_tenancy_enabled: true, web_ui_enabled: true, db_ok: true });
    vi.mocked(admin.fetchAdminStatus).mockResolvedValue(status);
    const { result, unmount } = renderHook(() => ({ status: hooks.useAdminStatus(), me: hooks.useAdminMe() }), {
      wrapper,
    });
    await waitFor(() => expect(result.current.me.data?.user_id).toBe('A'));
    await waitFor(() => expect(result.current.status.data?.db_ok).toBe(true));
    unmount();
    clearAdminSession();
    expect(client.getQueryData(['admin', 'status'])).toEqual(status);
    expect(client.getQueryCache().getAll()).toHaveLength(1);
  });
});
