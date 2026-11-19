import { act, cleanup, fireEvent, render, renderHook, screen, waitFor, within } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { DialogManagerProvider } from '@egose/shadcn-theme/components/widgets/dialog-manager';
import axios, { AxiosError, type AxiosResponse, type InternalAxiosRequestConfig } from 'axios';
import type { PropsWithChildren } from 'react';
import { MemoryRouter } from 'react-router';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { App } from './app';
import { authLifecycle } from './auth-lifecycle';
import {
  useAdminAliases,
  useAdminKeys,
  useAdminProviders,
  useCurrentWorkspace,
  useWorkspaceMembers,
  useWorkspaceTeams,
} from './hooks';
import { ProvidersPage } from './pages/providers-page';
import { adminClient, adminWorkspaceHeader } from './services/admin';
import { adminWorkspaceStore, clearAdminSession, setAdminSession, setAdminWorkspaceId } from './store';
import {
  adminAliasSchema,
  adminKeySchema,
  adminProviderSchema,
  adminWorkspaceSchema,
  type AdminWorkspace,
} from './types';
import { systemWorkspaceId } from './workspace-selection';

const aId = '11111111-1111-4111-8111-111111111111';
const bId = '22222222-2222-4222-8222-222222222222';
const system = adminWorkspaceSchema.parse({
  id: systemWorkspaceId,
  name: 'system',
  display_name: 'System',
  is_system: true,
  role: 'admin',
});
const workspaceA = adminWorkspaceSchema.parse({ id: aId, name: 'aaa-tenant', role: 'admin' });
const workspaceB = adminWorkspaceSchema.parse({ id: bId, name: 'bbb-tenant', role: 'admin' });
const originals = { public: axios.defaults.adapter, admin: adminClient.defaults.adapter };
let client: QueryClient;

function response(config: InternalAxiosRequestConfig, data: unknown): AxiosResponse {
  return { config, data, status: 200, statusText: 'OK', headers: {} };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason: unknown) => void;
  const promise = new Promise<T>((done, fail) => {
    resolve = done;
    reject = fail;
  });
  return { promise, resolve, reject };
}

beforeEach(() => {
  setAdminSession('workspace-session', 'workspace-refresh');
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
});

afterEach(() => {
  cleanup();
  client.clear();
  axios.defaults.adapter = originals.public;
  adminClient.defaults.adapter = originals.admin;
  clearAdminSession();
});

function wrapper({ children }: PropsWithChildren) {
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}

function fixture(
  options: {
    global?: boolean;
    workspaces?: AdminWorkspace[];
    me?: Promise<unknown>;
    memberships?: Promise<unknown>;
    empty?: boolean;
    unfiltered?: boolean;
    saveConflict?: boolean;
  } = {},
) {
  const requests: InternalAxiosRequestConfig[] = [];
  const writes: Array<{ url: string; workspace: unknown; body: Record<string, unknown> }> = [];
  const providers = new Map<string, ReturnType<typeof adminProviderSchema.parse>[]>();
  const aliases = new Map<string, ReturnType<typeof adminAliasSchema.parse>[]>();
  const keys = new Map<string, ReturnType<typeof adminKeySchema.parse>[]>();
  for (const [id, label] of [
    [systemWorkspaceId, 'system'],
    [aId, 'a'],
    [bId, 'b'],
  ]) {
    providers.set(
      id!,
      options.empty && id === aId
        ? []
        : [
            adminProviderSchema.parse({
              name: label + '-base',
              source: 'database',
              workspace_id: id,
              type: 'openai',
              enabled: true,
              has_credential: true,
              models: [{ name: 'm/one' }],
            }),
            ...(id === systemWorkspaceId
              ? [
                  adminProviderSchema.parse({
                    name: 'hcl-only',
                    source: 'config',
                    type: 'openai',
                    enabled: true,
                    models: [{ name: 'm' }],
                  }),
                ]
              : []),
          ],
    );
    aliases.set(
      id!,
      options.empty && id === aId
        ? []
        : [
            adminAliasSchema.parse({
              name: label + '-route',
              source: 'database',
              workspace_id: id,
              algorithm: 'round_robin',
              targets: [{ provider: label + '-base', model: 'm/one' }],
            }),
          ],
    );
    keys.set(
      id!,
      options.empty && id === aId
        ? []
        : [
            adminKeySchema.parse({
              id: label + '-key-id',
              name: label + '-key',
              source: 'database',
              workspace_id: id,
              enabled: true,
              can_manage: true,
            }),
          ],
    );
  }
  axios.defaults.adapter = async (config) =>
    response(config, { multi_tenancy_enabled: true, web_ui_enabled: true, db_ok: true });
  adminClient.defaults.adapter = async (config) => {
    requests.push(config);
    const url = config.url ?? '';
    const workspace = config.headers[adminWorkspaceHeader] as string;
    if (url === '/_internal/admin/me') {
      expect(workspace).toBeUndefined();
      return response(
        config,
        options.me
          ? await options.me
          : { user_id: 'user', email: 'workspace@example.test', is_admin: options.global ?? false },
      );
    }
    if (url === '/_internal/admin/workspaces') {
      expect(workspace).toBeUndefined();
      return response(
        config,
        options.memberships
          ? await options.memberships
          : { workspaces: options.workspaces ?? [workspaceA, system, workspaceB] },
      );
    }
    if (url === '/_internal/dashboard/snapshot') {
      expect(workspace).toBeUndefined();
      return response(config, { version: 'fixture', address: ':8080', auth_mode: 'none', start_time: '', now: '' });
    }
    if (url === '/_internal/admin/provider-types') {
      return response(config, {
        provider_types: [{ type: 'openai', credential: 'api_key', supports_healthcheck: true }],
      });
    }
    if (/\/workspaces\/[^/]+\/(members|teams)$/.test(url)) {
      return response(config, url.endsWith('members') ? { members: [] } : { teams: [] });
    }
    const kind = url.split('/')[3]!;
    const inventories = kind === 'providers' ? providers : kind === 'aliases' ? aliases : keys;
    if (config.method === 'get') {
      if (!['providers', 'aliases', 'keys'].includes(kind)) throw new Error(`Unexpected inventory request ${url}`);
      const rows = options.unfiltered ? [...inventories.values()].flat() : inventories.get(workspace);
      expect(rows).toBeDefined();
      return response(config, { [kind]: rows });
    }
    const body = config.data ? (JSON.parse(config.data) as Record<string, unknown>) : {};
    writes.push({ url, workspace, body });
    if (options.saveConflict && config.method === 'put') {
      throw new AxiosError('catalog changed', 'ERR_BAD_REQUEST', config, undefined, {
        ...response(config, 'catalog changed'),
        status: 409,
      });
    }
    const name = (body.name as string) ?? decodeURIComponent(url.split('/')[4] ?? '');
    if (kind === 'providers') {
      const saved = adminProviderSchema.parse({
        ...body,
        name,
        type: body.type ?? 'openai',
        enabled: true,
        source: 'database',
        workspace_id: workspace,
        models: body.models ?? [{ name: 'm/one' }],
      });
      providers.set(workspace, [...(providers.get(workspace) ?? []).filter((p) => p.name !== name), saved]);
      return response(config, saved);
    }
    if (kind === 'aliases') {
      const targets = body.targets ?? (body.providers as string[]).map((provider) => ({ provider, model: body.model }));
      const saved = adminAliasSchema.parse({
        ...body,
        name,
        targets,
        source: 'database',
        workspace_id: workspace,
        algorithm: body.algorithm ?? 'round_robin',
      });
      aliases.set(workspace, [saved]);
      return response(config, saved);
    }
    if (kind === 'keys') {
      const saved = adminKeySchema.parse({
        ...body,
        id: 'created-key',
        name,
        source: 'database',
        workspace_id: workspace,
        can_manage: true,
      });
      keys.set(workspace, [saved]);
      return response(config, { key: saved, token: 'one-time-fixture-token' });
    }
    throw new Error(`Unexpected mutation ${url}`);
  };
  const inventoryReads = () =>
    requests.filter(
      (config) => config.method === 'get' && /\/_internal\/admin\/(providers|aliases|keys)$/.test(config.url ?? ''),
    );
  return { requests, writes, providers, aliases, inventoryReads };
}

function renderPage(path: string) {
  render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[path]}>
        <App />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe('authorized workspace selection with real inventory services', () => {
  it('waits for both authority and memberships, then prefers System over an alphabetically earlier tenant', async () => {
    const me = deferred<unknown>();
    const memberships = deferred<unknown>();
    setAdminWorkspaceId(bId);
    const state = fixture({ me: me.promise, memberships: memberships.promise });
    const { result } = renderHook(
      () => ({
        selection: useCurrentWorkspace(),
        providers: useAdminProviders(),
        aliases: useAdminAliases(),
        keys: useAdminKeys(),
      }),
      { wrapper },
    );
    await waitFor(() => expect(state.requests.some((r) => r.url?.endsWith('/workspaces'))).toBe(true));
    expect(state.inventoryReads()).toEqual([]);
    await act(async () => me.resolve({ user_id: 'operator', email: 'operator@example.test', is_admin: true }));
    expect(state.inventoryReads()).toEqual([]);
    await act(async () => memberships.resolve({ workspaces: [workspaceA, system] }));
    await waitFor(() =>
      expect(
        result.current.keys.isSuccess && result.current.aliases.isSuccess && result.current.providers.isSuccess,
      ).toBe(true),
    );
    expect(result.current.selection.workspace?.id).toBe(systemWorkspaceId);
    expect(adminWorkspaceStore.workspaceId).toBe(systemWorkspaceId);
    expect(state.inventoryReads()).toHaveLength(3);
    expect(state.inventoryReads().every((r) => r.headers[adminWorkspaceHeader] === systemWorkspaceId)).toBe(true);
    expect(
      JSON.stringify(
        client
          .getQueryCache()
          .getAll()
          .map((q) => q.queryKey),
      ),
    ).not.toContain('"all"');
  });

  it.each(['account', 'memberships'])('fails closed when %s resolution fails', async (failed) => {
    const failure = deferred<unknown>();
    const state = fixture({
      me: failed === 'account' ? failure.promise : undefined,
      memberships: failed === 'memberships' ? failure.promise : undefined,
    });
    renderPage('/providers');
    await waitFor(() =>
      expect(
        state.requests.some(
          (request) => request.url === (failed === 'account' ? '/_internal/admin/me' : '/_internal/admin/workspaces'),
        ),
      ).toBe(true),
    );
    await act(async () => failure.reject(new Error('unavailable')));
    expect(await screen.findByText('Unable to verify workspace access. Sign in again or retry.')).toBeInTheDocument();
    expect(state.inventoryReads()).toEqual([]);
    expect(state.writes).toEqual([]);
    expect(screen.queryByRole('button', { name: 'Create provider' })).not.toBeInTheDocument();
  });

  it.each([
    ['global default', true, '', [workspaceA, system, workspaceB], systemWorkspaceId],
    ['global valid tenant choice', true, bId, [workspaceA, system, workspaceB], bId],
    ['tenant skips System', false, '', [system, workspaceA, workspaceB], aId],
    ['tenant valid second choice', false, bId, [system, workspaceA, workspaceB], bId],
    ['tenant invalid saved choice', false, bId, [system, workspaceA], aId],
  ] as const)(
    '%s uses one choice for the shell, hooks, queries and provider mutations',
    async (_name, global, saved, list, expected) => {
      setAdminWorkspaceId(saved);
      const state = fixture({ global, workspaces: [...list] });
      renderPage('/providers');
      await waitFor(() => expect(adminWorkspaceStore.workspaceId).toBe(expected));
      const label = expected === systemWorkspaceId ? 'System' : expected === aId ? workspaceA.name : workspaceB.name;
      expect(await screen.findByText(label)).toBeInTheDocument();
      await waitFor(() => expect(state.inventoryReads().length).toBeGreaterThan(0));
      expect(state.inventoryReads().every((r) => r.headers[adminWorkspaceHeader] === expected)).toBe(true);
      const button = await screen.findByRole('button', { name: 'Create provider' });
      await waitFor(() => expect(button).toBeEnabled());
      fireEvent.click(button);
      const form = within(await screen.findByRole('dialog', { name: 'Create provider' }));
      const base = expected === aId ? 'a-base' : expected === bId ? 'b-base' : 'system-base';
      const options = within(form.getByLabelText(/^Extends/))
        .getAllByRole('option')
        .map((o) => (o as HTMLOptionElement).value);
      expect(options).toContain(base);
      for (const foreign of ['a-base', 'b-base', 'system-base'].filter((name) => name !== base))
        expect(options).not.toContain(foreign);
      fireEvent.change(form.getByLabelText(/^Name \(lowercase/), { target: { value: 'new-child' } });
      fireEvent.change(form.getByLabelText(/^Extends/), { target: { value: base } });
      fireEvent.change(form.getByLabelText('API key (stored encrypted, write-only)'), {
        target: { value: 'fixture-upstream-key' },
      });
      fireEvent.click(form.getByRole('button', { name: 'Create provider' }));
      await waitFor(() => expect(state.writes).toHaveLength(1));
      expect(state.writes[0]).toMatchObject({ workspace: expected, body: { extends: base } });
    },
  );

  it.each(['member', 'admin'] as const)(
    'explicit System %s membership permits reads but never provider or alias writes',
    async (role) => {
      const state = fixture({ workspaces: [{ ...system, role }] });
      renderPage('/providers');
      expect(await screen.findByText('hcl-only')).toBeInTheDocument();
      expect(await screen.findByText('system-base')).toBeInTheDocument();
      expect(screen.getByRole('button', { name: 'Create provider' })).toBeDisabled();
      expect(screen.queryByRole('button', { name: 'Actions for system-base' })).not.toBeInTheDocument();
      expect(screen.getByText(`system · ${role}`)).toBeInTheDocument();
      fireEvent.click(screen.getByRole('link', { name: 'Aliases' }));
      expect(await screen.findByText('alias/system-route')).toBeInTheDocument();
      expect(screen.getByRole('button', { name: 'Create alias' })).toBeDisabled();
      expect(screen.queryByRole('button', { name: 'Actions for system-route' })).not.toBeInTheDocument();
      expect(state.writes).toEqual([]);
      expect(state.inventoryReads().every((r) => r.headers[adminWorkspaceHeader] === systemWorkspaceId)).toBe(true);
    },
  );

  it('has no tenant inventory or write target without memberships, even with a stale saved System choice', async () => {
    setAdminWorkspaceId(systemWorkspaceId);
    const state = fixture({ workspaces: [] });
    renderPage('/aliases');
    expect(await screen.findByText(/No authorized workspace selected/)).toBeInTheDocument();
    await waitFor(() => expect(adminWorkspaceStore.workspaceId).toBe(''));
    expect(state.inventoryReads()).toEqual([]);
    expect(screen.queryByRole('button', { name: 'Create alias' })).not.toBeInTheDocument();
    expect(screen.queryByText('alias/system-route')).not.toBeInTheDocument();
    expect(state.writes).toEqual([]);
  });

  it('selects by workspace identity when a tenant uses the System display name', async () => {
    const state = fixture({ global: true, workspaces: [{ ...workspaceA, display_name: 'System' }, system] });
    renderPage('/providers');
    const selected = await screen.findByText('System (system)');
    fireEvent.keyDown(selected.closest('button')!, { key: 'Enter' });
    fireEvent.click(await screen.findByRole('menuitem', { name: /System \(aaa-tenant\)/ }));
    await waitFor(() => expect(adminWorkspaceStore.workspaceId).toBe(aId));
    expect(await screen.findByText('a-base')).toBeInTheDocument();
    expect(await screen.findByText('System (aaa-tenant)')).toBeInTheDocument();
    expect(screen.queryByText('hcl-only')).not.toBeInTheDocument();
    expect(screen.queryByRole('link', { name: 'Users' })).not.toBeInTheDocument();
    const reads = state.inventoryReads();
    expect(reads[reads.length - 1]?.headers[adminWorkspaceHeader]).toBe(aId);
  });
});

describe('direct management links and fresh tenant creation', () => {
  it.each(['/providers', '/admin/providers', '/aliases', '/admin/aliases', '/manage/keys', '/admin/keys'])(
    'scopes actual inventory queries on direct link %s',
    async (path) => {
      const state = fixture({ workspaces: [workspaceA] });
      renderPage(path);
      const target = path.includes('aliases') ? 'alias/a-route' : path.includes('keys') ? 'a-key' : 'a-base';
      expect(await screen.findByText(target)).toBeInTheDocument();
      expect(state.inventoryReads().length).toBeGreaterThan(0);
      expect(state.inventoryReads().every((r) => r.headers[adminWorkspaceHeader] === aId)).toBe(true);
      for (const foreign of [
        'system-base',
        'b-base',
        'hcl-only',
        'alias/system-route',
        'alias/b-route',
        'system-key',
        'b-key',
      ]) {
        expect(screen.queryByText(foreign)).not.toBeInTheDocument();
      }
    },
  );

  it('creates the first provider, then an own-model alias and key from an empty workspace', async () => {
    const state = fixture({ workspaces: [workspaceA], empty: true });
    renderPage('/aliases');
    expect(await screen.findByText('No aliases in this workspace.')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Create alias' })).toBeDisabled();
    fireEvent.click(await screen.findByRole('link', { name: 'Create a provider' }));
    expect(
      await screen.findByText('No providers. Create a provider in this workspace to add models.'),
    ).toBeInTheDocument();
    const create = screen.getByRole('button', { name: 'Create provider' });
    await waitFor(() => expect(create).toBeEnabled());
    fireEvent.click(create);
    let form = within(await screen.findByRole('dialog', { name: 'Create provider' }));
    expect(within(form.getByLabelText(/^Extends/)).getAllByRole('option')).toHaveLength(1);
    fireEvent.change(form.getByLabelText('Name (lowercase, no spaces)'), { target: { value: 'first-provider' } });
    fireEvent.change(form.getByLabelText('Name', { exact: true }), { target: { value: 'm/one' } });
    fireEvent.change(form.getByLabelText('API key (stored encrypted, write-only)'), {
      target: { value: 'tenant-key' },
    });
    fireEvent.click(form.getByRole('button', { name: 'Create provider' }));
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    expect(await screen.findByText('first-provider')).toBeInTheDocument();
    expect(state.writes[0]).toMatchObject({
      workspace: aId,
      body: { name: 'first-provider', models: [{ name: 'm/one' }] },
    });

    fireEvent.click(screen.getByRole('link', { name: 'Aliases' }));
    const aliasButton = await screen.findByRole('button', { name: 'Create alias' });
    await waitFor(() => expect(aliasButton).toBeEnabled());
    fireEvent.click(aliasButton);
    form = within(await screen.findByRole('dialog', { name: 'Create alias' }));
    const picker = form.getByLabelText('Available targets in this workspace');
    expect(
      within(picker)
        .getAllByRole('option')
        .map((o) => (o as HTMLOptionElement).value),
    ).toEqual(['', 'first-provider/m/one']);
    fireEvent.change(picker, { target: { value: 'first-provider/m/one' } });
    fireEvent.click(form.getByRole('button', { name: 'Add target' }));
    fireEvent.change(form.getByLabelText('Name (lowercase, no spaces)'), { target: { value: 'first-alias' } });
    fireEvent.click(form.getByRole('button', { name: 'Create alias' }));
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    expect(state.writes[1]).toMatchObject({
      workspace: aId,
      body: { name: 'first-alias', targets: [{ provider: 'first-provider', model: 'm/one' }] },
    });

    fireEvent.click(screen.getByRole('link', { name: 'API keys' }));
    fireEvent.click(await screen.findByRole('button', { name: 'Create API key' }));
    form = within(await screen.findByRole('dialog', { name: 'Create API key' }));
    fireEvent.change(form.getByLabelText('Name', { exact: true }), { target: { value: 'first-key' } });
    fireEvent.click(form.getByRole('button', { name: 'Create API key' }));
    expect(await screen.findByRole('dialog', { name: 'New token' })).toBeInTheDocument();
    expect(state.writes[2]).toMatchObject({
      workspace: aId,
      body: { name: 'first-key', workspace_id: aId, owner_type: 'user' },
    });
    expect(state.inventoryReads().every((r) => r.headers[adminWorkspaceHeader] === aId)).toBe(true);
  });

  it.each([false, true])(
    'rejects foreign alias references before service I/O, including shorthand=%s',
    async (shorthand) => {
      const state = fixture({ workspaces: [workspaceA], unfiltered: true });
      renderPage('/aliases');
      const create = await screen.findByRole('button', { name: 'Create alias' });
      await waitFor(() => expect(create).toBeEnabled());
      fireEvent.click(create);
      const form = within(await screen.findByRole('dialog', { name: 'Create alias' }));
      const options = within(form.getByLabelText('Available targets in this workspace')).getAllByRole('option');
      expect(options.map((option) => (option as HTMLOptionElement).value)).toEqual(['', 'a-base/m/one']);
      fireEvent.change(form.getByLabelText('Name (lowercase, no spaces)'), { target: { value: 'foreign' } });
      if (shorthand) {
        fireEvent.change(form.getByLabelText('Providers'), { target: { value: 'a-base, b-base' } });
        fireEvent.change(form.getByLabelText('Model'), { target: { value: 'm/one' } });
      } else {
        fireEvent.change(form.getByLabelText(/^Targets/), { target: { value: 'a-base/m/one\nb-base/m/one' } });
      }
      fireEvent.click(form.getByRole('button', { name: 'Create alias' }));
      expect(await form.findByText('Choose targets from the selected workspace inventory.')).toBeInTheDocument();
      expect(state.writes).toEqual([]);
    },
  );
});

describe('stale selected workspace credential operations', () => {
  function standaloneProviders() {
    render(
      <QueryClientProvider client={client}>
        <MemoryRouter>
          <DialogManagerProvider>
            <ProvidersPage />
          </DialogManagerProvider>
        </MemoryRouter>
      </QueryClientProvider>,
    );
  }

  async function providerAction(action: string) {
    fireEvent.keyDown(await screen.findByRole('button', { name: 'Actions for a-base' }), { key: 'Enter' });
    fireEvent.click(await screen.findByRole('menuitem', { name: action }));
  }

  it('does not rotate an A provider credential after selection changes to B while the dialog is open', async () => {
    setAdminWorkspaceId(aId);
    const state = fixture({ workspaces: [workspaceA, workspaceB] });
    standaloneProviders();
    await providerAction('Rotate credential');
    const form = within(await screen.findByRole('dialog', { name: 'Rotate upstream credential for a-base' }));
    fireEvent.change(form.getByLabelText('New API key'), { target: { value: 'never-cross-context' } });
    act(() => setAdminWorkspaceId(bId));
    await screen.findByText('b-base');
    fireEvent.click(form.getByRole('button', { name: 'Save credential' }));
    await screen.findByText('Authentication context changed');
    expect(state.writes).toEqual([]);
    const reads = state.inventoryReads();
    expect(reads[reads.length - 1]?.headers[adminWorkspaceHeader]).toBe(bId);
  });

  it('blocks old provider-form recovery queries and saves instead of rereading B under A form state', async () => {
    setAdminWorkspaceId(aId);
    const state = fixture({ workspaces: [workspaceA, workspaceB], saveConflict: true });
    standaloneProviders();
    await providerAction('Edit');
    const form = within(await screen.findByRole('dialog', { name: 'Edit provider a-base' }));
    fireEvent.click(form.getByRole('button', { name: 'Save changes' }));
    expect(await form.findByRole('button', { name: 'Check saved state' })).toBeInTheDocument();
    act(() => setAdminWorkspaceId(bId));
    await screen.findByText('b-base');
    const reads = state.inventoryReads().length;
    fireEvent.click(form.getByRole('button', { name: 'Check saved state' }));
    await form.findByText('Authentication context changed');
    fireEvent.click(form.getByRole('button', { name: 'Reload current values' }));
    await waitFor(() => expect(state.inventoryReads()).toHaveLength(reads));
    expect(form.getByRole('button', { name: 'Save changes' })).toBeDisabled();
    expect(state.writes).toHaveLength(1);
    expect(state.writes[0]?.workspace).toBe(aId);
  });

  it.each(['members', 'teams'])('never repeats stale A key-dialog %s queries in the B context', async (kind) => {
    setAdminWorkspaceId(aId);
    const held = deferred<AxiosResponse>();
    const requests: InternalAxiosRequestConfig[] = [];
    adminClient.defaults.adapter = (config) => {
      requests.push(config);
      return held.promise;
    };
    const context = { workspaceId: aId, generation: authLifecycle.generation, session: authLifecycle.session };
    const { result } = renderHook(
      () => (kind === 'members' ? useWorkspaceMembers(aId, true, context) : useWorkspaceTeams(aId, true, context)),
      { wrapper },
    );
    await waitFor(() => expect(requests).toHaveLength(1));
    act(() => setAdminWorkspaceId(bId));
    expect(requests[0]?.signal?.aborted).toBe(true);
    await act(async () => held.resolve(response(requests[0]!, { [kind]: [] })));
    expect(result.current.data).toBeUndefined();
    await act(async () => {
      await result.current.refetch();
    });
    expect(requests).toHaveLength(1);
  });
});
