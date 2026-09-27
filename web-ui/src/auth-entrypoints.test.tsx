import { StrictMode } from 'react';
import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter, Route, Routes } from 'react-router';
import axios, { type AxiosResponse, type InternalAxiosRequestConfig } from 'axios';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { LoginPage } from './pages/login-page';
import { LogoutPage } from './pages/logout-page';
import { OidcCallbackPage } from './pages/oidc-callback-page';
import { TokenPage } from './pages/token-page';
import { authLifecycle } from './auth-lifecycle';
import {
  adminWorkspaceStore,
  adminStore,
  clearAdminSession,
  dashboardStore,
  setAdminWorkspaceId,
  setAdminSession,
  setDashboardToken,
} from './store';

const original = axios.defaults.adapter;
let client: QueryClient;
function response(config: InternalAxiosRequestConfig, data: unknown): AxiosResponse {
  return { config, data, status: 200, statusText: 'OK', headers: {} };
}
beforeEach(() => {
  setAdminSession('A', 'refresh-A');
  setAdminWorkspaceId('ws-A');
  setDashboardToken('dashboard-A');
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
});
afterEach(() => {
  cleanup();
  client.clear();
  axios.defaults.adapter = original;
  window.location.hash = '';
  clearAdminSession();
  setDashboardToken('');
});
function page(element: React.JSX.Element) {
  render(
    <StrictMode>
      <QueryClientProvider client={client}>
        <MemoryRouter initialEntries={['/entry']}>
          <Routes>
            <Route path="/entry" element={element} />
            <Route path="/" element={<p>Signed-in destination</p>} />
          </Routes>
        </MemoryRouter>
      </QueryClientProvider>
    </StrictMode>,
  );
}

describe('browser authentication entry points', () => {
  it('password login commits once through the shared session boundary', async () => {
    axios.defaults.adapter = async (config) =>
      response(
        config,
        config.url?.endsWith('/status')
          ? { multi_tenancy_enabled: true, web_ui_enabled: true, db_ok: true }
          : { access_token: 'B', refresh_token: 'refresh-B' },
      );
    const session = authLifecycle.session;
    page(<LoginPage />);
    fireEvent.change(screen.getByLabelText('Email'), { target: { value: 'B@example.com' } });
    fireEvent.change(screen.getByLabelText('Password'), { target: { value: 'password' } });
    fireEvent.click(screen.getByRole('button', { name: 'Sign in' }));
    expect(await screen.findByText('Signed-in destination')).toBeInTheDocument();
    expect(adminStore.accessToken).toBe('B');
    expect(adminWorkspaceStore.workspaceId).toBe('');
    expect(authLifecycle.session).toBe(session + 1);
  });

  it('OIDC callback replaces A exactly once and consumes the URL credentials', async () => {
    window.location.hash = '#access_token=oidc-B&refresh_token=oidc-refresh-B';
    const session = authLifecycle.session;
    page(<OidcCallbackPage />);
    expect(await screen.findByText('Signed-in destination')).toBeInTheDocument();
    expect(authLifecycle.session).toBe(session + 1);
    expect(adminStore.accessToken).toBe('oidc-B');
    expect(adminStore.refreshToken).toBe('oidc-refresh-B');
    expect(adminWorkspaceStore.workspaceId).toBe('');
    expect(window.location.hash).toBe('');
  });

  it('logout navigates and clears credentials before server revocation finishes', async () => {
    let resolve!: (response: AxiosResponse) => void;
    let config!: InternalAxiosRequestConfig;
    axios.defaults.adapter = (request) => {
      config = request;
      return new Promise((done) => {
        resolve = done;
      });
    };
    page(<LogoutPage />);
    expect(await screen.findByText('Signed-in destination')).toBeInTheDocument();
    expect(adminStore.accessToken).toBe('');
    expect(dashboardStore.token).toBe('');
    act(() => {
      setAdminSession('B', 'refresh-B');
    });
    await act(async () => {
      resolve(response(config, {}));
    });
    expect(adminStore.accessToken).toBe('B');
  });

  it('token sign-out invalidates pending validation without restoring the old credential', async () => {
    let resolve!: (response: AxiosResponse) => void;
    let config!: InternalAxiosRequestConfig;
    axios.defaults.adapter = (request) => {
      config = request;
      return new Promise((done) => {
        resolve = done;
      });
    };
    page(<TokenPage />);
    fireEvent.change(screen.getByLabelText('Bearer token'), { target: { value: 'candidate' } });
    fireEvent.click(screen.getByRole('button', { name: 'Save and connect' }));
    await waitFor(() => expect(config).toBeDefined());
    fireEvent.click(screen.getByRole('button', { name: 'Sign out' }));
    await act(async () => {
      resolve(response(config, { version: '', address: '', auth_mode: 'none', start_time: '', now: '' }));
    });
    expect(dashboardStore.token).toBe('');
    expect(screen.queryByText('Signed-in destination')).not.toBeInTheDocument();
  });
});
