import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { DialogManagerProvider } from '@egose/shadcn-theme/components/widgets/dialog-manager';
import axios, { type AxiosResponse, type InternalAxiosRequestConfig } from 'axios';
import { MemoryRouter } from 'react-router';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { AliasesPage } from './aliases-page';
import { adminClient } from '../services/admin';
import { clearAdminSession, setAdminSession } from '../store';
import { adminAliasSchema, type AdminAlias } from '../types';

const originalAdapter = axios.defaults.adapter;
const originalAdminAdapter = adminClient.defaults.adapter;
let client: QueryClient;
let writes: Array<{ method: string | undefined; url: string | undefined; body: unknown }>;
let listReads = 0;

const targets = [
  { provider: 'gateway', model: 'gpt-4o-mini' },
  { provider: 'gateway', model: 'z-ai' },
  { provider: 'gateway', model: 'z-ai/glm-5.2' },
  { provider: 'backup_2.eu', model: 'org_1/family.v2/3-model' },
];
const targetText = targets.map((t) => `${t.provider}/${t.model}`).join('\n');
const alias = (overrides: Record<string, unknown> = {}) =>
  adminAliasSchema.parse({
    name: 'fast',
    algorithm: 'round_robin',
    source: 'database',
    targets,
    ...overrides,
  });

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

function mount(initial?: AdminAlias, savedViews: AdminAlias[] = [], saveError?: string) {
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
      if (saveError) throw new Error(saveError);
      const saved = savedViews.shift();
      if (!saved) throw new Error('Unexpected save');
      current = saved;
      return response(config, saved);
    }
    if (config.url === '/_internal/admin/aliases') {
      listReads++;
      return response(config, { aliases: current ? [current] : [] });
    }
    throw new Error(`Unexpected admin request: ${config.url}`);
  };
  render(
    <QueryClientProvider client={client}>
      <MemoryRouter>
        <DialogManagerProvider>
          <AliasesPage />
        </DialogManagerProvider>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

async function create() {
  fireEvent.click(await screen.findByRole('button', { name: 'Create alias' }));
  const form = within(await screen.findByRole('dialog', { name: 'Create alias' }));
  fireEvent.change(form.getByLabelText(/^Name/), { target: { value: 'fast' } });
  return form;
}

async function edit() {
  fireEvent.keyDown(await screen.findByRole('button', { name: 'Actions for fast' }), { key: 'Enter' });
  fireEvent.click(await screen.findByRole('menuitem', { name: 'Edit' }));
  return within(await screen.findByRole('dialog', { name: 'Edit alias alias/fast' }));
}

async function save(label = 'Save changes') {
  const reads = listReads;
  fireEvent.click(within(screen.getByRole('dialog')).getByRole('button', { name: label }));
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
  await waitFor(() => expect(listReads).toBeGreaterThan(reads));
}

function change(label: string | RegExp, value: string) {
  fireEvent.change(screen.getByLabelText(label), { target: { value } });
}

describe('mounted alias target requests and reopened values', { timeout: 15000 }, () => {
  it('creates full and shortened collision targets, preserves deeper names and ordinary whitespace, then saves unchanged', async () => {
    mount(undefined, [alias(), alias()]);
    await create();
    change(
      /^Targets/,
      ' \r\n gateway / gpt-4o-mini \r\n\tgateway/z-ai\t\n gateway / z-ai/glm-5.2 \n\n backup_2.eu/org_1/family.v2/3-model \n',
    );
    await save('Create alias');
    const body = { name: 'fast', algorithm: 'round_robin', targets };
    expect(writes).toEqual([{ method: 'post', url: '/_internal/admin/aliases', body }]);
    expect((await edit()).getByLabelText(/^Targets/)).toHaveValue(targetText);
    await save();
    expect(writes[1]).toEqual({ method: 'put', url: '/_internal/admin/aliases/fast', body });
    expect((await edit()).getByLabelText(/^Targets/)).toHaveValue(targetText);
  });

  it.each(['round_robin', 'least_connections'])(
    'saves an unchanged %s alias with collision targets and all routing options',
    async (algorithm) => {
      const options = {
        algorithm,
        retry_status_codes: [429, 500, 503],
        session_affinity: { headers: ['x-session-id', 'x-client'] },
        encrypted_reasoning: {
          passthrough: false,
          on_caller_mismatch: 'strip_and_retry',
          match_messages: ['invalid signature', 'caller mismatch'],
        },
      };
      mount(alias(options), [alias(options)]);
      let form = await edit();
      expect(form.getByLabelText(/^Targets/)).toHaveValue(targetText);
      await save();
      expect(writes).toEqual([
        { method: 'put', url: '/_internal/admin/aliases/fast', body: { name: 'fast', targets, ...options } },
      ]);
      form = await edit();
      expect(form.getByLabelText(/^Targets/)).toHaveValue(targetText);
      expect(form.getByLabelText('Algorithm')).toHaveValue(algorithm);
      expect(form.getByLabelText(/^Retry status/)).toHaveValue('429,500,503');
      expect(form.getByRole('checkbox', { name: 'Session affinity' })).toBeChecked();
      expect(form.getByLabelText(/^Headers/)).toHaveValue('x-session-id, x-client');
      expect(form.getByRole('checkbox', { name: 'Encrypted reasoning handling' })).toBeChecked();
      expect(form.getByRole('checkbox', { name: 'Passthrough' })).not.toBeChecked();
      expect(form.getByLabelText(/^On caller mismatch/)).toHaveValue('strip_and_retry');
      expect(form.getByLabelText(/^Match messages/)).toHaveValue('invalid signature\ncaller mismatch');
    },
  );

  it.each(['gpt-4o-mini', 'z-ai/glm-5.2', 'org_1/family.v2/3-model'])(
    'creates shorthand model %s and reopens as unchanged full targets',
    async (model) => {
      const expanded = [
        { provider: 'gateway', model },
        { provider: 'backup_2.eu', model },
      ];
      const saved = alias({ targets: expanded, algorithm: 'least_connections' });
      mount(undefined, [saved, saved]);
      await create();
      change('Providers', ' gateway , backup_2.eu ');
      change('Model', ` ${model}\t`);
      change('Algorithm', 'least_connections');
      await save('Create alias');
      expect(writes[0]).toEqual({
        method: 'post',
        url: '/_internal/admin/aliases',
        body: {
          name: 'fast',
          algorithm: 'least_connections',
          providers: ['gateway', 'backup_2.eu'],
          model,
        },
      });
      let form = await edit();
      expect(form.getByLabelText(/^Targets/)).toHaveValue(`gateway/${model}\nbackup_2.eu/${model}`);
      expect(form.getByLabelText('Providers')).toHaveValue('');
      expect(form.getByLabelText('Model')).toHaveValue('');
      await save();
      expect(writes[1].body).toEqual({ name: 'fast', algorithm: 'least_connections', targets: expanded });
      form = await edit();
      expect(form.getByLabelText(/^Targets/)).toHaveValue(`gateway/${model}\nbackup_2.eu/${model}`);
    },
  );

  it.each([
    ['', /Enter at least one target/],
    ['gateway', /Line 2: Enter provider\/model/],
    ['/model', /Line 2: Provider/],
    ['gateway/', /Line 2: Model/],
    ['gateway//model', /Line 2: Model/],
    ['gateway/org//model', /Line 2: Model/],
    ['gateway/org/model/', /Line 2: Model/],
    ['gateway/Org/model', /Line 2: Model/],
    ['gateway/org/-model', /Line 2: Model/],
    ['gateway/org/model:latest', /Line 2: Model/],
    ['gateway/org/ model', /Line 2: Model/],
    ['gate way/model', /Line 2: Provider/],
    ['Gateway/model', /Line 2: Provider/],
    ['-gateway/model', /Line 2: Provider/],
    ['alias/model', /Line 2: Provider/],
    ['gateway\\model', /Line 2: Enter provider\/model/],
    ['gateway:model', /Line 2: Enter provider\/model/],
    ['gateway/model,backup/model', /Line 2: Model/],
  ])('rejects malformed target %j visibly before HTTP', async (raw, message) => {
    mount();
    const form = await create();
    change(/^Targets/, raw ? `gateway/gpt-4o-mini\n${raw}` : ' \n\t');
    fireEvent.click(form.getByRole('button', { name: 'Create alias' }));
    expect(await form.findByText(message)).toBeVisible();
    expect(form.getByLabelText(/^Targets/)).toHaveValue(raw ? `gateway/gpt-4o-mini\n${raw}` : ' \n\t');
    expect(writes).toEqual([]);
  });

  it.each([
    ['', 'model', 'Providers', /Enter at least one provider/],
    ['gateway', '', 'Model', /Model must/],
    ['gateway,,backup', 'model', 'Providers', /Provider 2:/],
    ['gateway,', 'model', 'Providers', /Provider 2:/],
    [',gateway', 'model', 'Providers', /Provider 1:/],
    ['gateway,gateway', 'model', 'Providers', /Duplicate provider/],
    ['Gateway', 'model', 'Providers', /Provider 1:/],
    ['alias', 'model', 'Providers', /Provider 1:/],
    ['gateway/backup', 'model', 'Providers', /Provider 1:/],
    ['gateway;backup', 'model', 'Providers', /Provider 1:/],
    ['gateway', '/model', 'Model', /Model must/],
    ['gateway', 'org//model', 'Model', /Model must/],
    ['gateway', 'org/model/', 'Model', /Model must/],
    ['gateway', 'Org/model', 'Model', /Model must/],
    ['gateway', 'org/_model', 'Model', /Model must/],
    ['gateway', 'org/ model', 'Model', /Model must/],
    ['gateway', 'org\\model', 'Model', /Model must/],
  ])('rejects malformed shorthand %j / %j before HTTP', async (providers, model, field, message) => {
    mount();
    const form = await create();
    change('Providers', providers);
    change('Model', model);
    fireEvent.click(form.getByRole('button', { name: 'Create alias' }));
    expect(await form.findByText(message)).toBeVisible();
    expect(form.getByLabelText(field)).toHaveValue(field === 'Providers' ? providers : model);
    expect(writes).toEqual([]);
  });

  it('rejects mixed inputs and malformed edits, then saves the corrected target without losing options', async () => {
    mount(alias(), [alias()]);
    const form = await edit();
    change('Providers', 'backup');
    change('Model', 'other');
    fireEvent.click(form.getByRole('button', { name: 'Save changes' }));
    expect(await form.findByText(/Use targets or shorthand/)).toBeVisible();
    expect(writes).toEqual([]);
    change('Providers', '');
    change('Model', '');
    change(/^Targets/, 'gateway/z-ai//glm-5.2');
    fireEvent.click(form.getByRole('button', { name: 'Save changes' }));
    expect(await form.findByText(/Line 1: Model/)).toBeVisible();
    expect(writes).toEqual([]);
    change(/^Targets/, targetText);
    await save();
    expect(writes[0].body).toEqual({ name: 'fast', algorithm: 'round_robin', targets });
  });

  it('retains retry-code form errors and server save errors with full identifiers available for correction', async () => {
    mount(undefined, [], 'saved but activation failed');
    const form = await create();
    change(/^Targets/, targetText);
    change(/^Retry status/, 'oops');
    fireEvent.click(form.getByRole('button', { name: 'Create alias' }));
    expect(await form.findByText('retry status codes must be integers')).toBeVisible();
    expect(writes).toEqual([]);
    change(/^Retry status/, '429, 503');
    fireEvent.click(form.getByRole('button', { name: 'Create alias' }));
    expect(await form.findByText('saved but activation failed')).toBeVisible();
    expect(writes[0].body).toEqual({ name: 'fast', algorithm: 'round_robin', targets, retry_status_codes: [429, 503] });
    expect(form.getByLabelText(/^Targets/)).toHaveValue(targetText);
  });
});
