import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { App } from './app';
import { fetchAdminMe, fetchAdminWorkspaces, fetchAdminStatus } from './services/admin';
import { fetchBlock, fetchBlocks, fetchPayload, fetchPayloads, fetchSnapshot } from './services/dashboard';
import { adminWorkspaceStore, clearAdminSession, setAdminSession, setDashboardToken } from './store';
import { adminWorkspaceSchema, adminStatusSchema, snapshotSchema } from './types';

vi.mock('./services/admin', async (importOriginal) => ({
  ...(await importOriginal<typeof import('./services/admin')>()),
  fetchAdminStatus: vi.fn(),
  fetchAdminMe: vi.fn(),
  fetchAdminWorkspaces: vi.fn(),
}));

vi.mock('./services/dashboard', async (importOriginal) => ({
  ...(await importOriginal<typeof import('./services/dashboard')>()),
  fetchSnapshot: vi.fn(),
  fetchPayloads: vi.fn(),
  fetchPayload: vi.fn(),
  fetchBlocks: vi.fn(),
  fetchBlock: vi.fn(),
}));

let queryClient: QueryClient;

beforeEach(() => {
  vi.resetAllMocks();
  clearAdminSession();
  setDashboardToken('');
  queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  vi.mocked(fetchAdminStatus).mockResolvedValue(
    adminStatusSchema.parse({ multi_tenancy_enabled: true, web_ui_enabled: true, db_ok: true }),
  );
  vi.mocked(fetchAdminMe).mockResolvedValue({ user_id: 'member', email: 'member@example.com', is_admin: false });
  vi.mocked(fetchAdminWorkspaces).mockResolvedValue([
    adminWorkspaceSchema.parse({ id: 'ws-a', name: 'ws-a', kind: 'organization', is_system: false, role: 'member' }),
  ]);
  vi.mocked(fetchSnapshot).mockResolvedValue(
    snapshotSchema.parse({
      version: 'test',
      address: ':8080',
      auth_mode: 'none',
      start_time: '',
      now: '',
      last_seq: 0,
    }),
  );
  vi.mocked(fetchPayloads).mockResolvedValue({ enabled: true, payloads: [] });
  vi.mocked(fetchBlocks).mockResolvedValue({ enabled: true, blocks: [] });
});

afterEach(() => {
  cleanup();
  queryClient.clear();
  clearAdminSession();
  setDashboardToken('');
});

function renderPage(path: string) {
  render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[path]}>
        <App />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

function expectNoDashboardReads() {
  for (const fetch of [fetchSnapshot, fetchPayloads, fetchPayload, fetchBlocks, fetchBlock]) {
    expect(fetch).not.toHaveBeenCalled();
  }
}

async function findDetailButton(name: RegExp) {
  await waitForWorkspace();
  await screen.findByText(name);
  return screen.getByRole('button', { name });
}

async function waitForWorkspace() {
  await waitFor(() => expect(adminWorkspaceStore.workspaceId).toBe('ws-a'));
}

describe('operator-only dashboard', () => {
  for (const role of ['member', 'admin'] as const) {
    it.each(['/', '/requests', '/payloads', '/blocks'])(
      `blocks ${role} direct link %s without global reads`,
      async (path) => {
        setAdminSession('ordinary-session', '');
        vi.mocked(fetchAdminWorkspaces).mockResolvedValue([
          adminWorkspaceSchema.parse({ id: 'ws-a', name: 'ws-a', kind: 'organization', is_system: false, role }),
        ]);
        renderPage(path);
        await waitFor(() => {
          expect(screen.getByRole('heading', { name: 'Operator access required' })).toBeInTheDocument();
          expect(screen.getByRole('link', { name: 'Manage workspace API keys' })).toHaveAttribute(
            'href',
            '/manage/keys',
          );
          for (const name of ['Overview', 'Recent requests', 'Payloads', 'Guardrail blocks']) {
            expect(screen.queryByRole('link', { name })).not.toBeInTheDocument();
          }
          expect(screen.getByRole('link', { name: 'API keys' })).toBeInTheDocument();
          expectNoDashboardReads();
        });
      },
    );
  }

  for (const credential of ['system-admin', 'dashboard-secret']) {
    it.each(['/', '/requests', '/payloads', '/blocks'])(`permits ${credential} on %s`, async (path) => {
      setAdminSession('session', '');
      if (credential === 'system-admin') {
        vi.mocked(fetchAdminMe).mockResolvedValue({
          user_id: 'operator',
          email: 'operator@example.com',
          is_admin: true,
        });
      } else {
        setDashboardToken('operator-secret');
      }
      renderPage(path);
      await waitFor(() => expect(fetchSnapshot).toHaveBeenCalled());
      await waitFor(() => expect(screen.getByRole('link', { name: 'Recent requests' })).toBeInTheDocument());
      expect(screen.queryByRole('heading', { name: 'Operator access required' })).not.toBeInTheDocument();
      if (path === '/payloads') await waitFor(() => expect(fetchPayloads).toHaveBeenCalled());
      if (path === '/blocks') await waitFor(() => expect(fetchBlocks).toHaveBeenCalled());
    });
  }

  it('waits for stored authority before loading dashboard data', async () => {
    setAdminSession('session', '');
    vi.mocked(fetchAdminMe).mockReturnValue(new Promise(() => {}));
    renderPage('/blocks');
    expect(await screen.findByText('Checking operator access...')).toBeInTheDocument();
    expectNoDashboardReads();
  });

  it('fails closed when the account cannot be verified', async () => {
    setAdminSession('session', '');
    vi.mocked(fetchAdminMe).mockRejectedValue(new Error('account disabled'));
    renderPage('/');
    await waitFor(() =>
      expect(screen.getByText('Unable to verify your account. Sign in again to check access.')).toBeInTheDocument(),
    );
    expectNoDashboardReads();
  });

  it('explains a server-side demotion instead of showing an empty request table or token repair', async () => {
    setAdminSession('session', '');
    vi.mocked(fetchAdminMe).mockResolvedValue({ user_id: 'operator', email: 'operator@example.com', is_admin: true });
    vi.mocked(fetchSnapshot).mockRejectedValue({
      response: { status: 403, data: 'dashboard operator access required\n' },
    });
    renderPage('/requests');
    await waitForWorkspace();
    expect(await screen.findByText(/Operator access required\. Global dashboard data/)).toBeInTheDocument();
    expect(screen.queryByRole('table')).not.toBeInTheDocument();
    expect(screen.queryByRole('link', { name: 'Fix token' })).not.toBeInTheDocument();
  });

  it.each(['/payloads', '/blocks'])('explains detail access denial on %s and hides decision controls', async (path) => {
    setAdminSession('session', '');
    vi.mocked(fetchAdminMe).mockResolvedValue({ user_id: 'operator', email: 'operator@example.com', is_admin: true });
    vi.mocked(fetchPayloads).mockResolvedValue({ enabled: true, payloads: [{ request_id: 'captured-request' }] });
    vi.mocked(fetchBlocks).mockResolvedValue({
      enabled: true,
      blocks: [
        {
          block_id: 'captured-block',
          ts: '',
          rule_ids: [],
          finding_count: 1,
        },
      ],
    });
    const denied = { response: { status: 403, data: 'dashboard operator access required\n' } };
    vi.mocked(fetchPayload).mockRejectedValue(denied);
    vi.mocked(fetchBlock).mockRejectedValue(denied);
    renderPage(path);
    fireEvent.click(await findDetailButton(/captured-/));
    expect(await screen.findByText(/Operator access required\. Global dashboard data/)).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Record decision' })).not.toBeInTheDocument();
  });

  it('removes A payload detail and operator navigation while B authority is pending, then denies B', async () => {
    setAdminSession('operator-A', 'refresh-A');
    vi.mocked(fetchAdminMe).mockResolvedValue({ user_id: 'A', email: 'A@example.com', is_admin: true });
    vi.mocked(fetchPayloads).mockResolvedValue({ enabled: true, payloads: [{ request_id: 'A-request' }] });
    vi.mocked(fetchPayload).mockResolvedValue({ secret: 'private-payload-A' }); // pragma: allowlist secret
    renderPage('/payloads');
    fireEvent.click(await findDetailButton(/A-request/));
    expect(await screen.findByText(/private-payload-A/)).toBeInTheDocument();
    let resolve!: (value: Awaited<ReturnType<typeof fetchAdminMe>>) => void;
    vi.mocked(fetchAdminMe).mockReturnValue(
      new Promise((done) => {
        resolve = done;
      }),
    );
    const calls = vi.mocked(fetchPayloads).mock.calls.length;
    act(() => {
      setAdminSession('ordinary-B', 'refresh-B');
    });
    await waitFor(() => expect(screen.queryByText(/private-payload-A/)).not.toBeInTheDocument());
    expect(screen.queryByRole('link', { name: 'Recent requests' })).not.toBeInTheDocument();
    await act(async () => {
      resolve({ user_id: 'B', email: 'B@example.com', is_admin: false });
    });
    await waitFor(() => expect(screen.getByRole('heading', { name: 'Operator access required' })).toBeInTheDocument());
    expect(fetchPayloads).toHaveBeenCalledTimes(calls);
    expect(screen.queryByText(/private-payload-A/)).not.toBeInTheDocument();
  });
});
