import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { DialogManagerProvider } from '@egose/shadcn-theme/components/widgets/dialog-manager';
import axios, { AxiosError, type AxiosResponse, type InternalAxiosRequestConfig } from 'axios';
import { MemoryRouter } from 'react-router';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ProvidersPage } from './providers-page';
import { adminClient } from '../services/admin';
import { clearAdminSession, setAdminWorkspaceId, setAdminSession } from '../store';
import { adminProviderSchema, type AdminProvider } from '../types';

const originalAdapter = axios.defaults.adapter;
const originalAdminAdapter = adminClient.defaults.adapter;
let client: QueryClient;
let writes: Array<{ method: string | undefined; url: string | undefined; body: unknown }>;
let listReads = 0;

const provider = (overrides: Record<string, unknown> = {}) =>
  adminProviderSchema.parse({
    name: 'primary',
    type: 'openai',
    source: 'database',
    enabled: true,
    has_credential: true,
    models: [{ name: 'gpt-4o-mini' }],
    ...overrides,
  });
const customHealthcheck = {
  path: '/ready',
  method: 'HEAD',
  expected_status: 204,
  expected_body: 'ready',
  interval: '45s',
  timeout: '9s',
  failure_threshold: 4,
  success_threshold: 3,
  send_authorization: true,
};
const defaultHealthcheck = {
  path: '/ready',
  method: 'GET',
  expected_status: 200,
  expected_body: '*',
  interval: '30s',
  timeout: '5s',
  failure_threshold: 2,
  success_threshold: 1,
  send_authorization: false,
};

function response(config: InternalAxiosRequestConfig, data: unknown): AxiosResponse {
  return { config, data, status: 200, statusText: 'OK', headers: {} };
}

beforeEach(() => {
  setAdminSession('form-admin', 'form-refresh');
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  writes = [];
  listReads = 0;
});

afterEach(() => {
  cleanup();
  client.clear();
  axios.defaults.adapter = originalAdapter;
  adminClient.defaults.adapter = originalAdminAdapter;
  clearAdminSession();
});

function mount(initial: AdminProvider | undefined, savedViews: AdminProvider[] = []) {
  let current = initial;
  axios.defaults.adapter = async (config) => {
    if (config.url === '/_internal/admin/status') {
      return response(config, { multi_tenancy_enabled: true, web_ui_enabled: true, db_ok: true });
    }
    throw new Error(`Unexpected public request: ${config.url}`);
  };
  adminClient.defaults.adapter = async (config) => {
    if (config.method === 'put' || config.method === 'post') {
      writes.push({ method: config.method, url: config.url, body: JSON.parse(config.data) });
      const saved = savedViews.shift();
      if (!saved) throw new Error('Unexpected save');
      current = saved;
      return response(config, saved);
    }
    if (config.url === '/_internal/admin/providers') {
      listReads++;
      return response(config, { providers: current ? [current] : [] });
    }
    if (config.url === '/_internal/admin/provider-types') {
      return response(config, {
        provider_types: [
          {
            type: 'openai',
            credential: 'api_key',
            supports_healthcheck: true,
            supported_capabilities: ['chat', 'responses'],
          },
          { type: 'github-copilot', credential: 'credential_ref', supports_healthcheck: false },
        ],
      });
    }
    throw new Error(`Unexpected admin request: ${config.url}`);
  };
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

async function edit() {
  const trigger = await screen.findByRole('button', { name: 'Actions for primary' });
  fireEvent.keyDown(trigger, { key: 'Enter' });
  const item = await screen.findByRole('menuitem', { name: 'Edit' });
  await waitFor(() => expect(item).not.toHaveAttribute('data-disabled'));
  fireEvent.click(item);
  return within(await screen.findByRole('dialog', { name: 'Edit provider primary' }));
}

async function create() {
  const button = await screen.findByRole('button', { name: 'Create provider' });
  await waitFor(() => expect(button).toBeEnabled());
  fireEvent.click(button);
  return within(await screen.findByRole('dialog', { name: 'Create provider' }));
}

async function save() {
  const reads = listReads;
  fireEvent.click(screen.getByRole('button', { name: 'Save changes' }));
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
  await waitFor(() => expect(listReads).toBeGreaterThan(reads));
}

function change(label: string | RegExp, value: string) {
  fireEvent.change(screen.getByLabelText(label), { target: { value } });
}

describe('mounted provider form requests and reopened values', { timeout: 15000 }, () => {
  it('persists enabled in both directions and omits untouched write-only credentials and models', async () => {
    mount(provider({ enabled: false }), [provider(), provider({ enabled: false })]);
    let form = await edit();
    expect(form.getByRole('checkbox', { name: 'Enabled' })).not.toBeChecked();
    expect(form.getByLabelText('API key (stored encrypted, write-only)')).toHaveValue('');
    fireEvent.click(form.getByRole('checkbox', { name: 'Enabled' }));
    await save();
    expect(writes).toEqual([{ method: 'put', url: '/_internal/admin/providers/primary', body: { enabled: true } }]);
    form = await edit();
    expect(form.getByRole('checkbox', { name: 'Enabled' })).toBeChecked();
    expect(form.getByLabelText('API key (stored encrypted, write-only)')).toHaveValue('');
    fireEvent.click(form.getByRole('checkbox', { name: 'Enabled' }));
    await save();
    expect(writes[1].body).toEqual({ enabled: false });
    form = await edit();
    expect(form.getByRole('checkbox', { name: 'Enabled' })).not.toBeChecked();
  });

  it('clears local strings, header lists and forwarding without resending an API-key reference', async () => {
    const refs = { api_key_ref_key: 'account', api_key_ref_path: '/secrets/keys.json' }; // pragma: allowlist secret
    mount(
      provider({
        ...refs,
        display_name: 'Old label',
        base_url: 'https://old.example/v1',
        upstream_header_timeout: '12s',
        user_agent: 'custom/1',
        forward_user_agent: true,
        forward_headers: ['x-custom', 'x-session-id'],
      }),
      [provider(refs)],
    );
    let form = await edit();
    fireEvent.change(form.getAllByLabelText('Display name (optional)')[0], { target: { value: '' } });
    for (const label of [/^Base URL/, /^Upstream header timeout/, /^User agent override/, /^Forward headers/])
      change(label, '');
    fireEvent.click(form.getByRole('checkbox', { name: 'Forward caller user agent' }));
    expect(form.getByText(/root forwarding still applies/)).toBeInTheDocument();
    await save();
    expect(writes[0].body).toEqual({
      display_name: '',
      base_url: '',
      upstream_header_timeout: '',
      user_agent: '',
      forward_user_agent: false,
      forward_headers: [],
    });
    form = await edit();
    expect(form.getAllByLabelText('Display name (optional)')[0]).toHaveValue('');
    for (const label of [/^Base URL/, /^Upstream header timeout/, /^User agent override/, /^Forward headers/]) {
      expect(form.getByLabelText(label)).toHaveValue('');
    }
    expect(form.getByRole('checkbox', { name: 'Forward caller user agent' })).not.toBeChecked();
    expect(form.getByLabelText('API key reference key (alternative to API key)')).toHaveValue('account');
    expect(form.getByLabelText('API key reference path (optional)')).toHaveValue('/secrets/keys.json');
  });

  it('sends false authorization and resets each healthcheck scalar to the API defaults', async () => {
    mount(provider({ healthcheck: customHealthcheck }), [provider({ healthcheck: defaultHealthcheck })]);
    let form = await edit();
    expect(form.getByRole('checkbox', { name: 'Custom healthcheck' })).toBeDisabled();
    expect(form.getByText(/cannot remove an existing healthcheck/)).toBeInTheDocument();
    for (const label of [
      'Method',
      'Expected status',
      /^Expected body/,
      'Interval',
      'Timeout',
      'Failure threshold',
      'Success threshold',
    ]) {
      change(label, '');
    }
    fireEvent.click(form.getByRole('checkbox', { name: 'Send authorization header' }));
    await save();
    expect(writes[0].body).toEqual({
      healthcheck: {
        method: '',
        expected_status: 0,
        expected_body: '',
        interval: '',
        timeout: '',
        failure_threshold: 0,
        success_threshold: 0,
        send_authorization: false,
      },
    });
    form = await edit();
    for (const [label, value] of [
      ['Path (required)', '/ready'],
      ['Method', 'GET'],
      ['Expected status', '200'],
      ['Expected body (optional, * matches anything)', '*'],
      ['Interval', '30s'],
      ['Timeout', '5s'],
      ['Failure threshold', '2'],
      ['Success threshold', '1'],
    ])
      expect(form.getByLabelText(label)).toHaveValue(value);
    expect(form.getByRole('checkbox', { name: 'Send authorization header' })).not.toBeChecked();
    expect(form.getByRole('checkbox', { name: 'Custom healthcheck' })).toBeChecked();
  });

  it('rejects clearing the healthcheck path visibly and preserves the block on an unrelated save', async () => {
    mount(provider({ healthcheck: customHealthcheck }), [provider({ enabled: false, healthcheck: customHealthcheck })]);
    const form = await edit();
    change('Path (required)', '');
    fireEvent.click(form.getByRole('button', { name: 'Save changes' }));
    expect(await form.findByText(/Enter a healthcheck path/)).toBeInTheDocument();
    expect(writes).toEqual([]);
    change('Path (required)', '/ready');
    fireEvent.click(form.getByRole('checkbox', { name: 'Enabled' }));
    await save();
    expect(writes[0].body).toEqual({ enabled: false });
    const reopened = await edit();
    expect(reopened.getByLabelText('Path (required)')).toHaveValue('/ready');
    expect(reopened.getByRole('checkbox', { name: 'Send authorization header' })).toBeChecked();
  });

  it('edits an inherited provider with no local models while retaining its Copilot reference', async () => {
    const inherited = provider({
      type: 'github-copilot',
      extends: 'base',
      models: [],
      display_name: 'Local label',
      copilot_credential_name: 'account',
      copilot_credential_path: '/secrets/keys.json',
    });
    mount(inherited, [{ ...inherited, display_name: '', enabled: false }]);
    let form = await edit();
    expect(form.getByText(/Models, transport settings and healthcheck are inherited/)).toBeInTheDocument();
    expect(form.queryByRole('button', { name: 'Add model' })).not.toBeInTheDocument();
    expect(form.queryByLabelText(/^Base URL/)).not.toBeInTheDocument();
    expect(form.queryByRole('checkbox', { name: 'Custom healthcheck' })).not.toBeInTheDocument();
    expect(form.getByLabelText('Credential source')).toHaveValue('keep');
    expect(form.getByText(/stored credential stays unchanged/)).toBeInTheDocument();
    change('Display name (optional)', '');
    fireEvent.click(form.getByRole('checkbox', { name: 'Enabled' }));
    await save();
    expect(writes[0].body).toEqual({ display_name: '', enabled: false });
    form = await edit();
    expect(form.getByLabelText(/^Extends/)).toHaveValue('base');
    fireEvent.change(form.getByLabelText('Credential source'), { target: { value: 'sidecar' } });
    expect(form.getByLabelText(/^Credential name/)).toHaveValue('account');
    expect(form.getByLabelText('Credential path (optional)')).toHaveValue('/secrets/keys.json');
    expect(form.getByLabelText('Display name (optional)')).toHaveValue('');
    expect(form.getByRole('checkbox', { name: 'Enabled' })).not.toBeChecked();
  });

  it.each(['api_key', 'credential_ref'])(
    'clears a changed %s reference path without clearing its identity',
    async (kind) => {
      const copilot = kind === 'credential_ref';
      const before = copilot
        ? provider({
            type: 'github-copilot',
            copilot_credential_name: 'account',
            copilot_credential_path: '/old/keys.json',
          })
        : provider({ api_key_ref_key: 'account', api_key_ref_path: '/old/keys.json' });
      const after = { ...before, api_key_ref_path: '', copilot_credential_path: '' };
      mount(before, [after]);
      const form = await edit();
      if (copilot) {
        fireEvent.change(form.getByLabelText('Credential source'), { target: { value: 'sidecar' } });
      }
      const label = copilot ? 'Credential path (optional)' : 'API key reference path (optional)';
      change(label, '');
      await save();
      expect(writes[0].body).toEqual(
        copilot ? { credential_ref: { name: 'account', path: '' } } : { api_key_ref: { key: 'account', path: '' } },
      );
      const reopened = await edit();
      if (copilot) {
        fireEvent.change(reopened.getByLabelText('Credential source'), { target: { value: 'sidecar' } });
      }
      expect(reopened.getByLabelText(label)).toHaveValue('');
    },
  );

  it('uses create defaults and sends a newly entered secret only on creation', async () => {
    mount(undefined, [provider(), provider()]);
    const form = await create();
    change('Name (lowercase, no spaces)', 'primary');
    change('Name', 'gpt-4o-mini');
    change('API key (stored encrypted, write-only)', 'fixture-key');
    expect(form.getByRole('checkbox', { name: 'Enabled' })).toBeChecked();
    expect(form.queryByLabelText('Path (required)')).not.toBeInTheDocument();
    fireEvent.click(form.getByRole('button', { name: 'Create provider' }));
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    expect(writes).toEqual([
      {
        method: 'post',
        url: '/_internal/admin/providers',
        body: { name: 'primary', type: 'openai', models: [{ name: 'gpt-4o-mini' }], api_key: 'fixture-key' }, // pragma: allowlist secret
      },
    ]);
    const reopened = await edit();
    expect(reopened.getByRole('checkbox', { name: 'Enabled' })).toBeChecked();
    expect(reopened.getByLabelText('API key (stored encrypted, write-only)')).toHaveValue('');
    await save();
    expect(writes[1].body).toEqual({});
  });

  it('creates an inherited provider without a local model', async () => {
    mount(undefined, [provider({ extends: 'base', models: [] })]);
    const form = await create();
    change('Name (lowercase, no spaces)', 'primary');
    change(/^Extends/, 'base');
    change('API key (stored encrypted, write-only)', 'fixture-key');
    expect(form.queryByLabelText('Name', { exact: true })).not.toBeInTheDocument();
    fireEvent.click(form.getByRole('button', { name: 'Create provider' }));
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    expect(writes[0].body).toEqual({ name: 'primary', type: 'openai', extends: 'base', api_key: 'fixture-key' }); // pragma: allowlist secret
    expect((await edit()).getByLabelText(/^Extends/)).toHaveValue('base');
  });

  it.each(['create', 'edit'])(
    'adds a healthcheck in %s with defaults and explicit forwarding/auth enabled',
    async (mode) => {
      const saved = provider({
        enabled: false,
        forward_user_agent: true,
        healthcheck: { ...defaultHealthcheck, send_authorization: true },
      });
      mount(mode === 'create' ? undefined : provider(), [saved]);
      const form = mode === 'create' ? await create() : await edit();
      if (mode === 'create') {
        change('Name (lowercase, no spaces)', 'primary');
        change('Name', 'gpt-4o-mini');
      }
      fireEvent.click(form.getByRole('checkbox', { name: 'Enabled' }));
      fireEvent.click(form.getByRole('checkbox', { name: 'Forward caller user agent' }));
      fireEvent.click(form.getByRole('checkbox', { name: 'Custom healthcheck' }));
      change('Path (required)', '/ready');
      fireEvent.click(form.getByRole('checkbox', { name: 'Send authorization header' }));
      fireEvent.click(form.getByRole('button', { name: mode === 'create' ? 'Create provider' : 'Save changes' }));
      await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
      expect(writes[0].body).toEqual({
        ...(mode === 'create' ? { name: 'primary', type: 'openai', models: [{ name: 'gpt-4o-mini' }] } : {}),
        enabled: false,
        forward_user_agent: true,
        healthcheck: { path: '/ready', method: 'GET', send_authorization: true },
      });
      const reopened = await edit();
      expect(reopened.getByRole('checkbox', { name: 'Enabled' })).not.toBeChecked();
      expect(reopened.getByRole('checkbox', { name: 'Forward caller user agent' })).toBeChecked();
      expect(reopened.getByRole('checkbox', { name: 'Send authorization header' })).toBeChecked();
      expect(reopened.getByLabelText('Expected status')).toHaveValue('200');
      expect(reopened.getByLabelText('Timeout')).toHaveValue('5s');
    },
  );

  it('replaces edited models so cleared model strings, pricing and capability lists use defaults', async () => {
    mount(
      provider({
        models: [
          {
            name: 'gpt-4o-mini',
            upstream_name: 'upstream',
            display_name: 'Model label',
            capabilities: ['chat'],
            pricing: { input_per_million: 7 },
          },
        ],
      }),
      [provider()],
    );
    const form = await edit();
    change('Upstream name (defaults to name)', '');
    fireEvent.change(form.getAllByLabelText('Display name (optional)')[1], { target: { value: '' } });
    change('Input', '');
    fireEvent.click(form.getByRole('checkbox', { name: 'chat' }));
    await save();
    expect(writes[0].body).toEqual({ models: [{ name: 'gpt-4o-mini' }] });
    const reopened = await edit();
    expect(reopened.getByLabelText('Upstream name (defaults to name)')).toHaveValue('');
    expect(reopened.getAllByLabelText('Display name (optional)')[1]).toHaveValue('');
    expect(reopened.getByLabelText('Input')).toHaveValue('');
    expect(reopened.getByRole('checkbox', { name: 'chat' })).not.toBeChecked();
  });

  it('clears inheritance only with valid local models and leaves the stored credential untouched', async () => {
    mount(provider({ extends: 'base', models: [] }), [provider()]);
    const form = await edit();
    change(/^Extends/, '');
    fireEvent.click(form.getByRole('button', { name: 'Save changes' }));
    expect(await form.findByText('Enter a model name.')).toBeInTheDocument();
    expect(writes).toEqual([]);
    change('Name', 'gpt-4o-mini');
    await save();
    expect(writes[0].body).toEqual({ extends: '', models: [{ name: 'gpt-4o-mini' }] });
    const reopened = await edit();
    expect(reopened.getByLabelText(/^Extends/)).toHaveValue('');
    expect(reopened.getByLabelText('Name', { exact: true })).toHaveValue('gpt-4o-mini');
  });
});

describe('copilot device authorization panel', { timeout: 20000 }, () => {
  const copilotProvider = (overrides: Record<string, unknown> = {}) =>
    provider({
      type: 'github-copilot',
      models: [{ name: 'gpt-4o', protocol: 'chat' }],
      copilot_credential_source: 'database',
      updated_at: '2026-09-27T10:00:00.000000000Z',
      ...overrides,
    });
  const future = (ms: number) => new Date(Date.now() + ms).toISOString();
  const flowPending = (id = 'flow-1', extra: Record<string, unknown> = {}) => ({
    id,
    workspace_id: 'ws-1',
    status: 'pending',
    user_code: 'ABCD-1234',
    verification_uri: 'https://github.com/login/device',
    expires_at: future(300_000),
    poll_after_ms: 15,
    ...extra,
  });
  const flowReady = (id = 'flow-1', extra: Record<string, unknown> = {}) => ({
    id,
    workspace_id: 'ws-1',
    status: 'ready',
    ready_expires_at: future(600_000),
    provider_name: '',
    ...extra,
  });

  function failResponse(
    config: InternalAxiosRequestConfig,
    status: number,
    data: unknown,
    headers: Record<string, string> = {},
  ): never {
    const res = { config, data, status, statusText: String(status), headers } as AxiosResponse;
    throw new AxiosError(typeof data === 'string' ? data : `status ${status}`, String(status), config, undefined, res);
  }

  function mountDevice(options: {
    initial?: AdminProvider;
    savedViews?: AdminProvider[];
    onStart?: (body: Record<string, unknown>) => Record<string, unknown>;
    onPoll?: (id: string, count: number) => Record<string, unknown>;
    onSave?: (body: Record<string, unknown>, config: InternalAxiosRequestConfig) => AdminProvider | never;
  }) {
    const { initial, savedViews = [], onStart, onPoll, onSave } = options;
    let current = initial;
    const queue = [...savedViews];
    const flowCalls = { starts: [] as unknown[], polls: 0, cancels: 0, gets: 0 };
    const flows = new Map<string, Record<string, unknown>>();
    setAdminSession('form-admin', 'form-refresh');
    setAdminWorkspaceId('ws-1');
    client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    writes = [];
    listReads = 0;
    axios.defaults.adapter = async (config) => {
      if (config.url === '/_internal/admin/status') {
        return response(config, { multi_tenancy_enabled: true, web_ui_enabled: true, db_ok: true });
      }
      throw new Error(`Unexpected public request: ${config.url}`);
    };
    adminClient.defaults.adapter = async (config) => {
      const url = config.url ?? '';
      const method = config.method;
      if (url === '/_internal/admin/copilot-device-flows' && method === 'post') {
        const body = JSON.parse(config.data as string) as Record<string, unknown>;
        flowCalls.starts.push(body);
        if ((config as { timeout?: number }).timeout !== 25_000) {
          throw new Error(`start timeout must be 25s, got ${(config as { timeout?: number }).timeout}`);
        }
        const next = onStart ? onStart(body) : flowPending(`flow-${flowCalls.starts.length}`);
        flows.set(next.id as string, next);
        return response(config, next);
      }
      const pollMatch = url.match(/\/copilot-device-flows\/([^/]+)\/poll$/);
      if (pollMatch && method === 'post') {
        flowCalls.polls++;
        if ((config as { timeout?: number }).timeout !== 25_000) {
          throw new Error('poll timeout must be 25s');
        }
        const id = decodeURIComponent(pollMatch[1]!);
        const next = onPoll ? onPoll(id, flowCalls.polls) : (flows.get(id) ?? flowReady(id));
        flows.set(id, next);
        return response(config, next);
      }
      const flowIdMatch = url.match(/\/copilot-device-flows\/([^/]+)$/);
      if (flowIdMatch && (method === 'get' || method === 'delete')) {
        const id = decodeURIComponent(flowIdMatch[1]!);
        if (method === 'get') flowCalls.gets++;
        else flowCalls.cancels++;
        const currentFlow = flows.get(id);
        if (!currentFlow) failResponse(config, 404, 'device flow not found');
        if (method === 'delete') {
          const cancelled = { ...currentFlow, status: 'cancelled', error_code: 'cancelled' };
          flows.set(id, cancelled);
          return response(config, cancelled);
        }
        return response(config, currentFlow);
      }
      if ((method === 'put' || method === 'post') && url.startsWith('/_internal/admin/providers')) {
        const body = JSON.parse(config.data as string) as Record<string, unknown>;
        writes.push({ method, url, body });
        if (onSave) {
          const result = onSave(body, config);
          current = result;
          return response(config, result);
        }
        const saved = queue.shift();
        if (!saved) throw new Error('Unexpected save');
        current = saved;
        return response(config, saved);
      }
      if (url === '/_internal/admin/providers') {
        listReads++;
        return response(config, { providers: current ? [current] : [] });
      }
      if (url === '/_internal/admin/provider-types') {
        return response(config, {
          provider_types: [
            { type: 'openai', credential: 'api_key', supports_healthcheck: true },
            {
              type: 'github-copilot',
              credential: 'credential_ref',
              supports_healthcheck: false,
              supports_device_authorization: true,
            },
          ],
        });
      }
      throw new Error(`Unexpected admin request: ${method} ${url}`);
    };
    render(
      <QueryClientProvider client={client}>
        <MemoryRouter>
          <DialogManagerProvider>
            <ProvidersPage />
          </DialogManagerProvider>
        </MemoryRouter>
      </QueryClientProvider>,
    );
    return { flowCalls, flows };
  }

  it('creates with a device flow, gating save until ready and sending flow ID only', async () => {
    mountDevice({
      savedViews: [copilotProvider({ name: 'copilot-one' })],
      onStart: (body) => {
        expect(body.client_id).toBe('Iv1.test');
        return flowPending('flow-1');
      },
      onPoll: () => flowReady('flow-1'),
    });
    const form = await create();
    fireEvent.change(form.getByLabelText('Type'), { target: { value: 'github-copilot' } });
    expect(form.getByLabelText('Credential source')).toHaveValue('device');
    expect(form.queryByLabelText('API key (stored encrypted, write-only)')).not.toBeInTheDocument();
    expect(form.getByRole('button', { name: 'Create provider' })).toBeDisabled();
    expect(form.getByText(/Save is available after authorization is ready/)).toBeInTheDocument();
    change('Name (lowercase, no spaces)', 'copilot-one');
    change('Name', 'gpt-4o');
    fireEvent.change(form.getByLabelText('GitHub OAuth client ID'), { target: { value: 'Iv1.test' } });
    fireEvent.click(form.getByRole('button', { name: 'Connect GitHub' }));
    expect(await form.findByText(/Waiting for GitHub approval/)).toBeInTheDocument();
    expect(form.getByText('ABCD-1234')).toBeInTheDocument();
    expect(form.getByRole('link', { name: 'Open GitHub to authorize' })).toHaveAttribute(
      'href',
      'https://github.com/login/device',
    );
    expect(await form.findByText('Authorized; save provider to apply.')).toBeInTheDocument();
    await waitFor(() => expect(form.getByRole('button', { name: 'Create provider' })).toBeEnabled());
    fireEvent.click(form.getByRole('button', { name: 'Create provider' }));
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    expect(writes).toHaveLength(1);
    expect(writes[0].body).toMatchObject({ name: 'copilot-one', copilot_device_flow_id: 'flow-1' });
    expect(writes[0].body).not.toHaveProperty('credential_ref');
    expect(JSON.stringify(writes[0].body)).not.toContain('ABCD-1234');
  });

  it('keeps edit credentials by default and reauthorizes with revision', async () => {
    const before = copilotProvider({ name: 'primary' });
    mountDevice({
      initial: before,
      savedViews: [
        copilotProvider({ name: 'primary', updated_at: '2026-09-27T11:00:00.000000000Z' }),
        copilotProvider({ name: 'primary', updated_at: '2026-09-27T12:00:00.000000000Z' }),
      ],
      onStart: (body) => {
        expect(body.provider_name).toBe('primary');
        expect(body).not.toHaveProperty('workspace_id');
        return flowPending('flow-edit');
      },
      onPoll: () => flowReady('flow-edit'),
    });
    let form = await edit();
    expect(form.getByLabelText('Credential source')).toHaveValue('keep');
    await save();
    expect(writes[0].body).toEqual({ expected_updated_at: '2026-09-27T10:00:00.000000000Z' });
    form = await edit();
    fireEvent.change(form.getByLabelText('Credential source'), { target: { value: 'device' } });
    fireEvent.change(form.getByLabelText('GitHub OAuth client ID'), { target: { value: 'Iv1.edit' } });
    fireEvent.click(form.getByRole('button', { name: 'Connect GitHub' }));
    expect(await form.findByText('Authorized; save provider to apply.')).toBeInTheDocument();
    await save();
    expect(writes[1].body).toMatchObject({
      copilot_device_flow_id: 'flow-edit',
      expected_updated_at: '2026-09-27T11:00:00.000000000Z',
    });
    expect(writes[1].body).not.toHaveProperty('credential_ref');
  });

  it('switches to sidecar and never shows an API-key form for Copilot', async () => {
    const before = copilotProvider({ name: 'primary', copilot_credential_source: 'database' });
    mountDevice({ initial: before, savedViews: [before] });
    const form = await edit();
    expect(form.queryByLabelText('API key (stored encrypted, write-only)')).not.toBeInTheDocument();
    expect(form.queryByLabelText('API key reference key (alternative to API key)')).not.toBeInTheDocument();
    fireEvent.change(form.getByLabelText('Credential source'), { target: { value: 'sidecar' } });
    fireEvent.change(form.getByLabelText(/^Credential name/), { target: { value: 'cli-name' } });
    await save();
    expect(writes[0].body).toEqual({
      credential_ref: { path: '', name: 'cli-name' },
      expected_updated_at: '2026-09-27T10:00:00.000000000Z',
    });
  });

  it('handles denial then retry and cancel without overlapping polls', async () => {
    let starts = 0;
    const state = mountDevice({
      onStart: () => {
        starts++;
        return flowPending(`flow-retry-${starts}`);
      },
      onPoll: (id) => {
        if (id === 'flow-retry-1') return { ...flowPending(id), status: 'denied', error_code: 'access_denied' };
        return { ...flowPending(id), poll_after_ms: 2000, status: 'pending' };
      },
    });
    const form = await create();
    fireEvent.change(form.getByLabelText('Type'), { target: { value: 'github-copilot' } });
    change('Name', 'gpt-4o');
    fireEvent.change(form.getByLabelText('GitHub OAuth client ID'), { target: { value: 'Iv1.x' } });
    fireEvent.click(form.getByRole('button', { name: 'Connect GitHub' }));
    expect(await form.findByText(/Authorization denied/, undefined, { timeout: 5000 })).toBeInTheDocument();
    const pollsAfterDenial = state.flowCalls.polls;
    fireEvent.click(form.getByRole('button', { name: 'Retry authorization' }));
    expect(await form.findByText(/Waiting for GitHub approval/)).toBeInTheDocument();
    await waitFor(() => expect(state.flowCalls.polls).toBeGreaterThan(pollsAfterDenial));
    fireEvent.click(form.getByRole('button', { name: 'Cancel authorization' }));
    expect(await form.findByText(/Authorization cancelled/)).toBeInTheDocument();
  });

  it('preserves ready flow across a 409 and recovers via reread', async () => {
    const before = copilotProvider({ name: 'primary' });
    let saves = 0;
    mountDevice({
      initial: before,
      onStart: () => flowPending('flow-conflict'),
      onPoll: () => flowReady('flow-conflict'),
      onSave: (body, config) => {
        saves++;
        if (saves === 1) {
          expect(body.copilot_device_flow_id).toBe('flow-conflict');
          failResponse(config, 409, 'catalog changed concurrently; read current state and retry');
        }
        return copilotProvider({ name: 'primary', updated_at: '2026-09-27T12:00:00.000000000Z' });
      },
    });
    const form = await edit();
    fireEvent.change(form.getByLabelText('Credential source'), { target: { value: 'device' } });
    fireEvent.change(form.getByLabelText('GitHub OAuth client ID'), { target: { value: 'Iv1.c' } });
    fireEvent.click(form.getByRole('button', { name: 'Connect GitHub' }));
    expect(await form.findByText('Authorized; save provider to apply.')).toBeInTheDocument();
    fireEvent.click(form.getByRole('button', { name: 'Save changes' }));
    expect(await form.findByText(/reread current state/)).toBeInTheDocument();
    expect(screen.queryByRole('dialog')).toBeInTheDocument();
    expect(await form.findByText('Authorized; save provider to apply.')).toBeInTheDocument();
    fireEvent.click(form.getByRole('button', { name: 'Check saved state' }));
    expect(await form.findByText(/Current state loaded/)).toBeInTheDocument();
    fireEvent.click(form.getByRole('button', { name: 'Reload current values' }));
    expect(await form.findByText(/Reloaded revision/)).toBeInTheDocument();
    fireEvent.click(form.getByRole('button', { name: 'Save changes' }));
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    expect(saves).toBe(2);
  });

  it('reports saved-but-activation-failed and recovers network loss without duplicate creation', async () => {
    const before = copilotProvider({ name: 'primary' });
    mountDevice({
      initial: before,
      onStart: () => flowPending('flow-loss'),
      onPoll: () => flowReady('flow-loss'),
      onSave: (body, config) => {
        expect(body.copilot_device_flow_id).toBe('flow-loss');
        failResponse(config, 500, 'saved but activation failed', { 'x-aiproxy-catalog-saved': 'true' });
      },
    });
    const form = await edit();
    fireEvent.change(form.getByLabelText('Credential source'), { target: { value: 'device' } });
    fireEvent.change(form.getByLabelText('GitHub OAuth client ID'), { target: { value: 'Iv1.s' } });
    fireEvent.click(form.getByRole('button', { name: 'Connect GitHub' }));
    expect(await form.findByText('Authorized; save provider to apply.')).toBeInTheDocument();
    fireEvent.click(form.getByRole('button', { name: 'Save changes' }));
    expect(await form.findByText(/saved, but activation failed/)).toBeInTheDocument();
    expect(screen.queryByRole('dialog')).toBeInTheDocument();
  });

  it('opens reauthorization for Copilot rotate instead of the API-key form', async () => {
    const copilot = copilotProvider({ name: 'primary' });
    mountDevice({ initial: copilot, savedViews: [copilot] });
    const trigger = await screen.findByRole('button', { name: 'Actions for primary' });
    fireEvent.keyDown(trigger, { key: 'Enter' });
    const item = await screen.findByRole('menuitem', { name: 'Rotate credential' });
    fireEvent.click(item);
    const dialog = await screen.findByRole('dialog', { name: 'Edit provider primary' });
    const scoped = within(dialog);
    expect(scoped.getByLabelText('Credential source')).toBeInTheDocument();
    expect(scoped.queryByLabelText('New API key')).not.toBeInTheDocument();
    expect(scoped.queryByLabelText('API key (stored encrypted, write-only)')).not.toBeInTheDocument();
  });

  it('ignores late poll results after source switching and blocks wrong-workspace save', async () => {
    let releasePoll!: (v: Record<string, unknown>) => void;
    const gate = new Promise<Record<string, unknown>>((resolve) => {
      releasePoll = resolve as (v: Record<string, unknown>) => void;
    });
    let pollStarted = false;
    const state = mountDevice({
      onStart: () => flowPending('flow-late', { poll_after_ms: 10 }),
      onPoll: () => {
        pollStarted = true;
        return gate as unknown as Record<string, unknown>;
      },
    });
    void state;
    const form = await create();
    fireEvent.change(form.getByLabelText('Type'), { target: { value: 'github-copilot' } });
    change('Name', 'gpt-4o');
    fireEvent.change(form.getByLabelText('GitHub OAuth client ID'), { target: { value: 'Iv1.late' } });
    fireEvent.click(form.getByRole('button', { name: 'Connect GitHub' }));
    expect(await form.findByText(/Waiting for GitHub approval/)).toBeInTheDocument();
    await waitFor(() => expect(pollStarted).toBe(true));
    fireEvent.change(form.getByLabelText('Credential source'), { target: { value: 'sidecar' } });
    expect(form.queryByText(/Waiting for GitHub approval/)).not.toBeInTheDocument();
    releasePoll(flowReady('flow-late'));
    await vi.waitFor(() => expect(form.queryByText('Authorized; save provider to apply.')).not.toBeInTheDocument());
    fireEvent.change(form.getByLabelText('Credential source'), { target: { value: 'device' } });
    fireEvent.change(form.getByLabelText('GitHub OAuth client ID'), { target: { value: 'Iv1.again' } });
    fireEvent.click(form.getByRole('button', { name: 'Connect GitHub' }));
    expect(await form.findByText(/Waiting for GitHub approval/, undefined, { timeout: 5000 })).toBeInTheDocument();
    setAdminWorkspaceId('ws-other');
    await waitFor(() => expect(form.getByText(/Workspace changed/)).toBeInTheDocument());
    expect(form.getByRole('button', { name: 'Create provider' })).toBeDisabled();
  });
});
