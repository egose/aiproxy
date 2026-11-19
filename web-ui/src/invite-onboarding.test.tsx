import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import axios, { AxiosError, type AxiosResponse, type InternalAxiosRequestConfig } from 'axios';
import { BrowserRouter, useNavigate } from 'react-router';
import { StrictMode } from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { App } from './app';
import { adminClient, adminWorkspaceHeader, acceptAdminInvite } from './services/admin';
import { dashboardClient } from './services/client';
import {
  adminStore,
  adminWorkspaceStore,
  clearAdminSession,
  setAdminSession,
  setAdminWorkspaceId,
  setDashboardToken,
} from './store';
import { systemWorkspaceId } from './workspace-selection';

const token = 'fixture-private-invitation';
const workspaceId = '11111111-1111-4111-8111-111111111111';
const originals = {
  public: axios.defaults.adapter,
  admin: adminClient.defaults.adapter,
  dashboard: dashboardClient.defaults.adapter,
};
let client: QueryClient;
function response(config: InternalAxiosRequestConfig, data: unknown): AxiosResponse {
  return { config, data, status: 200, statusText: 'OK', headers: {} };
}
beforeEach(() => {
  clearAdminSession();
  localStorage.clear();
  sessionStorage.clear();
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
});
afterEach(() => {
  cleanup();
  client.clear();
  axios.defaults.adapter = originals.public;
  adminClient.defaults.adapter = originals.admin;
  dashboardClient.defaults.adapter = originals.dashboard;
  clearAdminSession();
  setDashboardToken('');
  window.history.replaceState(null, '', '/');
});
function mount(path: string) {
  window.history.replaceState(null, '', path);
  render(
    <StrictMode>
      <QueryClientProvider client={client}>
        <BrowserRouter>
          <App />
          <RouteControl />
        </BrowserRouter>
      </QueryClientProvider>
    </StrictMode>,
  );
}
function RouteControl() {
  const navigate = useNavigate();
  return <button onClick={() => navigate('/providers')}>Open providers route</button>;
}
function fixture(rejected = false) {
  const requests: InternalAxiosRequestConfig[] = [];
  dashboardClient.defaults.adapter = async (config) => {
    requests.push(config);
    throw new Error('unexpected dashboard request ' + config.url);
  };
  axios.defaults.adapter = async (config) => {
    requests.push(config);
    if (config.url === '/_internal/admin/status')
      return response(config, {
        multi_tenancy_enabled: true,
        registration_enabled: false,
        web_ui_enabled: true,
        db_ok: true,
      });
    const body = JSON.parse(config.data);
    if (config.url === '/_internal/admin/invites/accept') {
      expect(config.method).toBe('post');
      expect(config.headers.Authorization).toBeUndefined();
      expect(config.headers[adminWorkspaceHeader]).toBeUndefined();
      expect(body).toEqual({ token, password: 'invited-password' }); // pragma: allowlist secret
      if (rejected)
        throw new AxiosError('private error ' + token, 'ERR_BAD_REQUEST', config, undefined, {
          ...response(config, token),
          status: 400,
        });
      return response(config, { id: 'invited-user', email: 'invited@example.test', role: 'user', source: 'database' });
    }
    if (config.url === '/_internal/admin/login') {
      expect(body).toEqual({ email: 'invited@example.test', password: 'invited-password' }); // pragma: allowlist secret
      return response(config, { access_token: 'invited-session', refresh_token: 'invited-refresh' });
    }
    throw new Error('unexpected public request ' + config.url);
  };
  adminClient.defaults.adapter = async (config) => {
    requests.push(config);
    expect(config.headers.Authorization).toBe('Bearer invited-session');
    if (config.url === '/_internal/admin/me')
      return response(config, { user_id: 'invited-user', email: 'invited@example.test', is_admin: false });
    if (config.url === '/_internal/admin/workspaces')
      return response(config, {
        workspaces: [
          { id: workspaceId, name: 'invited-tenant', display_name: 'Invited Tenant', role: 'member', is_system: false },
        ],
      });
    if (config.url === '/_internal/admin/providers') {
      expect(config.headers[adminWorkspaceHeader]).toBe(workspaceId);
      return response(config, { providers: [] });
    }
    throw new Error('unexpected private request ' + config.url);
  };
  return requests;
}

describe('signed-out invitation onboarding', () => {
  it('rejects a delayed redemption result after account replacement without retaining the request error', async () => {
    let resolve!: (response: AxiosResponse) => void;
    let request!: InternalAxiosRequestConfig;
    axios.defaults.adapter = (config) => {
      request = config;
      return new Promise((done) => {
        resolve = done;
      });
    };
    const pending = acceptAdminInvite(token, 'invited-password');
    await waitFor(() => expect(resolve).toBeDefined());
    setAdminSession('replacement', 'replacement-refresh');
    resolve(response(request, { id: 'invited-user', email: 'invited@example.test', role: 'user', source: 'database' }));
    await expect(pending).rejects.toThrow('Unable to accept invitation');
    expect(adminStore.accessToken).toBe('replacement');
  });
  it('redeems a manually entered code with registration closed, then signs in to the invited workspace', async () => {
    const requests = fixture();
    setAdminWorkspaceId('foreign-saved-workspace');
    setDashboardToken('previous-dashboard-token');
    mount('/invite');
    const input = await screen.findByLabelText('Invitation code');
    expect(input).toHaveValue('');
    fireEvent.change(input, { target: { value: ` ${token} ` } });
    fireEvent.change(screen.getByLabelText('Password (min 8 characters)'), { target: { value: 'invited-password' } });
    expect(window.location.href).toBe(`${window.location.origin}/invite`);
    expect(requests.some((request) => request.url === '/_internal/admin/invites/accept')).toBe(false);
    fireEvent.click(screen.getByRole('button', { name: 'Accept invitation' }));
    await screen.findByText(/Your account is ready/);
    expect(adminStore.accessToken).toBe('');
    expect(requests.some((request) => request.url === '/_internal/admin/login')).toBe(false);
    expect(screen.queryByLabelText('Invitation code')).toBeNull();
    expect(JSON.stringify(window.history.state)).not.toContain(token);
    fireEvent.click(screen.getByRole('link', { name: 'Sign in' }));
    fireEvent.change(await screen.findByLabelText('Email'), { target: { value: 'invited@example.test' } });
    fireEvent.change(screen.getByLabelText('Password'), { target: { value: 'invited-password' } });
    fireEvent.click(screen.getByRole('button', { name: 'Sign in' }));
    await waitFor(() => expect(adminWorkspaceStore.workspaceId).toBe(workspaceId));
    fireEvent.click(screen.getByRole('button', { name: 'Open providers route' }));
    await waitFor(() => expect(requests.some((request) => request.url === '/_internal/admin/providers')).toBe(true));
    expect(requests.some((request) => request.url?.startsWith('/_internal/dashboard/'))).toBe(false);
    expect(requests.every((request) => !request.url?.includes(token))).toBe(true);
    const carryingCode = requests.filter((request) => typeof request.data === 'string' && request.data.includes(token));
    expect(carryingCode.map((request) => request.url)).toEqual(['/_internal/admin/invites/accept']);
    expect(
      JSON.stringify(
        client
          .getQueryCache()
          .getAll()
          .map((query) => query.state.data),
      ),
    ).not.toContain(token);
    expect(client.getMutationCache().getAll()).toHaveLength(0);
    expect(JSON.stringify(localStorage)).not.toContain(token);
    expect(JSON.stringify(sessionStorage)).not.toContain(token);
    window.history.back();
    await waitFor(() => expect(window.location.pathname).not.toBe('/providers'));
    expect(window.location.href).not.toContain(token);
  });

  it('uses a manually entered code and exposes only a sanitized failure, with no login or cache retention', async () => {
    const requests = fixture(true);
    mount('/invite');
    fireEvent.change(await screen.findByLabelText('Invitation code'), { target: { value: token } });
    fireEvent.change(screen.getByLabelText('Password (min 8 characters)'), { target: { value: 'invited-password' } });
    fireEvent.click(screen.getByRole('button', { name: 'Accept invitation' }));
    expect(await screen.findByRole('alert')).not.toHaveTextContent(token);
    expect(requests.some((request) => request.url === '/_internal/admin/login')).toBe(false);
    expect(client.getMutationCache().getAll()).toHaveLength(0);
    expect(JSON.stringify(localStorage)).not.toContain(token);
    expect(JSON.stringify(sessionStorage)).not.toContain(token);
    await expect(acceptAdminInvite(token, 'invited-password')).rejects.toThrow('Unable to accept invitation');
    try {
      await acceptAdminInvite(token, 'invited-password');
    } catch (error) {
      expect(error).not.toHaveProperty('config');
      expect(String(error)).not.toContain(token);
    }
  });

  it('offers a clean invitation route when public registration is closed', async () => {
    fixture();
    mount('/register');
    await screen.findByText('Public registration is closed');
    const link = screen.getByRole('link', { name: 'Accept invitation' });
    expect(link).toHaveAttribute('href', '/invite');
    fireEvent.click(link);
    expect(await screen.findByLabelText('Invitation code')).toHaveValue('');
  });

  it.each([
    ['query', '?'],
    ['fragment', '#'],
  ])('does not prefill or redeem a legacy %s URL', async (_kind, delimiter) => {
    const requests = fixture();
    mount(`/invite${delimiter}token=${token}`);
    const input = await screen.findByLabelText('Invitation code');
    expect(input).toHaveValue('');
    await waitFor(() => expect(window.location.href).toBe(`${window.location.origin}/invite`));
    expect(input).toHaveValue('');
    fireEvent.change(screen.getByLabelText('Password (min 8 characters)'), { target: { value: 'invited-password' } });
    fireEvent.submit(input.closest('form')!);
    expect(requests.some((request) => request.url === '/_internal/admin/invites/accept')).toBe(false);
    expect(screen.queryByText(/Your account is ready/)).not.toBeInTheDocument();
  });
});

describe('administrator invitation delivery', () => {
  const writeText = vi.fn<(value: string) => Promise<void>>();
  const originalClipboard = Object.getOwnPropertyDescriptor(navigator, 'clipboard');

  beforeEach(() => {
    writeText.mockReset().mockResolvedValue(undefined);
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } });
  });

  afterEach(() => {
    if (originalClipboard) Object.defineProperty(navigator, 'clipboard', originalClipboard);
    else Reflect.deleteProperty(navigator, 'clipboard');
  });

  it.each([
    { path: '/admin/users', system: true, title: 'Invitation code' },
    { path: '/manage/members', system: false, title: 'Invitation code for invited-tenant' },
  ])('shows instructions and copies only the code from $path', async ({ path, system, title }) => {
    const requests = fixture();
    setAdminSession('admin-session', 'admin-refresh');
    const email = 'invitee@example.test';
    const workspace = system
      ? { id: systemWorkspaceId, name: 'system', role: 'admin', is_system: true }
      : { id: workspaceId, name: 'invited-tenant', role: 'admin', is_system: false };
    setAdminWorkspaceId(workspace.id);
    const invite = { id: 'pending-invite', email, expires_at: '2030-01-01T00:00:00Z' };
    const entries: (typeof invite)[] = [];
    adminClient.defaults.adapter = async (config) => {
      requests.push(config);
      expect(config.headers.Authorization).toBe('Bearer admin-session');
      switch (config.url) {
        case '/_internal/admin/me':
          return response(config, { user_id: 'inviter', email: 'inviter@example.test', is_admin: system });
        case '/_internal/admin/workspaces':
          return response(config, { workspaces: [workspace] });
        case '/_internal/admin/users':
          return response(config, { users: [] });
        case `/_internal/admin/workspaces/${workspaceId}/members`:
          return response(config, { members: [] });
        case '/_internal/dashboard/snapshot':
          return response(config, { version: 'test', address: ':8080', auth_mode: 'none', start_time: '', now: '' });
        case '/_internal/admin/invites':
          if (config.method === 'get') return response(config, { invites: entries.slice() });
          expect(config.method).toBe('post');
          expect(JSON.parse(config.data)).toEqual(
            system ? { email, role: 'user' } : { email, workspace_id: workspaceId, workspace_role: 'member' },
          );
          entries.push(invite);
          return response(config, { invite, token });
        default:
          throw new Error('unexpected private request ' + config.url);
      }
    };
    mount(path);
    if (!system) {
      fireEvent.click(await screen.findByRole('button', { name: 'Invite' }));
    }
    const form = system ? screen : within(await screen.findByRole('dialog', { name: 'Invite to invited-tenant' }));
    fireEvent.change(await form.findByLabelText(system ? 'Email' : 'Email (new users set their own password)'), {
      target: { value: email },
    });
    fireEvent.click(form.getByRole('button', { name: system ? 'Create invite' : 'Invite' }));
    const dialog = await screen.findByRole('dialog', { name: title });
    expect(dialog).toHaveTextContent(/\/invite.*send the code separately.*never in a URL/i);
    expect(within(dialog).getByText(token)).toBeInTheDocument();
    expect(Array.from(document.links, (link) => link.href).every((href) => !href.includes(token))).toBe(true);
    fireEvent.click(within(dialog).getByRole('button', { name: 'Copy' }));
    await waitFor(() => expect(writeText.mock.calls).toEqual([[token]]));
    fireEvent.click(within(dialog).getByRole('button', { name: 'Done' }));
    await waitFor(() => expect(dialog).not.toBeInTheDocument());
    await waitFor(() => expect(client.isMutating()).toBe(0));
    expect(window.location.href).toBe(`${window.location.origin}${path}`);
    expect(JSON.stringify(window.history.state)).not.toContain(token);
    expect(JSON.stringify(localStorage)).not.toContain(token);
    expect(JSON.stringify(sessionStorage)).not.toContain(token);
    expect(
      JSON.stringify(
        client
          .getQueryCache()
          .getAll()
          .map((query) => query.state.data),
      ),
    ).not.toContain(token);
    expect(
      JSON.stringify(
        client
          .getMutationCache()
          .getAll()
          .map((mutation) => mutation.state.data),
      ),
    ).not.toContain(token);
    expect(
      requests.filter((request) => request.url === '/_internal/admin/invites' && request.method === 'post'),
    ).toHaveLength(1);
    expect(requests.some((request) => request.url === '/_internal/admin/invites/accept')).toBe(false);
  });
});
