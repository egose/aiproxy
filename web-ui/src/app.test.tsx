import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import axios, { AxiosError, type AxiosAdapter, type InternalAxiosRequestConfig } from 'axios';
import { MemoryRouter } from 'react-router';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { App } from './app';
import { adminClient } from './services/admin';
import { dashboardClient } from './services/client';
import { clearAdminSession, setDashboardToken } from './store';

const originalAdapters = [axios.defaults.adapter, adminClient.defaults.adapter, dashboardClient.defaults.adapter];
let queryClient: QueryClient;

function response(config: InternalAxiosRequestConfig, data: unknown, status = 200) {
  return { config, data, status, statusText: String(status), headers: {} };
}

function unauthorized(config: InternalAxiosRequestConfig) {
  return new AxiosError('unauthorized', 'ERR_BAD_REQUEST', config, undefined, response(config, 'unauthorized', 401));
}

function serverStatus(config: InternalAxiosRequestConfig, multi: boolean) {
  return response(config, { multi_tenancy_enabled: multi, web_ui_enabled: true, db_ok: multi });
}

beforeEach(() => {
  clearAdminSession();
  setDashboardToken('');
  axios.defaults.adapter = async (config) => serverStatus(config, false);
  queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
});

afterEach(() => {
  cleanup();
  queryClient.clear();
  [axios.defaults.adapter, adminClient.defaults.adapter, dashboardClient.defaults.adapter] = originalAdapters;
  clearAdminSession();
  setDashboardToken('');
});

function renderApp(path = '/') {
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[path]}>
        <App />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe('dashboard shell', () => {
  it('renders navigation and overview', async () => {
    renderApp('/');
    expect(await screen.findByText('Providers')).toBeInTheDocument();
  });

  it('renders the token page', async () => {
    renderApp('/token');
    expect(await screen.findByText('Dashboard token')).toBeInTheDocument();
  });

  it('loads Overview with the admin session after sign-in even when the browser has a stale dashboard token', async () => {
    setDashboardToken('stale-dashboard');
    axios.defaults.adapter = async (config) => {
      if (config.url === '/_internal/admin/status') return serverStatus(config, true);
      expect(config.url).toBe('/_internal/admin/login');
      return response(config, { access_token: 'account-session', refresh_token: 'account-refresh' });
    };
    const account = vi.fn<AxiosAdapter>(async (config) => {
      expect(config.headers.Authorization).toBe('Bearer account-session');
      switch (config.url) {
        case '/_internal/admin/me':
          return response(config, { user_id: 'operator', email: 'operator@example.com', is_admin: true });
        case '/_internal/admin/workspaces':
          return response(config, { workspaces: [] });
        case '/_internal/dashboard/snapshot':
          return response(config, { version: 'test', address: ':8080', auth_mode: 'none', start_time: '', now: '' });
        default:
          throw new Error(`Unexpected account request: ${config.url}`);
      }
    });
    const token = vi.fn<AxiosAdapter>(async (config) => {
      throw unauthorized(config);
    });
    adminClient.defaults.adapter = account;
    dashboardClient.defaults.adapter = token;

    renderApp();
    fireEvent.change(await screen.findByLabelText('Email'), { target: { value: 'operator@example.com' } });
    fireEvent.change(screen.getByLabelText('Password'), { target: { value: 'password' } });
    fireEvent.click(screen.getByRole('button', { name: 'Sign in' }));

    expect(await screen.findByText('No usage recorded yet.')).toBeInTheDocument();
    expect(account.mock.calls.some(([config]) => config.url === '/_internal/dashboard/snapshot')).toBe(true);
    expect(token).not.toHaveBeenCalled();
    expect(screen.queryByText('Fix token')).not.toBeInTheDocument();
  });
});
