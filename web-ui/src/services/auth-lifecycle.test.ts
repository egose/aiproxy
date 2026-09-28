import axios, { AxiosError, type AxiosAdapter, type AxiosResponse, type InternalAxiosRequestConfig } from 'axios';
import { act, cleanup, renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { createElement, type PropsWithChildren } from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import * as admin from './admin';
import * as dashboard from './dashboard';
import { dashboardClient } from './client';
import { authLifecycle } from '../auth-lifecycle';
import { useAdminMe } from '../hooks';
import {
  adminWorkspaceStore,
  adminStore,
  clearAdminSession,
  dashboardStore,
  setAdminWorkspaceId,
  setAdminSession,
  setDashboardToken,
} from '../store';

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: unknown) => void;
  const promise = new Promise<T>((done, fail) => {
    resolve = done;
    reject = fail;
  });
  return { promise, resolve, reject };
}

function response(config: InternalAxiosRequestConfig, data: unknown, status = 200): AxiosResponse {
  return { config, data, status, statusText: String(status), headers: {} };
}

function unauthorized(config: InternalAxiosRequestConfig) {
  return new AxiosError('unauthorized', 'ERR_BAD_REQUEST', config, undefined, response(config, {}, 401));
}

const originals = {
  admin: admin.adminClient.defaults.adapter,
  dashboard: dashboardClient.defaults.adapter,
  axios: axios.defaults.adapter,
};
beforeEach(() => {
  setDashboardToken('');
  setAdminSession('access-A', 'refresh-A');
  setAdminWorkspaceId('ws-A');
});
afterEach(() => {
  cleanup();
  admin.adminClient.defaults.adapter = originals.admin;
  dashboardClient.defaults.adapter = originals.dashboard;
  axios.defaults.adapter = originals.axios;
  vi.restoreAllMocks();
  clearAdminSession();
  setDashboardToken('');
});

const adminCalls = [
  ['fetchAdminMe', () => admin.fetchAdminMe()],
  ['fetchAdminProviders', () => admin.fetchAdminProviders()],
  ['fetchProviderTypes', () => admin.fetchProviderTypes()],
  ['createAdminProvider', () => admin.createAdminProvider({})],
  ['updateAdminProvider', () => admin.updateAdminProvider('id', {})],
  ['deleteAdminProvider', () => admin.deleteAdminProvider('id')],
  ['setProviderCredential', () => admin.setProviderCredential('id', {})],
  ['fetchAdminAliases', () => admin.fetchAdminAliases()],
  ['createAdminAlias', () => admin.createAdminAlias({})],
  ['updateAdminAlias', () => admin.updateAdminAlias('id', {})],
  ['deleteAdminAlias', () => admin.deleteAdminAlias('id')],
  ['fetchAdminKeys', () => admin.fetchAdminKeys()],
  ['createAdminKey', () => admin.createAdminKey({})],
  ['rotateAdminKey', () => admin.rotateAdminKey('id')],
  ['revokeAdminKey', () => admin.revokeAdminKey('id')],
  ['deleteAdminKey', () => admin.deleteAdminKey('id')],
  ['fetchAdminKey', () => admin.fetchAdminKey('id')],
  ['updateAdminKey', () => admin.updateAdminKey('id', {})],
  ['fetchAdminUsers', () => admin.fetchAdminUsers()],
  ['createAdminUser', () => admin.createAdminUser({})],
  ['setUserRole', () => admin.setUserRole('id', 'user')],
  ['resetUserPassword', () => admin.resetUserPassword('id', 'password')],
  ['deleteAdminUser', () => admin.deleteAdminUser('id')],
  ['setUserDisabled', () => admin.setUserDisabled('id', true)],
  ['fetchAdminInvites', () => admin.fetchAdminInvites()],
  ['createAdminInvite', () => admin.createAdminInvite({})],
  ['deleteAdminInvite', () => admin.deleteAdminInvite('id')],
  ['fetchAdminOIDCConfig', () => admin.fetchAdminOIDCConfig()],
  ['updateAdminOIDCConfig', () => admin.updateAdminOIDCConfig({})],
  ['fetchAdminWorkspaces', () => admin.fetchAdminWorkspaces()],
  ['createAdminWorkspace', () => admin.createAdminWorkspace({})],
  ['fetchAdminWorkspace', () => admin.fetchAdminWorkspace('id')],
  ['updateAdminWorkspace', () => admin.updateAdminWorkspace('id', {})],
  ['deleteAdminWorkspace', () => admin.deleteAdminWorkspace('id')],
  ['fetchWorkspaceTeams', () => admin.fetchWorkspaceTeams('ws-A')],
  ['createWorkspaceTeam', () => admin.createWorkspaceTeam('ws-A', {})],
  ['updateWorkspaceTeam', () => admin.updateWorkspaceTeam('ws-A', 'id', {})],
  ['deleteWorkspaceTeam', () => admin.deleteWorkspaceTeam('ws-A', 'id')],
  ['fetchWorkspaceMembers', () => admin.fetchWorkspaceMembers('ws-A')],
  ['addWorkspaceMember', () => admin.addWorkspaceMember('ws-A', {})],
  ['setWorkspaceMemberRole', () => admin.setWorkspaceMemberRole('ws-A', 'id', 'admin')],
  ['removeWorkspaceMember', () => admin.removeWorkspaceMember('ws-A', 'id')],
  ['fetchTeamMembers', () => admin.fetchTeamMembers('ws-A', 'id')],
  ['addTeamMember', () => admin.addTeamMember('ws-A', 'id', {})],
  ['removeTeamMember', () => admin.removeTeamMember('ws-A', 'id', 'user')],
  ['setTeamMemberRole', () => admin.setTeamMemberRole('ws-A', 'id', 'user', 'member')],
  ['fetchScopeQuota', () => admin.fetchScopeQuota('ws-A', 'users', 'id')],
  ['updateScopeQuota', () => admin.updateScopeQuota('ws-A', 'users', 'id', {})],
  ['startCopilotDeviceFlow', () => admin.startCopilotDeviceFlow({ client_id: 'test' })],
  ['fetchCopilotDeviceFlow', () => admin.fetchCopilotDeviceFlow('id')],
  ['pollCopilotDeviceFlow', () => admin.pollCopilotDeviceFlow('id')],
  ['cancelCopilotDeviceFlow', () => admin.cancelCopilotDeviceFlow('id')],
] as const;

const dashboardCalls = [
  ['fetchSnapshot', () => dashboard.fetchSnapshot()],
  ['fetchPayloads', () => dashboard.fetchPayloads()],
  ['fetchPayload', () => dashboard.fetchPayload('id')],
  ['fetchBlocks', () => dashboard.fetchBlocks()],
  ['fetchBlock', () => dashboard.fetchBlock('id')],
  ['decideBlock', () => dashboard.decideBlock('id', 'deny', [])],
] as const;

describe('all authenticated service entry points', () => {
  it('enumerates every admin service, with explicit public/session exceptions', () => {
    const functions = Object.entries(admin)
      .filter(([, value]) => typeof value === 'function')
      .map(([name]) => name);
    expect(functions.sort()).toEqual(
      [
        ...adminCalls.map(([name]) => name),
        'adminClient',
        'adminLogin',
        'adminLogout',
        'fetchAdminStatus',
        'publicRegister',
      ].sort(),
    );
  });

  it.each(adminCalls)('%s aborts and rejects a delayed result after account replacement', async (_name, call) => {
    const held = deferred<AxiosResponse>();
    let config!: InternalAxiosRequestConfig;
    admin.adminClient.defaults.adapter = (request) => {
      config = request;
      return held.promise;
    };
    const pending = call();
    const rejected = expect(pending).rejects.toSatisfy(axios.isCancel);
    expect(config.headers.Authorization).toBe('Bearer access-A');
    expect(config.headers[admin.adminWorkspaceHeader]).toBe('ws-A');
    setAdminSession('access-B', 'refresh-B');
    expect(config.signal?.aborted).toBe(true);
    held.resolve(response(config, { secret: 'A', token: 'one-time-A' }));
    await rejected;
    expect(adminStore.accessToken).toBe('access-B');
  });

  for (const credential of ['session', 'token']) {
    it.each(dashboardCalls)(`%s rejects delayed dashboard ${credential} data and decisions`, async (_name, call) => {
      if (credential === 'token') setDashboardToken('token-A');
      const held = deferred<AxiosResponse>();
      let config!: InternalAxiosRequestConfig;
      const client = credential === 'token' ? dashboardClient : admin.adminClient;
      client.defaults.adapter = (request) => {
        config = request;
        return held.promise;
      };
      const pending = call();
      const rejected = expect(pending).rejects.toSatisfy(axios.isCancel);
      expect(config.headers.Authorization).toBe(`Bearer ${credential === 'token' ? 'token-A' : 'access-A'}`);
      setDashboardToken('token-B');
      expect(config.signal?.aborted).toBe(true);
      held.resolve(response(config, { secret: 'A' }));
      await rejected;
    });
  }

  it('rejects delayed workspace data after changing the selected workspace', async () => {
    const held = deferred<AxiosResponse>();
    let config!: InternalAxiosRequestConfig;
    admin.adminClient.defaults.adapter = (request) => {
      config = request;
      return held.promise;
    };
    const pending = admin.fetchAdminKeys();
    const rejected = expect(pending).rejects.toSatisfy(axios.isCancel);
    setAdminWorkspaceId('ws-B');
    held.resolve(response(config, { keys: [] }));
    await rejected;
    expect(config.signal?.aborted).toBe(true);
  });
});

describe('refresh ownership and authentication entry points', () => {
  it('isolates the mounted query through A → refresh failure → B with delayed B data', async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const wrapper = ({ children }: PropsWithChildren) => createElement(QueryClientProvider, { client }, children);
    admin.adminClient.defaults.adapter = async (config) =>
      response(config, { user_id: 'A', email: 'A@example.com', is_admin: true });
    const { result } = renderHook(() => useAdminMe(), { wrapper });
    await waitFor(() => expect(result.current.data?.user_id).toBe('A'));
    const oldKey = client.getQueryCache().getAll()[0].queryKey;
    const heldRefresh = deferred<AxiosResponse>();
    const refreshAdapter = vi.fn<AxiosAdapter>(() => heldRefresh.promise);
    axios.defaults.adapter = refreshAdapter;
    admin.adminClient.defaults.adapter = async (config) => {
      throw unauthorized(config);
    };
    let refetch!: Promise<unknown>;
    act(() => {
      refetch = result.current.refetch();
    });
    await waitFor(() => expect(refreshAdapter).toHaveBeenCalledTimes(1));
    await act(async () => {
      heldRefresh.reject(new Error('revoked'));
      await refetch;
    });
    expect(adminStore.accessToken).toBe('');
    expect(result.current.data).toBeUndefined();
    expect(client.getQueryData(oldKey)).toBeUndefined();
    const heldB = deferred<AxiosResponse>();
    let configB!: InternalAxiosRequestConfig;
    admin.adminClient.defaults.adapter = (config) => {
      configB = config;
      return heldB.promise;
    };
    act(() => {
      setAdminSession('access-B', 'refresh-B');
    });
    await waitFor(() => expect(configB).toBeDefined());
    expect(result.current.data).toBeUndefined();
    await act(async () => {
      heldB.resolve(response(configB, { user_id: 'B', email: 'B@example.com', is_admin: false }));
    });
    await waitFor(() => expect(result.current.data?.user_id).toBe('B'));
    expect(result.current.data?.is_admin).toBe(false);
    expect(client.getQueryData(oldKey)).toBeUndefined();
    client.clear();
  });

  function beginRefresh() {
    const held = deferred<AxiosResponse>();
    let refreshConfig!: InternalAxiosRequestConfig;
    axios.defaults.adapter = (config) => {
      refreshConfig = config;
      return held.promise;
    };
    admin.adminClient.defaults.adapter = async (config) => {
      throw unauthorized(config);
    };
    const pending = admin.fetchAdminMe();
    const rejected = expect(pending).rejects.toBeDefined();
    return { held, pending, rejected, config: () => refreshConfig };
  }

  it.each(['success', 'failure'])('ignores delayed A refresh %s after direct B replacement', async (outcome) => {
    const run = beginRefresh();
    await vi.waitFor(() => expect(run.config()).toBeDefined());
    setAdminSession('access-B', 'refresh-B');
    setAdminWorkspaceId('ws-B');
    if (outcome === 'success')
      run.held.resolve(response(run.config(), { access_token: 'late-A', refresh_token: 'late-refresh-A' }));
    else run.held.reject(new Error('refresh failed'));
    await run.rejected;
    expect(adminStore.accessToken).toBe('access-B');
    expect(adminStore.refreshToken).toBe('refresh-B');
    expect(adminWorkspaceStore.workspaceId).toBe('ws-B');
    expect(localStorage.getItem('aiproxy.admin-access-token')).toBe('access-B');
  });

  it('A refresh failure clears the session and workspace before B signs in', async () => {
    const run = beginRefresh();
    await vi.waitFor(() => expect(run.config()).toBeDefined());
    const generation = authLifecycle.generation;
    run.held.reject(new Error('refresh revoked'));
    await run.rejected;
    expect(adminStore.accessToken).toBe('');
    expect(adminWorkspaceStore.workspaceId).toBe('');
    expect(authLifecycle.generation).toBeGreaterThan(generation);
    expect(localStorage.getItem('aiproxy.admin-access-token')).toBeNull();
    setAdminSession('access-B', 'refresh-B');
    expect(adminStore.accessToken).toBe('access-B');
  });

  it('coalesces concurrent 401s and retries late 401s with the refreshed token without cache churn', async () => {
    const held = deferred<AxiosResponse>();
    const late = deferred<AxiosResponse>();
    let lateConfig!: InternalAxiosRequestConfig;
    let refreshConfig!: InternalAxiosRequestConfig;
    const refreshAdapter = vi.fn<AxiosAdapter>((config) => {
      refreshConfig = config;
      return held.promise;
    });
    axios.defaults.adapter = refreshAdapter;
    let requests = 0;
    admin.adminClient.defaults.adapter = async (config) => {
      if (config.headers.Authorization === 'Bearer access-A') {
        requests++;
        if (requests === 3) {
          lateConfig = config;
          return late.promise;
        }
        throw unauthorized(config);
      }
      expect(config.headers.Authorization).toBe('Bearer refreshed-A');
      return response(config, { user_id: 'A', email: 'A@example.com', is_admin: true });
    };
    const generation = authLifecycle.generation;
    const first = admin.fetchAdminMe();
    const second = admin.fetchAdminMe();
    const third = admin.fetchAdminMe();
    await vi.waitFor(() => expect(refreshAdapter).toHaveBeenCalledTimes(1));
    held.resolve(response(refreshConfig, { access_token: 'refreshed-A', refresh_token: 'rotated-A' }));
    expect((await first).user_id).toBe('A');
    expect((await second).user_id).toBe('A');
    late.reject(unauthorized(lateConfig));
    expect((await third).user_id).toBe('A');
    expect(refreshAdapter).toHaveBeenCalledTimes(1);
    expect(authLifecycle.generation).toBe(generation);
    expect(adminWorkspaceStore.workspaceId).toBe('ws-A');
    expect(adminStore.refreshToken).toBe('rotated-A');
  });

  it('does not refresh a delayed A 401 using B credentials', async () => {
    const held = deferred<AxiosResponse>();
    let config!: InternalAxiosRequestConfig;
    admin.adminClient.defaults.adapter = (request) => {
      config = request;
      return held.promise;
    };
    const refreshSpy = vi.spyOn(axios, 'post');
    const pending = admin.fetchAdminMe();
    const rejected = expect(pending).rejects.toSatisfy(axios.isCancel);
    setAdminSession('B', 'refresh-B');
    held.reject(unauthorized(config));
    await rejected;
    expect(refreshSpy).not.toHaveBeenCalled();
  });

  it.each(['logout', 'replacement'])('discards delayed login after %s', async (transition) => {
    const held = deferred<AxiosResponse>();
    let config!: InternalAxiosRequestConfig;
    axios.defaults.adapter = (request) => {
      config = request;
      return held.promise;
    };
    const pending = admin.adminLogin('A@example.com', 'password');
    const rejected = expect(pending).rejects.toSatisfy(axios.isCancel);
    if (transition === 'logout') clearAdminSession();
    else setAdminSession('B', 'refresh-B');
    held.resolve(response(config, { access_token: 'late-A', refresh_token: 'late-refresh-A' }));
    await rejected;
    expect(adminStore.accessToken).toBe(transition === 'logout' ? '' : 'B');
  });

  it('login replaces the account through the shared boundary and resets selection', async () => {
    axios.defaults.adapter = async (config) => response(config, { access_token: 'B', refresh_token: 'refresh-B' });
    const generation = authLifecycle.generation;
    await admin.adminLogin('B@example.com', 'password');
    expect(authLifecycle.generation).toBeGreaterThan(generation);
    expect(adminStore.accessToken).toBe('B');
    expect(adminWorkspaceStore.workspaceId).toBe('');
  });

  it.each(['success', 'failure'])('logout clears immediately and its delayed %s cannot clear B', async (outcome) => {
    setDashboardToken('operator-A');
    const held = deferred<AxiosResponse>();
    let config!: InternalAxiosRequestConfig;
    axios.defaults.adapter = (request) => {
      config = request;
      return held.promise;
    };
    const pending = admin.adminLogout().catch(() => undefined);
    expect(adminStore.accessToken).toBe('');
    expect(adminWorkspaceStore.workspaceId).toBe('');
    expect(dashboardStore.token).toBe('');
    expect(config.headers.Authorization).toBe('Bearer access-A');
    expect(JSON.parse(config.data).refresh_token).toBe('refresh-A');
    setAdminSession('B', 'refresh-B');
    if (outcome === 'success') held.resolve(response(config, {}));
    else held.reject(new Error('network down'));
    await pending;
    expect(adminStore.accessToken).toBe('B');
  });

  it('a delayed refresh cannot resurrect a logged-out session', async () => {
    const run = beginRefresh();
    await vi.waitFor(() => expect(run.config()).toBeDefined());
    clearAdminSession();
    run.held.resolve(response(run.config(), { access_token: 'late-A', refresh_token: 'late-refresh-A' }));
    await run.rejected;
    expect(adminStore.accessToken).toBe('');
  });

  const snapshot = { version: 'test', address: ':8080', auth_mode: 'none', start_time: '', now: '' };
  it.each(['replace', 'signout-empty', 'account'])(
    'token validation cannot overwrite a later %s transition',
    async (transition) => {
      const held = deferred<AxiosResponse>();
      let config!: InternalAxiosRequestConfig;
      axios.defaults.adapter = (request) => {
        config = request;
        return held.promise;
      };
      const pending = dashboard.connectDashboardToken('candidate-A');
      const rejected = expect(pending).rejects.toSatisfy(axios.isCancel);
      expect(dashboardStore.token).toBe('');
      if (transition === 'replace') setDashboardToken('token-B');
      else if (transition === 'account') setAdminSession('B', 'refresh-B');
      else setDashboardToken('');
      held.resolve(response(config, snapshot));
      await rejected;
      expect(dashboardStore.token).toBe(transition === 'replace' ? 'token-B' : '');
    },
  );

  it('token verification commits only valid results and failed verification never restores an old token', async () => {
    setDashboardToken('original');
    axios.defaults.adapter = async (config) => response(config, snapshot);
    await dashboard.connectDashboardToken('valid');
    expect(dashboardStore.token).toBe('valid');
    axios.defaults.adapter = async (config) => {
      throw unauthorized(config);
    };
    await expect(dashboard.connectDashboardToken('invalid')).rejects.toBeDefined();
    expect(dashboardStore.token).toBe('valid');
  });
});
