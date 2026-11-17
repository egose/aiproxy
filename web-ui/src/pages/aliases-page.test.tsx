import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { DialogManagerProvider } from '@egose/shadcn-theme/components/widgets/dialog-manager';
import axios, { AxiosError, type AxiosResponse, type InternalAxiosRequestConfig } from 'axios';
import { MemoryRouter } from 'react-router';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { AliasesPage } from './aliases-page';
import { adminClient } from '../services/admin';
import { clearAdminSession, setAdminSession } from '../store';
import { adminAliasSchema, type AdminAlias, type EncryptedReasoningDeclaration } from '../types';

const originalAdapter = axios.defaults.adapter;
const originalAdminAdapter = adminClient.defaults.adapter;
let client: QueryClient;
let writes: Array<{ method: string | undefined; url: string | undefined; body: unknown }>;
let listReads = 0;
let adminReads: string[];

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
    encrypted_reasoning: null,
    encrypted_reasoning_effective: null,
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
  adminReads = [];
});

afterEach(() => {
  cleanup();
  client.clear();
  axios.defaults.adapter = originalAdapter;
  adminClient.defaults.adapter = originalAdminAdapter;
  clearAdminSession();
});

function mount(initial?: AdminAlias, savedViews: AdminAlias[] = [], saveError?: string, denyList = false) {
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
    adminReads.push(config.url ?? '');
    if (config.url === '/_internal/admin/me') {
      return response(config, { user_id: 'form-user', email: 'form@example.test', is_admin: false });
    }
    if (config.url === '/_internal/admin/aliases') {
      listReads++;
      if (denyList) {
        const denied = { ...response(config, 'forbidden'), status: 403, statusText: 'Forbidden' };
        throw new AxiosError('forbidden', 'ERR_BAD_REQUEST', config, undefined, denied);
      }
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
        encrypted_reasoning_effective: {
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
        {
          method: 'put',
          url: '/_internal/admin/aliases/fast',
          body: {
            name: 'fast',
            targets,
            algorithm,
            retry_status_codes: options.retry_status_codes,
            session_affinity: options.session_affinity,
          },
        },
      ]);
      form = await edit();
      expect(form.getByLabelText(/^Targets/)).toHaveValue(targetText);
      expect(form.getByLabelText('Algorithm')).toHaveValue(algorithm);
      expect(form.getByLabelText(/^Retry status/)).toHaveValue('429,500,503');
      expect(form.getByRole('checkbox', { name: 'Session affinity' })).toBeChecked();
      expect(form.getByLabelText(/^Headers/)).toHaveValue('x-session-id, x-client');
      expect(policy().getByLabelText('Policy mode')).toHaveValue('custom');
      expect(policy().getByLabelText('Passthrough member')).toHaveValue('false');
      expect(policy().getByLabelText('Mismatch member')).toHaveValue('strip_and_retry');
      expect(policy().getByLabelText('Match messages member')).toHaveValue('declare');
      expect(policy().getByLabelText('Match messages (one per line)')).toHaveValue(
        'invalid signature\ncaller mismatch',
      );
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

const safeReasoning = { passthrough: true, on_caller_mismatch: 'fail', match_messages: ['builtin mismatch'] };
const stripReasoning = { ...safeReasoning, passthrough: false };
const customLocal = {
  passthrough: false,
  on_caller_mismatch: 'strip_and_retry',
  match_messages: ['  Caller MISMATCH  ', 'Other Error'],
};
const customEffective = {
  passthrough: false,
  on_caller_mismatch: 'strip_and_retry',
  match_messages: ['caller mismatch', 'other error'],
};
const exactText = { normalizer: (text: string) => text };
const localCases: Array<[EncryptedReasoningDeclaration, typeof safeReasoning]> = [
  [{}, safeReasoning],
  [{ passthrough: false }, stripReasoning],
  [{ on_caller_mismatch: '' }, safeReasoning],
  [{ match_messages: [] }, safeReasoning],
  [{ on_caller_mismatch: '', match_messages: [] }, safeReasoning],
  [safeReasoning, safeReasoning],
  [customLocal, customEffective],
  [{ match_messages: ['  RAW\nPattern  '] }, { ...safeReasoning, match_messages: ['raw\npattern'] }],
];

function policy() {
  return within(screen.getByRole('group', { name: 'Alias encrypted reasoning' }));
}

function configurePolicy(local: EncryptedReasoningDeclaration | null) {
  const group = policy();
  fireEvent.change(group.getByLabelText('Policy mode'), {
    target: { value: local === null ? 'inherit' : Object.keys(local).length === 0 ? 'reset' : 'custom' },
  });
  if (local === null || Object.keys(local).length === 0) return;
  if ('passthrough' in local)
    fireEvent.change(group.getByLabelText('Passthrough member'), { target: { value: String(local.passthrough) } });
  if ('on_caller_mismatch' in local)
    fireEvent.change(group.getByLabelText('Mismatch member'), { target: { value: local.on_caller_mismatch } });
  if ('match_messages' in local) {
    fireEvent.change(group.getByLabelText('Match messages member'), { target: { value: 'declare' } });
    fireEvent.change(group.getByLabelText('Match messages (one per line)'), {
      target: { value: local.match_messages?.join('\n') },
    });
  }
}

describe('mounted alias sparse reasoning intent and saved context', { timeout: 15000 }, () => {
  it.each([[undefined, null], [null, null], ...localCases] as Array<
    [EncryptedReasoningDeclaration | null | undefined, typeof safeReasoning | null]
  >)('preserves untouched local %j by omission on noop and unrelated routing saves', async (local, effective) => {
    const before = alias({ encrypted_reasoning: local, encrypted_reasoning_effective: effective });
    mount(before, [before, alias({ ...before, algorithm: 'least_connections' })]);
    await edit();
    await save();
    expect(writes[0]).toEqual({
      method: 'put',
      url: '/_internal/admin/aliases/fast',
      body: { name: 'fast', algorithm: 'round_robin', targets },
    });
    await edit();
    const mode = local == null ? 'inherit' : Object.keys(local).length === 0 ? 'reset' : 'custom';
    expect(policy().getByLabelText('Policy mode')).toHaveValue(mode);
    expect(
      policy().getByText(`Selected local declaration: ${JSON.stringify(local ?? null)}`, exactText),
    ).toBeInTheDocument();
    change('Algorithm', 'least_connections');
    await save();
    expect(writes[1].body).toEqual({ name: 'fast', algorithm: 'least_connections', targets });
    await edit();
    expect(policy().getByLabelText('Policy mode')).toHaveValue(mode);
    expect(
      policy().getByText(`Selected local declaration: ${JSON.stringify(local ?? null)}`, exactText),
    ).toBeInTheDocument();
  });

  it.each(localCases.slice(0, -1))(
    'creates exact sparse declaration %j and reopens without default filling',
    async (local, effective) => {
      const saved = alias({ encrypted_reasoning: local, encrypted_reasoning_effective: effective });
      mount(undefined, [saved, saved]);
      await create();
      change(/^Targets/, targetText);
      configurePolicy(local);
      await save('Create alias');
      expect(writes[0]).toEqual({
        method: 'post',
        url: '/_internal/admin/aliases',
        body: { name: 'fast', algorithm: 'round_robin', targets, encrypted_reasoning: local },
      });
      await edit();
      expect(policy().getByText(`Selected local declaration: ${JSON.stringify(local)}`, exactText)).toBeInTheDocument();
      expect(policy().getByText('Saved effective policy: explicit alias override')).toBeInTheDocument();
      expect(policy().getByText(JSON.stringify(effective, null, 2), exactText)).toBeInTheDocument();
      expect(policy().queryByText(/source unavailable|inheritance unavailable/)).not.toBeInTheDocument();
      await save();
      expect(writes[1].body).toEqual({ name: 'fast', algorithm: 'round_robin', targets });
    },
  );

  it.each([localCases[0], localCases[1], [customLocal, customEffective]] as typeof localCases)(
    'removes alias override %j with explicit null, never false or an empty reset',
    async (local, effective) => {
      mount(alias({ encrypted_reasoning: local, encrypted_reasoning_effective: effective }), [alias()]);
      await edit();
      configurePolicy(null);
      await save();
      expect(writes[0].body).toEqual({ name: 'fast', algorithm: 'round_robin', targets, encrypted_reasoning: null });
      await edit();
      expect(policy().getByLabelText('Policy mode')).toHaveValue('inherit');
      expect(policy().getByText(/Saved effective policy: target-dependent/)).toBeInTheDocument();
    },
  );

  it('replaces a false custom override with an explicit safe reset, then omits the saved reset unchanged', async () => {
    const reset = alias({ encrypted_reasoning: {}, encrypted_reasoning_effective: safeReasoning });
    mount(alias({ encrypted_reasoning: customLocal, encrypted_reasoning_effective: customEffective }), [reset, reset]);
    await edit();
    configurePolicy({});
    expect(policy().getByText(JSON.stringify(customEffective, null, 2), exactText)).toBeInTheDocument();
    expect(policy().getByText(/saved state, not unsaved edits or proof of runtime activation/)).toBeInTheDocument();
    await save();
    expect(writes[0].body).toEqual({ name: 'fast', algorithm: 'round_robin', targets, encrypted_reasoning: {} });
    await edit();
    expect(policy().getByLabelText('Policy mode')).toHaveValue('reset');
    expect(policy().getByText(JSON.stringify(safeReasoning, null, 2), exactText)).toBeInTheDocument();
    await save();
    expect(writes[1].body).not.toHaveProperty('encrypted_reasoning');
  });

  it('edits one member without rewriting false, explicit empty members, or raw multiline pattern spelling', async () => {
    const before = { passthrough: false, on_caller_mismatch: '', match_messages: ['  RAW\nPattern  '] };
    const changed = { ...before, on_caller_mismatch: 'strip_and_retry' };
    const withoutPassthrough = { on_caller_mismatch: 'strip_and_retry', match_messages: before.match_messages };
    mount(
      alias({
        encrypted_reasoning: before,
        encrypted_reasoning_effective: { ...stripReasoning, match_messages: ['raw\npattern'] },
      }),
      [
        alias({
          encrypted_reasoning: changed,
          encrypted_reasoning_effective: {
            ...stripReasoning,
            on_caller_mismatch: 'strip_and_retry',
            match_messages: ['raw\npattern'],
          },
        }),
        alias({
          encrypted_reasoning: withoutPassthrough,
          encrypted_reasoning_effective: {
            ...safeReasoning,
            on_caller_mismatch: 'strip_and_retry',
            match_messages: ['raw\npattern'],
          },
        }),
        alias({
          encrypted_reasoning: { on_caller_mismatch: '', match_messages: [] },
          encrypted_reasoning_effective: safeReasoning,
        }),
      ],
    );
    await edit();
    fireEvent.change(policy().getByLabelText('Mismatch member'), { target: { value: 'strip_and_retry' } });
    await save();
    expect(writes[0].body).toEqual({ name: 'fast', algorithm: 'round_robin', targets, encrypted_reasoning: changed });
    await edit();
    expect(policy().getByLabelText('Match messages (one per line)')).toHaveValue('  RAW\nPattern  ');
    fireEvent.change(policy().getByLabelText('Passthrough member'), { target: { value: 'omit' } });
    await save();
    expect(writes[1].body).toEqual({
      name: 'fast',
      algorithm: 'round_robin',
      targets,
      encrypted_reasoning: withoutPassthrough,
    });
    await edit();
    fireEvent.change(policy().getByLabelText('Mismatch member'), { target: { value: '' } });
    fireEvent.change(policy().getByLabelText('Match messages (one per line)'), { target: { value: '' } });
    await save();
    expect(writes[2].body).toEqual({
      name: 'fast',
      algorithm: 'round_robin',
      targets,
      encrypted_reasoning: { on_caller_mismatch: '', match_messages: [] },
    });
    await edit();
    expect(policy().getByLabelText('Passthrough member')).toHaveValue('omit');
    expect(policy().getByLabelText('Mismatch member')).toHaveValue('');
    expect(policy().getByLabelText('Match messages member')).toHaveValue('declare');
    expect(policy().getByLabelText('Match messages (one per line)')).toHaveValue('');
  });

  it('adds only declared custom members while keeping two model targets target-dependent in saved context', async () => {
    const twoModels = [
      { provider: 'gateway', model: 'z-ai' },
      { provider: 'gateway', model: 'z-ai/glm-5.2' },
    ];
    mount(alias({ targets: twoModels }), [
      alias({
        targets: twoModels,
        encrypted_reasoning: { on_caller_mismatch: 'strip_and_retry' },
        encrypted_reasoning_effective: { ...safeReasoning, on_caller_mismatch: 'strip_and_retry' },
      }),
    ]);
    const form = await edit();
    expect(form.getByLabelText(/^Targets/)).toHaveValue('gateway/z-ai\ngateway/z-ai/glm-5.2');
    expect(policy().getByText(/Each selected target resolves its own model → provider → root/)).toBeInTheDocument();
    expect(policy().queryByText(/unavailable|source builtin|source root/)).not.toBeInTheDocument();
    expect(screen.getByRole('group', { name: 'Alias encrypted reasoning' }).querySelector('pre')).toBeNull();
    expect(form.getByText(/at most one extra attempt across the entire request/)).toBeInTheDocument();
    expect(form.getByText(/keeps a stripped body through normal failover/)).toBeInTheDocument();
    configurePolicy({ on_caller_mismatch: 'strip_and_retry' });
    expect(policy().getByText(/Saved effective policy: target-dependent/)).toBeInTheDocument();
    await save();
    expect(writes[0].body).toEqual({
      name: 'fast',
      algorithm: 'round_robin',
      targets: twoModels,
      encrypted_reasoning: { on_caller_mismatch: 'strip_and_retry' },
    });
    expect(adminReads.every((url) => ['/_internal/admin/me', '/_internal/admin/aliases'].includes(url))).toBe(true);
    await edit();
    expect(policy().getByText('Saved effective policy: explicit alias override')).toBeInTheDocument();
    expect(policy().queryByText(/target-dependent/)).not.toBeInTheDocument();
  });

  it.each([
    ['duplicate', 'a pattern\n A PATTERN ', /unique after trimming and lowercasing/],
    ['Go whitespace duplicate', '\u0085ABCD\nabcd', /unique after trimming and lowercasing/],
    ['interior blank', 'first message\n\nlast message', /blank lines are invalid/],
    ['trailing blank', 'first message\n', /blank lines are invalid/],
    ['blank', ' \u3000 ', /4–256 UTF-8 bytes/],
    ['short', 'abc', /4–256 UTF-8 bytes/],
    ['long', 'a'.repeat(257), /4–256 UTF-8 bytes/],
    ['UTF-8 bytes', '😀'.repeat(65), /4–256 UTF-8 bytes/],
    ['count', Array.from({ length: 17 }, (_, i) => `pattern ${i}`).join('\n'), /at most 16/],
  ] as const)('rejects %s patterns with a visible policy field error before HTTP', async (_, messages, error) => {
    mount(alias());
    const form = await edit();
    configurePolicy({ match_messages: ['valid message'] });
    fireEvent.change(policy().getByLabelText('Match messages (one per line)'), { target: { value: messages } });
    fireEvent.click(form.getByRole('button', { name: 'Save changes' }));
    expect(await policy().findByRole('alert')).toHaveTextContent(error);
    expect(policy().getByLabelText('Match messages (one per line)')).toHaveAttribute('aria-invalid', 'true');
    expect(writes).toEqual([]);
  });

  it('rejects an invalid mismatch member on create and permits correction without materializing other members', async () => {
    mount(undefined, [
      alias({
        encrypted_reasoning: { on_caller_mismatch: 'fail' },
        encrypted_reasoning_effective: safeReasoning,
      }),
    ]);
    const form = await create();
    change(/^Targets/, targetText);
    configurePolicy({ on_caller_mismatch: '' });
    const select = policy().getByLabelText('Mismatch member');
    const invalid = document.createElement('option');
    invalid.value = 'retry';
    select.appendChild(invalid);
    fireEvent.change(select, { target: { value: 'retry' } });
    fireEvent.click(form.getByRole('button', { name: 'Create alias' }));
    expect(await policy().findByRole('alert')).toHaveTextContent(
      'Mismatch handling must be fail, strip_and_retry, or explicitly empty.',
    );
    expect(writes).toEqual([]);
    fireEvent.change(select, { target: { value: 'fail' } });
    await save('Create alias');
    expect(writes[0].body).toEqual({
      name: 'fast',
      algorithm: 'round_robin',
      targets,
      encrypted_reasoning: { on_caller_mismatch: 'fail' },
    });
  });

  it('rejects blank patterns on create, then deliberately saves an empty list rather than omitting it', async () => {
    mount(undefined, [
      alias({ encrypted_reasoning: { match_messages: [] }, encrypted_reasoning_effective: safeReasoning }),
    ]);
    const form = await create();
    change(/^Targets/, targetText);
    configurePolicy({ match_messages: ['valid message', '', 'another message'] });
    fireEvent.click(form.getByRole('button', { name: 'Create alias' }));
    expect(await policy().findByRole('alert')).toHaveTextContent('blank lines are invalid');
    expect(writes).toEqual([]);
    fireEvent.change(policy().getByLabelText('Match messages (one per line)'), { target: { value: '' } });
    await save('Create alias');
    expect(writes[0].body).toEqual({
      name: 'fast',
      algorithm: 'round_robin',
      targets,
      encrypted_reasoning: { match_messages: [] },
    });
    await edit();
    expect(policy().getByLabelText('Policy mode')).toHaveValue('custom');
    expect(policy().getByLabelText('Match messages member')).toHaveValue('declare');
    expect(policy().getByLabelText('Match messages (one per line)')).toHaveValue('');
  });

  it('accepts normalized UTF-8 byte and count boundaries without normalizing the local write', async () => {
    const raw = [' 😀 ', '😀'.repeat(64), ...Array.from({ length: 14 }, (_, i) => `PATTERN ${i}`)];
    const normalized = ['😀', '😀'.repeat(64), ...Array.from({ length: 14 }, (_, i) => `pattern ${i}`)];
    mount(undefined, [
      alias({
        encrypted_reasoning: { match_messages: raw },
        encrypted_reasoning_effective: { ...safeReasoning, match_messages: normalized },
      }),
    ]);
    await create();
    change(/^Targets/, targetText);
    configurePolicy({ match_messages: raw });
    await save('Create alias');
    expect(writes[0].body).toEqual({
      name: 'fast',
      algorithm: 'round_robin',
      targets,
      encrypted_reasoning: { match_messages: raw },
    });
    await edit();
    expect(policy().getByLabelText('Match messages (one per line)')).toHaveValue(raw.join('\n'));
    expect(
      policy().getByText(JSON.stringify({ ...safeReasoning, match_messages: normalized }, null, 2), exactText),
    ).toBeInTheDocument();
  });

  it.each([
    [null, null],
    [{}, safeReasoning],
    [customLocal, customEffective],
  ])('shows static alias intent %j and context read-only without edit actions', async (local, effective) => {
    mount(alias({ source: 'config', encrypted_reasoning: local, encrypted_reasoning_effective: effective }));
    fireEvent.click(await screen.findByText('Policy (read-only)'));
    const context = within(screen.getByRole('region', { name: 'Alias policy' }));
    expect(
      context.getByText(`Local declaration: ${local == null ? 'none (inherit)' : JSON.stringify(local)}`, exactText),
    ).toBeInTheDocument();
    expect(context.getByText(local == null ? /target-dependent/ : /explicit alias override/)).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Actions for fast' })).not.toBeInTheDocument();
    expect(screen.queryByLabelText('Policy mode')).not.toBeInTheDocument();
    expect(writes).toEqual([]);
  });

  it('does not reveal alias intent or add context lookups when the alias list read is forbidden', async () => {
    mount(
      alias({ encrypted_reasoning: customLocal, encrypted_reasoning_effective: customEffective }),
      [],
      undefined,
      true,
    );
    expect(await screen.findByText('No aliases.')).toBeInTheDocument();
    await waitFor(() => expect(listReads).toBe(1));
    expect(screen.queryByRole('button', { name: 'Actions for fast' })).not.toBeInTheDocument();
    expect(screen.queryByText('Policy (read-only)')).not.toBeInTheDocument();
    expect(screen.queryByLabelText('Policy mode')).not.toBeInTheDocument();
    expect(adminReads).toContain('/_internal/admin/aliases');
    expect(adminReads.every((url) => ['/_internal/admin/me', '/_internal/admin/aliases'].includes(url))).toBe(true);
    expect(writes).toEqual([]);
  });

  it('retains unsaved intent and saved-only context when activation fails instead of claiming it applied', async () => {
    mount(
      alias({ encrypted_reasoning: customLocal, encrypted_reasoning_effective: customEffective }),
      [],
      'saved but activation failed',
    );
    const form = await edit();
    const reads = listReads;
    configurePolicy({});
    fireEvent.click(form.getByRole('button', { name: 'Save changes' }));
    expect(await form.findByText('saved but activation failed')).toBeVisible();
    expect(writes[0].body).toEqual({ name: 'fast', algorithm: 'round_robin', targets, encrypted_reasoning: {} });
    expect(policy().getByLabelText('Policy mode')).toHaveValue('reset');
    expect(policy().getByText(JSON.stringify(customEffective, null, 2), exactText)).toBeInTheDocument();
    expect(policy().getByText(/not unsaved edits or proof of runtime activation/)).toBeInTheDocument();
    expect(listReads).toBe(reads);
  });
});
