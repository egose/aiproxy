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
import type { WorkspaceContext } from '../workspace-selection';
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
function context(): WorkspaceContext {
  return {
    workspaceId: adminWorkspaceStore.workspaceId,
    generation: authLifecycle.generation,
    session: authLifecycle.session,
  };
}

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
  ['fetchAdminProviders', () => admin.fetchAdminProviders(context())],
  ['fetchProviderTypes', () => admin.fetchProviderTypes()],
  ['createAdminProvider', () => admin.createAdminProvider({}, context())],
  ['updateAdminProvider', () => admin.updateAdminProvider('id', {}, context())],
  ['deleteAdminProvider', () => admin.deleteAdminProvider('id', context())],
  ['setProviderCredential', () => admin.setProviderCredential('id', {}, context())],
  ['fetchAdminAliases', () => admin.fetchAdminAliases(context())],
  ['createAdminAlias', () => admin.createAdminAlias({}, context())],
  ['updateAdminAlias', () => admin.updateAdminAlias('id', {}, context())],
  ['deleteAdminAlias', () => admin.deleteAdminAlias('id', context())],
  ['fetchAdminKeys', () => admin.fetchAdminKeys(context())],
  ['createAdminKey', () => admin.createAdminKey({}, context())],
  ['rotateAdminKey', () => admin.rotateAdminKey('id', context())],
  ['revokeAdminKey', () => admin.revokeAdminKey('id', context())],
  ['deleteAdminKey', () => admin.deleteAdminKey('id', context())],
  ['fetchAdminKey', () => admin.fetchAdminKey('id', context())],
  ['updateAdminKey', () => admin.updateAdminKey('id', {}, context())],
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
  ['startCopilotDeviceFlow', () => admin.startCopilotDeviceFlow({ client_id: 'test' }, context())],
  ['fetchCopilotDeviceFlow', () => admin.fetchCopilotDeviceFlow('id', context())],
  ['pollCopilotDeviceFlow', () => admin.pollCopilotDeviceFlow('id', context())],
  ['cancelCopilotDeviceFlow', () => admin.cancelCopilotDeviceFlow('id', context())],
] as const;

const dashboardCalls = [
  ['fetchSnapshot', dashboard.fetchSnapshot],
  ['fetchPayloads', dashboard.fetchPayloads],
  ['fetchPayload', (credential) => dashboard.fetchPayload(credential, 'id')],
  ['fetchBlocks', dashboard.fetchBlocks],
  ['fetchBlock', (credential) => dashboard.fetchBlock(credential, 'id')],
  ['decideBlock', (credential) => dashboard.decideBlock(credential, 'id', 'deny', [])],
] satisfies [string, (credential: dashboard.DashboardCredential) => Promise<unknown>][];

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
        'acceptAdminInvite',
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
    const scoped = /Provider$|Alias$|Key$|Credential$|Providers$|Aliases$|Keys$|DeviceFlow$/.test(_name);
    expect(config.headers[admin.adminWorkspaceHeader]).toBe(scoped ? 'ws-A' : undefined);
    setAdminSession('access-B', 'refresh-B');
    expect(config.signal?.aborted).toBe(true);
    held.resolve(response(config, { secret: 'A', token: 'one-time-A' }));
    await rejected;
    expect(adminStore.accessToken).toBe('access-B');
  });

  for (const credential of ['session', 'token'] as const) {
    it.each(dashboardCalls)(`%s rejects delayed dashboard ${credential} data and decisions`, async (_name, call) => {
      setDashboardToken('token-A');
      const held = deferred<AxiosResponse>();
      let config!: InternalAxiosRequestConfig;
      const client = credential === 'token' ? dashboardClient : admin.adminClient;
      client.defaults.adapter = (request) => {
        config = request;
        return held.promise;
      };
      const pending = call(credential);
      const rejected = expect(pending).rejects.toSatisfy(axios.isCancel);
      expect(config.headers.Authorization).toBe(`Bearer ${credential === 'token' ? 'token-A' : 'access-A'}`);
      expect(config.headers[admin.adminWorkspaceHeader]).toBeUndefined();
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
    const pending = admin.fetchAdminKeys(context());
    const rejected = expect(pending).rejects.toSatisfy(axios.isCancel);
    setAdminWorkspaceId('ws-B');
    held.resolve(response(config, { keys: [] }));
    await rejected;
    expect(config.signal?.aborted).toBe(true);
  });
});

const workspaceCalls: Array<[string, (context: WorkspaceContext) => Promise<unknown>]> = [
  ['provider list', (context) => admin.fetchAdminProviders(context)],
  ['provider create', (context) => admin.createAdminProvider({}, context)],
  ['provider update', (context) => admin.updateAdminProvider('a-provider', {}, context)],
  ['provider delete', (context) => admin.deleteAdminProvider('a-provider', context)],
  [
    'provider credential',
    (context) => admin.setProviderCredential('a-provider', { api_key: 'never-cross-context' }, context), // pragma: allowlist secret
  ],
  ['alias list', (context) => admin.fetchAdminAliases(context)],
  ['alias create', (context) => admin.createAdminAlias({}, context)],
  ['alias update', (context) => admin.updateAdminAlias('a-alias', {}, context)],
  ['alias delete', (context) => admin.deleteAdminAlias('a-alias', context)],
  ['key list', (context) => admin.fetchAdminKeys(context)],
  ['key detail', (context) => admin.fetchAdminKey('a-key', context)],
  ['key create', (context) => admin.createAdminKey({}, context)],
  ['key update', (context) => admin.updateAdminKey('a-key', {}, context)],
  ['key rotate', (context) => admin.rotateAdminKey('a-key', context)],
  ['key revoke', (context) => admin.revokeAdminKey('a-key', context)],
  ['key delete', (context) => admin.deleteAdminKey('a-key', context)],
  ['device start', (context) => admin.startCopilotDeviceFlow({ client_id: 'fixture' }, context)],
  ['device status', (context) => admin.fetchCopilotDeviceFlow('a-flow', context)],
  ['device poll', (context) => admin.pollCopilotDeviceFlow('a-flow', context)],
  ['device cancel', (context) => admin.cancelCopilotDeviceFlow('a-flow', context)],
];

describe('workspace-pinned services', () => {
  it.each(workspaceCalls)(
    '%s rejects stale or unresolved selection before sending credentials or a mutation',
    async (_name, call) => {
      const saved = context();
      const adapter = vi.fn<AxiosAdapter>();
      admin.adminClient.defaults.adapter = adapter;
      setAdminWorkspaceId('ws-B');
      await expect(call(saved)).rejects.toSatisfy(axios.isCancel);
      setAdminWorkspaceId('ws-A');
      await expect(call(saved)).rejects.toSatisfy(axios.isCancel);
      await expect(call(undefined as never)).rejects.toSatisfy(axios.isCancel);
      expect(adapter).not.toHaveBeenCalled();
    },
  );

  it.each([admin.createAdminProvider, admin.createAdminAlias, admin.createAdminKey])(
    'does not let an explicit destination replace the authorized selected workspace',
    async (create) => {
      const adapter = vi.fn<AxiosAdapter>();
      admin.adminClient.defaults.adapter = adapter;
      await expect(create({ workspace_id: 'ws-B' }, context())).rejects.toThrow('selected workspace');
      expect(adapter).not.toHaveBeenCalled();
    },
  );
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

  it('refreshes a dashboard session on 401 without falling back to a stored dashboard token', async () => {
    setDashboardToken('token-A');
    const refresh = vi.fn<AxiosAdapter>(async (config) => {
      expect(config.url).toBe('/_internal/admin/refresh');
      expect(JSON.parse(config.data).refresh_token).toBe('refresh-A');
      return response(config, { access_token: 'refreshed-A', refresh_token: 'rotated-A' });
    });
    const account = vi.fn<AxiosAdapter>(async (config) => {
      if (config.headers.Authorization === 'Bearer access-A') throw unauthorized(config);
      expect(config.headers.Authorization).toBe('Bearer refreshed-A');
      return response(config, { request_id: 'id' });
    });
    const token = vi.fn<AxiosAdapter>();
    axios.defaults.adapter = refresh;
    admin.adminClient.defaults.adapter = account;
    dashboardClient.defaults.adapter = token;

    await expect(dashboard.fetchPayload('session', 'id')).resolves.toEqual({ request_id: 'id' });
    expect(refresh).toHaveBeenCalledTimes(1);
    expect(account).toHaveBeenCalledTimes(2);
    expect(token).not.toHaveBeenCalled();
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
