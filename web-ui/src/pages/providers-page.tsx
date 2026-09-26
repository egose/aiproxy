import { zodResolver } from '@hookform/resolvers/zod';
import { HookFormCheckbox } from '@egose/shadcn-theme/components/form/hook-checkbox';
import { HookFormNativeSelect } from '@egose/shadcn-theme/components/form/hook-native-select';
import { HookFormTextInput } from '@egose/shadcn-theme/components/form/hook-text-input';
import { Alert, AlertDescription } from '@egose/shadcn-theme/components/ui/alert';
import { Button } from '@egose/shadcn-theme/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@egose/shadcn-theme/components/ui/card';
import { Checkbox } from '@egose/shadcn-theme/components/ui/checkbox';
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@egose/shadcn-theme/components/ui/dialog';
import { Input } from '@egose/shadcn-theme/components/ui/input';
import { Separator } from '@egose/shadcn-theme/components/ui/separator';
import { Spinner } from '@egose/shadcn-theme/components/ui/spinner';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@egose/shadcn-theme/components/ui/table';
import { createTypedDialog, useDialog } from '@egose/shadcn-theme/components/widgets/dialog-manager';
import { ActionMenu } from '@egose/shadcn-theme/components/widgets/action-menu';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useMemo, useState } from 'react';
import { FormProvider, useFieldArray, useForm } from 'react-hook-form';
import { Link } from 'react-router';
import { z } from 'zod';
import { DataTable, type ManagementColumnDef } from '../components/data-table';
import { useConfirm } from '../components/dialogs';
import { SourceBadge } from '../components/source-badge';
import { OrgBadge } from '../components/org-select';
import { useAdminProviders, useAdminSession, useAdminStatus, useProviderTypes, useSnapshot } from '../hooks';
import {
  createAdminProvider,
  deleteAdminProvider,
  setProviderCredential,
  updateAdminProvider,
} from '../services/admin';
import { errorMessage } from '../services/dashboard';
import type { AdminProvider, Provider, ProviderTypeInfo } from '../types';

type SnapshotSortKey = 'name' | 'type' | 'models' | 'display';
type SnapshotSort = { key: SnapshotSortKey; desc: boolean } | null;

function SnapshotProviders() {
  const snapshot = useSnapshot();
  const [sorting, setSorting] = useState<SnapshotSort>(null);
  const [filter, setFilter] = useState('');

  const rows = useMemo(() => {
    const q = filter.trim().toLowerCase();
    const all = snapshot.data?.providers ?? [];
    const filtered =
      q === ''
        ? all
        : all.filter(
            (p) =>
              p.name.toLowerCase().includes(q) ||
              p.type.toLowerCase().includes(q) ||
              (p.display_name ?? '').toLowerCase().includes(q),
          );
    if (!sorting) return filtered;
    const dir = sorting.desc ? -1 : 1;
    const value = (p: Provider): string | number => {
      switch (sorting.key) {
        case 'type':
          return p.type;
        case 'models':
          return p.models.length;
        case 'display':
          return p.display_name ?? '';
        default:
          return p.name;
      }
    };
    return [...filtered].sort((a, b) => {
      const va = value(a);
      const vb = value(b);
      if (va < vb) return -dir;
      if (va > vb) return dir;
      return 0;
    });
  }, [snapshot.data, filter, sorting]);

  const toggleSort = (key: SnapshotSortKey) => {
    setSorting((prev) => {
      if (prev?.key !== key) return { key, desc: false };
      if (!prev.desc) return { key, desc: true };
      return null;
    });
  };

  const headers: Array<{ key: SnapshotSortKey; label: string }> = [
    { key: 'name', label: 'Provider' },
    { key: 'type', label: 'Type' },
    { key: 'models', label: 'Models' },
    { key: 'display', label: 'Display name' },
  ];

  return (
    <Card>
      <CardHeader>
        <CardTitle>Providers ({rows.length})</CardTitle>
      </CardHeader>
      <CardContent className="grid gap-4 pt-0">
        <Input placeholder="Filter providers..." value={filter} onChange={(e) => setFilter(e.target.value)} />
        <div className="overflow-x-auto">
          <Table>
            <TableHeader>
              <TableRow>
                {headers.map((h) => (
                  <TableHead key={h.key} className="cursor-pointer" onClick={() => toggleSort(h.key)}>
                    {h.label}
                    {sorting?.key === h.key ? (sorting.desc ? ' ▼' : ' ▲') : ''}
                  </TableHead>
                ))}
              </TableRow>
            </TableHeader>
            <TableBody>
              {rows.map((p) => (
                <TableRow key={p.name}>
                  <TableCell className="font-mono">{p.name}</TableCell>
                  <TableCell className="font-mono">{p.type}</TableCell>
                  <TableCell className="font-mono">{p.models.length}</TableCell>
                  <TableCell className="font-mono">{p.display_name ?? ''}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      </CardContent>
    </Card>
  );
}

const modelRowSchema = z.object({
  name: z.string().trim().min(1, 'Enter a model name.'),
  upstreamName: z.string().optional(),
  displayName: z.string().optional(),
  protocol: z.string().optional(),
  capabilities: z.array(z.string()).optional(),
  pricingInput: z.string().optional(),
  pricingOutput: z.string().optional(),
  pricingCached: z.string().optional(),
  pricingCacheWrite: z.string().optional(),
});

const providerFormSchema = z.object({
  name: z.string().trim().min(1, 'Enter a provider name.'),
  type: z.string().min(1, 'Select a type.'),
  displayName: z.string().optional(),
  baseUrl: z.string().optional(),
  enabled: z.boolean().optional(),
  extendsName: z.string().optional(),
  userAgent: z.string().optional(),
  timeout: z.string().optional(),
  forwardUserAgent: z.boolean().optional(),
  forwardHeaders: z.string().optional(),
  apiKey: z.string().optional(),
  apiKeyRefPath: z.string().optional(),
  apiKeyRefKey: z.string().optional(),
  credentialPath: z.string().optional(),
  credentialName: z.string().optional(),
  models: z.array(modelRowSchema).min(1, 'Add at least one model.'),
  showHealthcheck: z.boolean().optional(),
  hcPath: z.string().optional(),
  hcMethod: z.string().optional(),
  hcStatus: z.string().optional(),
  hcBody: z.string().optional(),
  hcInterval: z.string().optional(),
  hcTimeout: z.string().optional(),
  hcFailures: z.string().optional(),
  hcSuccesses: z.string().optional(),
  hcSendAuth: z.boolean().optional(),
});

type ProviderFormValues = z.infer<typeof providerFormSchema>;

const emptyModelRow = (): ProviderFormValues['models'][number] => ({
  name: '',
  upstreamName: '',
  displayName: '',
  protocol: '',
  capabilities: [],
  pricingInput: '',
  pricingOutput: '',
  pricingCached: '',
  pricingCacheWrite: '',
});

const emptyProviderForm = (): ProviderFormValues => ({
  name: '',
  type: 'openai',
  displayName: '',
  baseUrl: '',
  enabled: true,
  extendsName: '',
  userAgent: '',
  timeout: '',
  forwardUserAgent: false,
  forwardHeaders: '',
  apiKey: '',
  apiKeyRefPath: '',
  apiKeyRefKey: '',
  credentialPath: '',
  credentialName: '',
  models: [emptyModelRow()],
  showHealthcheck: false,
  hcPath: '',
  hcMethod: 'GET',
  hcStatus: '',
  hcBody: '',
  hcInterval: '',
  hcTimeout: '',
  hcFailures: '',
  hcSuccesses: '',
  hcSendAuth: false,
});

function providerToFormValues(p: AdminProvider): ProviderFormValues {
  return {
    name: p.name,
    type: p.type,
    displayName: p.display_name ?? '',
    baseUrl: p.base_url ?? '',
    enabled: p.enabled,
    extendsName: p.extends ?? '',
    userAgent: p.user_agent ?? '',
    timeout: p.upstream_header_timeout ?? '',
    forwardUserAgent: p.forward_user_agent ?? false,
    forwardHeaders: (p.forward_headers ?? []).join(', '),
    apiKey: '',
    apiKeyRefPath: p.api_key_ref_path ?? '',
    apiKeyRefKey: p.api_key_ref_key ?? '',
    credentialPath: p.copilot_credential_path ?? '',
    credentialName: p.copilot_credential_name ?? '',
    models:
      p.models.length > 0
        ? p.models.map((m) => ({
            name: m.name,
            upstreamName: m.upstream_name && m.upstream_name !== m.name ? m.upstream_name : '',
            displayName: m.display_name ?? '',
            protocol: m.protocol ?? '',
            capabilities: [...(m.capabilities ?? [])],
            pricingInput: m.pricing?.input_per_million?.toString() ?? '',
            pricingOutput: m.pricing?.output_per_million?.toString() ?? '',
            pricingCached: m.pricing?.cached_per_million?.toString() ?? '',
            pricingCacheWrite: m.pricing?.cache_write_per_million?.toString() ?? '',
          }))
        : [emptyModelRow()],
    showHealthcheck: p.healthcheck != null,
    hcPath: p.healthcheck?.path ?? '',
    hcMethod: p.healthcheck?.method || 'GET',
    hcStatus: p.healthcheck?.expected_status ? String(p.healthcheck.expected_status) : '',
    hcBody: p.healthcheck?.expected_body ?? '',
    hcInterval: p.healthcheck?.interval ?? '',
    hcTimeout: p.healthcheck?.timeout ?? '',
    hcFailures: p.healthcheck?.failure_threshold ? String(p.healthcheck.failure_threshold) : '',
    hcSuccesses: p.healthcheck?.success_threshold ? String(p.healthcheck.success_threshold) : '',
    hcSendAuth: p.healthcheck?.send_authorization ?? false,
  };
}

function modelRowPricing(row: ProviderFormValues['models'][number]): Record<string, number> | undefined {
  const rates: Record<string, number> = {};
  const pairs: Array<[string, string]> = [
    ['input_per_million', row.pricingInput ?? ''],
    ['output_per_million', row.pricingOutput ?? ''],
    ['cached_per_million', row.pricingCached ?? ''],
    ['cache_write_per_million', row.pricingCacheWrite ?? ''],
  ];
  for (const [key, raw] of pairs) {
    const trimmed = raw.trim();
    if (trimmed === '') continue;
    const value = Number(trimmed);
    if (!Number.isFinite(value)) throw new Error(`pricing rate ${key} must be a number`);
    rates[key] = value;
  }
  return Object.keys(rates).length > 0 ? rates : undefined;
}

function buildProviderBody(v: ProviderFormValues, credentialKind: string): Record<string, unknown> {
  const modelPayloads = v.models
    .map((row) => {
      const trimmed = row.name.trim();
      if (trimmed === '') return null;
      const payload: Record<string, unknown> = { name: trimmed };
      if ((row.displayName ?? '').trim() !== '') payload.display_name = (row.displayName ?? '').trim();
      if ((row.upstreamName ?? '').trim() !== '') payload.upstream_name = (row.upstreamName ?? '').trim();
      if ((row.protocol ?? '') !== '') payload.protocol = row.protocol;
      if ((row.capabilities ?? []).length > 0) payload.capabilities = row.capabilities;
      const pricing = modelRowPricing(row);
      if (pricing !== undefined) payload.pricing = pricing;
      return payload;
    })
    .filter((m): m is Record<string, unknown> => m !== null);
  const body: Record<string, unknown> = {
    name: v.name.trim(),
    type: v.type,
    models: modelPayloads,
  };
  if ((v.displayName ?? '').trim() !== '') body.display_name = (v.displayName ?? '').trim();
  if ((v.baseUrl ?? '').trim() !== '') body.base_url = (v.baseUrl ?? '').trim();
  if ((v.timeout ?? '').trim() !== '') body.upstream_header_timeout = (v.timeout ?? '').trim();
  if ((v.userAgent ?? '') !== '') body.user_agent = v.userAgent;
  if (v.forwardUserAgent) body.forward_user_agent = true;
  const headers = (v.forwardHeaders ?? '')
    .split(',')
    .map((h) => h.trim())
    .filter(Boolean);
  if (headers.length > 0) body.forward_headers = headers;
  if (!v.enabled) body.enabled = false;
  if ((v.extendsName ?? '').trim() !== '') body.extends = (v.extendsName ?? '').trim();
  if (credentialKind === 'credential_ref') {
    if ((v.credentialPath ?? '').trim() !== '')
      body.credential_ref = { path: (v.credentialPath ?? '').trim(), name: (v.credentialName ?? '').trim() };
    else body.credential_ref = { name: (v.credentialName ?? '').trim() };
  } else {
    if ((v.apiKey ?? '') !== '') body.api_key = v.apiKey;
    if ((v.apiKeyRefKey ?? '').trim() !== '') {
      body.api_key_ref = { path: (v.apiKeyRefPath ?? '').trim(), key: (v.apiKeyRefKey ?? '').trim() };
    }
  }
  if (v.showHealthcheck && (v.hcPath ?? '').trim() !== '') {
    const hc: Record<string, unknown> = { path: (v.hcPath ?? '').trim(), method: v.hcMethod };
    const status = (v.hcStatus ?? '').trim();
    if (status !== '') hc.expected_status = Number(status);
    if ((v.hcBody ?? '') !== '') hc.expected_body = v.hcBody;
    if ((v.hcInterval ?? '').trim() !== '') hc.interval = (v.hcInterval ?? '').trim();
    if ((v.hcTimeout ?? '').trim() !== '') hc.timeout = (v.hcTimeout ?? '').trim();
    const failures = (v.hcFailures ?? '').trim();
    if (failures !== '') hc.failure_threshold = Number(failures);
    const successes = (v.hcSuccesses ?? '').trim();
    if (successes !== '') hc.success_threshold = Number(successes);
    if (v.hcSendAuth) hc.send_authorization = true;
    body.healthcheck = hc;
  }
  return body;
}

function ProviderForm({
  initial,
  submitLabel,
  onSubmit,
  onCancel,
  lockName,
  providerTypeInfos,
}: {
  initial: ProviderFormValues;
  submitLabel: string;
  onSubmit: (body: Record<string, unknown>) => Promise<void>;
  onCancel?: () => void;
  lockName: boolean;
  providerTypeInfos: ProviderTypeInfo[] | undefined;
}) {
  const form = useForm<ProviderFormValues>({ resolver: zodResolver(providerFormSchema), defaultValues: initial });
  const { fields, append, remove } = useFieldArray({ control: form.control, name: 'models' });
  const submitMutation = useMutation({
    mutationFn: async (values: ProviderFormValues) => {
      const typeInfo = providerTypeInfos?.find((t) => t.type === values.type);
      return onSubmit(buildProviderBody(values, typeInfo?.credential ?? 'api_key'));
    },
  });

  const selectedType = form.watch('type');
  const typeInfo = providerTypeInfos?.find((t) => t.type === selectedType);
  const credentialKind = typeInfo?.credential ?? 'api_key';
  const protocolRequired = typeInfo?.model_protocol_required ?? false;
  const supportedCaps = typeInfo?.supported_capabilities ?? [];

  const toggleCapability = (index: number, capability: string) => {
    const current = form.getValues(`models.${index}.capabilities`) ?? [];
    form.setValue(
      `models.${index}.capabilities`,
      current.includes(capability) ? current.filter((c) => c !== capability) : [...current, capability],
      { shouldValidate: true },
    );
  };

  return (
    <FormProvider {...form}>
      <form className="grid gap-4" onSubmit={form.handleSubmit((values) => submitMutation.mutate(values))}>
        {submitMutation.error && (
          <Alert variant="danger">
            <AlertDescription>{errorMessage(submitMutation.error)}</AlertDescription>
          </Alert>
        )}
        <HookFormTextInput<ProviderFormValues>
          name="name"
          label="Name (lowercase, no spaces)"
          placeholder="db-openai"
          disabled={lockName}
        />
        <HookFormNativeSelect<ProviderFormValues>
          name="type"
          label="Type"
          data={(providerTypeInfos ?? [{ type: 'openai' }]).map((t) => ({ label: t.type, value: t.type }))}
        />
        <HookFormTextInput<ProviderFormValues> name="displayName" label="Display name (optional)" />
        <HookFormTextInput<ProviderFormValues>
          name="baseUrl"
          label={`Base URL${typeInfo?.requires_base_url ? ' (required for this type)' : ' (optional)'}`}
          placeholder="https://..."
        />
        <HookFormCheckbox<ProviderFormValues> name="enabled" label="Enabled" />
        <Separator />
        {credentialKind === 'credential_ref' ? (
          <>
            <HookFormTextInput<ProviderFormValues>
              name="credentialName"
              label="Credential name (required, from `aiproxy login`)"
              placeholder="copilot-main"
            />
            <HookFormTextInput<ProviderFormValues> name="credentialPath" label="Credential path (optional)" />
          </>
        ) : (
          <>
            <HookFormTextInput<ProviderFormValues>
              name="apiKey"
              label="API key (stored encrypted, write-only)"
              type="password"
              autoComplete="off"
              placeholder="sk-..."
            />
            <HookFormTextInput<ProviderFormValues>
              name="apiKeyRefKey"
              label="API key reference key (alternative to API key)"
              placeholder="key name in the secrets file"
            />
            <HookFormTextInput<ProviderFormValues> name="apiKeyRefPath" label="API key reference path (optional)" />
          </>
        )}
        <Separator />
        <div className="grid gap-2">
          <span className="text-sm font-medium">Models (at least one required)</span>
          {form.formState.errors.models?.message && (
            <span className="text-sm text-red-500">{form.formState.errors.models.message}</span>
          )}
          {fields.map((field, index) => (
            <div key={field.id} className="grid gap-2 rounded-md border p-3">
              <div className="grid grid-cols-2 gap-2">
                <HookFormTextInput<ProviderFormValues>
                  name={`models.${index}.name`}
                  label="Name"
                  placeholder="gpt-4o-mini"
                />
                <HookFormTextInput<ProviderFormValues>
                  name={`models.${index}.upstreamName`}
                  label="Upstream name (defaults to name)"
                />
              </div>
              <HookFormTextInput<ProviderFormValues>
                name={`models.${index}.displayName`}
                label="Display name (optional)"
              />
              {protocolRequired && (
                <HookFormNativeSelect<ProviderFormValues>
                  name={`models.${index}.protocol`}
                  label="Protocol (required for this type)"
                  data={[
                    { label: 'Select protocol...', value: '' },
                    ...(typeInfo?.protocols ?? []).map((p) => ({ label: p, value: p })),
                  ]}
                />
              )}
              {supportedCaps.length > 0 && (
                <div className="grid gap-1">
                  <span className="text-sm font-medium">Capabilities (empty = type defaults)</span>
                  <div className="flex flex-wrap gap-3">
                    {supportedCaps.map((cap) => (
                      <label key={cap} className="flex items-center gap-1 text-sm">
                        <Checkbox
                          checked={(form.watch(`models.${index}.capabilities`) ?? []).includes(cap)}
                          onCheckedChange={() => toggleCapability(index, cap)}
                        />
                        <span className="font-mono">{cap}</span>
                      </label>
                    ))}
                  </div>
                </div>
              )}
              <div className="grid gap-1">
                <span className="text-sm font-medium">Pricing per million tokens (optional)</span>
                <div className="grid grid-cols-2 gap-2">
                  <HookFormTextInput<ProviderFormValues>
                    name={`models.${index}.pricingInput`}
                    label="Input"
                    inputMode="decimal"
                  />
                  <HookFormTextInput<ProviderFormValues>
                    name={`models.${index}.pricingOutput`}
                    label="Output"
                    inputMode="decimal"
                  />
                  <HookFormTextInput<ProviderFormValues>
                    name={`models.${index}.pricingCached`}
                    label="Cached"
                    inputMode="decimal"
                  />
                  <HookFormTextInput<ProviderFormValues>
                    name={`models.${index}.pricingCacheWrite`}
                    label="Cache write"
                    inputMode="decimal"
                  />
                </div>
              </div>
              {fields.length > 1 && (
                <div>
                  <Button
                    appearance="outline"
                    type="button"
                    disabled={submitMutation.isPending}
                    onClick={() => remove(index)}
                  >
                    Remove model
                  </Button>
                </div>
              )}
            </div>
          ))}
          <div>
            <Button
              appearance="outline"
              type="button"
              disabled={submitMutation.isPending}
              onClick={() => append(emptyModelRow())}
            >
              Add model
            </Button>
          </div>
        </div>
        <Separator />
        <HookFormTextInput<ProviderFormValues>
          name="extendsName"
          label="Extends (optional, static base provider of the same type)"
          placeholder="base provider name"
        />
        <HookFormTextInput<ProviderFormValues> name="userAgent" label="User agent override (optional)" />
        <HookFormTextInput<ProviderFormValues>
          name="timeout"
          label="Upstream header timeout (optional, e.g. 30s)"
          placeholder="30s"
        />
        <HookFormCheckbox<ProviderFormValues> name="forwardUserAgent" label="Forward caller user agent" />
        <HookFormTextInput<ProviderFormValues>
          name="forwardHeaders"
          label="Forward headers (optional, comma separated)"
          placeholder="x-custom-header"
        />
        {typeInfo?.supports_healthcheck !== false && (
          <>
            <HookFormCheckbox<ProviderFormValues> name="showHealthcheck" label="Custom healthcheck" />
            <HookFormTextInput<ProviderFormValues> name="hcPath" label="Path (required)" placeholder="/healthz" />
            <div className="grid grid-cols-2 gap-2">
              <HookFormNativeSelect<ProviderFormValues>
                name="hcMethod"
                label="Method"
                data={[
                  { label: 'GET', value: 'GET' },
                  { label: 'HEAD', value: 'HEAD' },
                ]}
              />
              <HookFormTextInput<ProviderFormValues>
                name="hcStatus"
                label="Expected status"
                placeholder="200"
                inputMode="numeric"
              />
              <HookFormTextInput<ProviderFormValues> name="hcInterval" label="Interval" placeholder="30s" />
              <HookFormTextInput<ProviderFormValues> name="hcTimeout" label="Timeout" placeholder="5s" />
              <HookFormTextInput<ProviderFormValues>
                name="hcFailures"
                label="Failure threshold"
                placeholder="2"
                inputMode="numeric"
              />
              <HookFormTextInput<ProviderFormValues>
                name="hcSuccesses"
                label="Success threshold"
                placeholder="1"
                inputMode="numeric"
              />
            </div>
            <HookFormTextInput<ProviderFormValues>
              name="hcBody"
              label="Expected body (optional, * matches anything)"
              placeholder="*"
            />
            <HookFormCheckbox<ProviderFormValues> name="hcSendAuth" label="Send authorization header" />
          </>
        )}
        <div className="flex gap-2">
          <Button variant="primary" type="submit" disabled={submitMutation.isPending}>
            {submitMutation.isPending ? 'Saving...' : submitLabel}
          </Button>
          {onCancel && (
            <Button appearance="outline" type="button" disabled={submitMutation.isPending} onClick={onCancel}>
              Cancel
            </Button>
          )}
        </div>
      </form>
    </FormProvider>
  );
}

const EditProviderDialog = createTypedDialog<
  { provider: AdminProvider; typeInfos: ProviderTypeInfo[] | undefined },
  boolean
>(({ open, args, onClose }) => (
  <Dialog open={open} onOpenChange={(o) => !o && onClose(false)}>
    <DialogContent className="max-h-[calc(100dvh-2rem)] overflow-auto sm:max-w-3xl">
      <DialogHeader>
        <DialogTitle>Edit provider {args.provider.name}</DialogTitle>
      </DialogHeader>
      <ProviderForm
        key={`edit-${args.provider.name}`}
        initial={providerToFormValues(args.provider)}
        submitLabel="Save changes"
        lockName
        providerTypeInfos={args.typeInfos}
        onSubmit={async (body) => {
          await updateAdminProvider(args.provider.name, body);
          onClose(true);
        }}
        onCancel={() => onClose(false)}
      />
    </DialogContent>
  </Dialog>
));

const CreateProviderDialog = createTypedDialog<{ typeInfos: ProviderTypeInfo[] | undefined }, boolean>(
  ({ open, args, onClose }) => (
    <Dialog open={open} onOpenChange={(o) => !o && onClose(false)}>
      <DialogContent className="max-h-[calc(100dvh-2rem)] overflow-auto sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle>Create provider</DialogTitle>
        </DialogHeader>
        <ProviderForm
          initial={emptyProviderForm()}
          submitLabel="Create provider"
          lockName={false}
          providerTypeInfos={args.typeInfos}
          onSubmit={async (body) => {
            await createAdminProvider(body);
            onClose(true);
          }}
          onCancel={() => onClose(false)}
        />
      </DialogContent>
    </Dialog>
  ),
);

const credentialSchema = z.object({
  name: z.string().trim().min(1, 'Enter a database provider name.'),
  api_key: z.string().min(1, 'Enter the new API key.'),
});

type CredentialForm = z.infer<typeof credentialSchema>;

const RotateCredentialDialog = createTypedDialog<{ name?: string }, { name: string; api_key: string } | null>(
  ({ open, args, onClose }) => {
    const form = useForm<CredentialForm>({
      resolver: zodResolver(credentialSchema),
      defaultValues: { name: args.name ?? '', api_key: '' },
    });
    return (
      <Dialog open={open} onOpenChange={(o) => !o && onClose(null)}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>Rotate upstream credential{args.name ? ` for ${args.name}` : ''}</DialogTitle>
          </DialogHeader>
          <FormProvider {...form}>
            <form className="grid gap-4" onSubmit={form.handleSubmit((values) => onClose(values))}>
              <HookFormTextInput<CredentialForm> name="name" label="Database provider name" disabled={!!args.name} />
              <HookFormTextInput<CredentialForm>
                name="api_key"
                label="New API key"
                type="password"
                autoComplete="off"
              />
              <DialogFooter>
                <Button appearance="outline" type="button" onClick={() => onClose(null)}>
                  Cancel
                </Button>
                <Button variant="primary" type="submit">
                  Save credential
                </Button>
              </DialogFooter>
            </form>
          </FormProvider>
        </DialogContent>
      </Dialog>
    );
  },
);

function ManagedProviders() {
  const providers = useAdminProviders(true);
  const providerTypes = useProviderTypes(true);
  const queryClient = useQueryClient();
  const { openDialog } = useDialog();
  const confirm = useConfirm();
  const [error, setError] = useState<string | null>(null);

  const refresh = () => queryClient.invalidateQueries({ queryKey: ['admin', 'providers'] });

  const deleteMutation = useMutation({
    mutationFn: deleteAdminProvider,
    onSuccess: refresh,
    onError: (err) => setError(errorMessage(err)),
  });

  const credentialMutation = useMutation({
    mutationFn: async ({ name, api_key }: { name: string; api_key: string }) =>
      setProviderCredential(name, { api_key }),
    onSuccess: refresh,
    onError: (err) => setError(errorMessage(err)),
  });

  const onCreate = async () => {
    try {
      const created = await openDialog(CreateProviderDialog, { typeInfos: providerTypes.data });
      if (created) {
        setError(null);
        await refresh();
      }
    } catch {
      return;
    }
  };

  const openEdit = async (provider: AdminProvider) => {
    try {
      const saved = await openDialog(EditProviderDialog, { provider, typeInfos: providerTypes.data });
      if (saved) {
        setError(null);
        await refresh();
      }
    } catch {
      return;
    }
  };

  const askDelete = async (name: string) => {
    const ok = await confirm({
      title: 'Delete provider',
      description: `Delete provider "${name}"?`,
      confirmText: 'Delete',
    });
    if (ok) {
      setError(null);
      deleteMutation.mutate(name);
    }
  };

  const openRotate = async (name?: string) => {
    try {
      const values = await openDialog(RotateCredentialDialog, name ? { name } : {});
      if (values) {
        setError(null);
        credentialMutation.mutate(values);
      }
    } catch {
      return;
    }
  };

  const rows = providers.data ?? [];
  const busy = deleteMutation.isPending || credentialMutation.isPending;

  const columns: ManagementColumnDef<AdminProvider>[] = useMemo(
    () => [
      {
        accessorKey: 'name',
        header: 'Provider',
        cell: ({ row }) => (
          <span className="font-mono font-medium" title={row.original.display_name || undefined}>
            {row.original.name}
          </span>
        ),
      },
      {
        accessorKey: 'type',
        header: 'Type',
        cell: ({ row }) => <span className="font-mono">{row.original.type}</span>,
      },
      {
        accessorKey: 'source',
        header: 'Source',
        cell: ({ row }) => <SourceBadge source={row.original.source} />,
      },
      {
        id: 'org',
        header: 'Org',
        accessorFn: (row) => row.org_name || row.org_id,
        cell: ({ row }) => <OrgBadge orgId={row.original.org_id} orgName={row.original.org_name} />,
      },
      {
        id: 'models',
        header: 'Models',
        accessorFn: (row) => row.models.length,
        cell: ({ row }) => (
          <span className="text-sm">
            <span className="text-slate-500">
              {row.original.models.length} model(s){row.original.has_credential ? '' : ' · no credential'}
            </span>{' '}
            <span className="font-mono text-xs text-slate-500">
              {row.original.models.map((m) => m.name).join(', ')}
            </span>
          </span>
        ),
      },
      {
        id: 'actions',
        header: '',
        enableSorting: false,
        enableGlobalFilter: false,
        cell: ({ row }) => {
          const p = row.original;
          return p.source === 'database' ? (
            <ActionMenu
              items={[
                { label: 'Edit', onSelect: () => openEdit(p), disabled: busy },
                { label: 'Rotate credential', onSelect: () => openRotate(p.name), disabled: busy },
                { label: 'Delete', onSelect: () => askDelete(p.name), disabled: busy, variant: 'destructive' },
              ]}
            />
          ) : (
            <span className="text-xs text-slate-400" title="Defined in the static config file">
              read-only
            </span>
          );
        },
      },
    ],
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [busy],
  );

  return (
    <>
      {error && (
        <Alert variant="danger">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}
      <Card>
        <CardHeader className="flex flex-row items-center justify-between gap-2">
          <CardTitle>Providers ({rows.length})</CardTitle>
          <Button variant="primary" type="button" onClick={onCreate}>
            Create provider
          </Button>
        </CardHeader>
        <CardContent>
          <DataTable
            columns={columns}
            data={rows}
            filterPlaceholder="Filter providers..."
            getRowId={(row) => row.name}
            emptyText="No providers."
          />
          {providers.isLoading && (
            <div className="text-sm text-slate-500">
              <Spinner size="small">Loading providers...</Spinner>
            </div>
          )}
        </CardContent>
      </Card>
    </>
  );
}

export function ProvidersPage() {
  const status = useAdminStatus();
  const session = useAdminSession();
  const multi = status.data?.multi_tenancy_enabled === true;
  const signedIn = !!session.accessToken;

  return (
    <div className="grid w-full gap-6 p-6">
      {multi && !signedIn && (
        <Alert>
          <AlertDescription>
            Multi-tenancy is enabled.{' '}
            <Link className="underline" to="/login">
              Sign in
            </Link>{' '}
            to add, delete, or rotate database providers.
          </AlertDescription>
        </Alert>
      )}
      {multi && signedIn ? <ManagedProviders /> : <SnapshotProviders />}
    </div>
  );
}
