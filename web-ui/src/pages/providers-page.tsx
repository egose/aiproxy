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
import axios from 'axios';
import { useEffect, useMemo, useRef, useState } from 'react';
import { FormProvider, useFieldArray, useForm } from 'react-hook-form';
import { Link } from 'react-router';
import { z } from 'zod';
import { useSnapshot as useValtioSnapshot } from 'valtio';
import { DataTable, type ManagementColumnDef } from '../components/data-table';
import { useConfirm } from '../components/dialogs';
import { SourceBadge } from '../components/source-badge';
import { WorkspaceBadge } from '../components/workspace-select';
import {
  useAdminWorkspace,
  useAdminProviders,
  useAdminSession,
  useAdminStatus,
  useProviderTypes,
  useSnapshot,
} from '../hooks';
import { authLifecycle } from '../auth-lifecycle';
import { adminWorkspaceStore } from '../store';
import {
  cancelCopilotDeviceFlow,
  createAdminProvider,
  deleteAdminProvider,
  fetchAdminProviders,
  fetchCopilotDeviceFlow,
  pollCopilotDeviceFlow,
  setProviderCredential,
  startCopilotDeviceFlow,
  updateAdminProvider,
} from '../services/admin';
import { errorMessage } from '../services/dashboard';
import { copilotDeviceFlowStatusSchema } from '../types';
import type { AdminProvider, CopilotDeviceFlowStatus, Provider, ProviderTypeInfo } from '../types';

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
  name: z.string().trim(),
  upstreamName: z.string().optional(),
  displayName: z.string().optional(),
  protocol: z.string().optional(),
  capabilities: z.array(z.string()).optional(),
  pricingInput: z.string().optional(),
  pricingOutput: z.string().optional(),
  pricingCached: z.string().optional(),
  pricingCacheWrite: z.string().optional(),
});

const providerFormSchema = z
  .object({
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
    models: z.array(modelRowSchema),
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
  })
  .superRefine((values, ctx) => {
    if (values.extendsName?.trim()) return;
    if (values.models.length === 0) {
      ctx.addIssue({ code: 'custom', path: ['models'], message: 'Add at least one model.' });
    }
    values.models.forEach((model, index) => {
      if (!model.name) {
        ctx.addIssue({ code: 'custom', path: ['models', index, 'name'], message: 'Enter a model name.' });
      }
    });
    if (values.showHealthcheck && !values.hcPath?.trim()) {
      ctx.addIssue({
        code: 'custom',
        path: ['hcPath'],
        message: 'Enter a healthcheck path. Existing healthchecks cannot be removed here.',
      });
    }
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

function buildProviderBody(
  v: ProviderFormValues,
  credentialKind: string,
  initial?: ProviderFormValues,
  device?: { flowId?: string; expectedUpdatedAt?: string; suppressCopilotRef?: boolean; workspaceId?: string },
): Record<string, unknown> {
  const changed = (...fields: Array<keyof ProviderFormValues>) =>
    !initial || fields.some((field) => JSON.stringify(v[field]) !== JSON.stringify(initial[field]));
  const inherited = !!v.extendsName?.trim();
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
  const body: Record<string, unknown> = initial ? {} : { name: v.name.trim() };
  const set = (key: string, field: keyof ProviderFormValues, value: unknown, create = true) => {
    if (changed(field) && (initial || create)) body[key] = value;
  };
  set('type', 'type', v.type);
  set('display_name', 'displayName', (v.displayName ?? '').trim(), !!v.displayName?.trim());
  set('enabled', 'enabled', !!v.enabled, !v.enabled);
  set('extends', 'extendsName', (v.extendsName ?? '').trim(), inherited);
  if (!inherited) {
    set('models', 'models', modelPayloads);
    set('base_url', 'baseUrl', (v.baseUrl ?? '').trim(), !!v.baseUrl?.trim());
    set('upstream_header_timeout', 'timeout', (v.timeout ?? '').trim(), !!v.timeout?.trim());
    set('user_agent', 'userAgent', v.userAgent ?? '', !!v.userAgent);
    set('forward_user_agent', 'forwardUserAgent', !!v.forwardUserAgent, !!v.forwardUserAgent);
    const headers = (v.forwardHeaders ?? '')
      .split(',')
      .map((h) => h.trim())
      .filter(Boolean);
    set('forward_headers', 'forwardHeaders', headers, headers.length > 0);
  }
  if (credentialKind === 'credential_ref') {
    if (device?.flowId) {
      body.copilot_device_flow_id = device.flowId;
    } else if (!device?.suppressCopilotRef && changed('credentialPath', 'credentialName')) {
      body.credential_ref = { path: (v.credentialPath ?? '').trim(), name: (v.credentialName ?? '').trim() };
    }
  } else {
    if ((v.apiKey ?? '') !== '') body.api_key = v.apiKey;
    if (changed('apiKeyRefPath', 'apiKeyRefKey') && (initial || v.apiKeyRefKey?.trim())) {
      body.api_key_ref = { path: (v.apiKeyRefPath ?? '').trim(), key: (v.apiKeyRefKey ?? '').trim() };
    }
  }
  if (!inherited && v.showHealthcheck) {
    const hc: Record<string, unknown> = {};
    const fields: Array<[string, keyof ProviderFormValues, unknown]> = [
      ['path', 'hcPath', (v.hcPath ?? '').trim()],
      ['method', 'hcMethod', v.hcMethod ?? ''],
      ['expected_status', 'hcStatus', Number(v.hcStatus ?? '')],
      ['expected_body', 'hcBody', v.hcBody ?? ''],
      ['interval', 'hcInterval', (v.hcInterval ?? '').trim()],
      ['timeout', 'hcTimeout', (v.hcTimeout ?? '').trim()],
      ['failure_threshold', 'hcFailures', Number(v.hcFailures ?? '')],
      ['success_threshold', 'hcSuccesses', Number(v.hcSuccesses ?? '')],
      ['send_authorization', 'hcSendAuth', !!v.hcSendAuth],
    ];
    for (const [key, field, value] of fields) {
      if (initial?.showHealthcheck ? changed(field) : value !== '' && value !== 0 && value !== false) {
        hc[key] = value;
      }
    }
    if (Object.keys(hc).length > 0) body.healthcheck = hc;
  }
  if (initial && device?.expectedUpdatedAt) {
    body.expected_updated_at = device.expectedUpdatedAt;
  }
  if (!initial && device?.workspaceId) {
    body.workspace_id = device.workspaceId;
  }
  return body;
}

function countdownText(expiresAt: string | undefined, nowMs: number): string {
  if (!expiresAt) return 'unknown';
  const remaining = Date.parse(expiresAt) - nowMs;
  if (!Number.isFinite(remaining) || remaining <= 0) return 'expired';
  const seconds = Math.floor(remaining / 1000);
  const minutes = Math.floor(seconds / 60);
  if (minutes > 0) return `${minutes}m ${seconds % 60}s`;
  return `${seconds}s`;
}

function terminalFlowMessage(flow: CopilotDeviceFlowStatus): string {
  switch (flow.status) {
    case 'denied':
      return 'Authorization denied on GitHub. Retry to start a new authorization.';
    case 'expired':
      return 'Authorization expired before approval. Retry to start a new authorization.';
    case 'cancelled':
      return 'Authorization cancelled. Retry to start a new authorization.';
    default:
      return `Authorization ${flow.status}${flow.error_code ? ` (${flow.error_code})` : ''}. Retry to start a new authorization.`;
  }
}

function ProviderForm({
  initial,
  editProvider,
  submitLabel,
  onSubmit,
  onCancel,
  lockName,
  providerTypeInfos,
}: {
  initial: ProviderFormValues;
  editProvider?: AdminProvider;
  submitLabel: string;
  onSubmit: (body: Record<string, unknown>) => Promise<void>;
  onCancel?: () => void;
  lockName: boolean;
  providerTypeInfos: ProviderTypeInfo[] | undefined;
}) {
  const form = useForm<ProviderFormValues>({ resolver: zodResolver(providerFormSchema), defaultValues: initial });
  const { fields, append, remove } = useFieldArray({ control: form.control, name: 'models' });
  const queryClient = useQueryClient();
  const selectedType = form.watch('type');
  const inherited = !!form.watch('extendsName')?.trim();
  const showHealthcheck = form.watch('showHealthcheck');
  const typeInfo = providerTypeInfos?.find((t) => t.type === selectedType);
  const credentialKind = typeInfo?.credential ?? 'api_key';
  const protocolRequired = typeInfo?.model_protocol_required ?? false;
  const supportedCaps = typeInfo?.supported_capabilities ?? [];
  const supportsDevice = typeInfo?.supports_device_authorization === true || selectedType === 'github-copilot';
  const isCopilotForm = credentialKind === 'credential_ref' && selectedType === 'github-copilot';
  const deviceEnabled = supportsDevice && selectedType === 'github-copilot';

  const { workspaceId: ambientWorkspaceId } = useAdminWorkspace();
  const authSnap = useValtioSnapshot(authLifecycle);
  const [copilotSource, setCopilotSource] = useState<'device' | 'sidecar' | 'keep'>(lockName ? 'keep' : 'device');
  const [copilotClientId, setCopilotClientId] = useState('');
  const [copilotFlow, setCopilotFlow] = useState<CopilotDeviceFlowStatus | null>(null);
  const [copilotPinned, setCopilotPinned] = useState<{
    workspaceId: string;
    providerName: string;
    generation: number;
    session: number;
  } | null>(null);
  const [copilotError, setCopilotError] = useState<string | null>(null);
  const [copilotStarting, setCopilotStarting] = useState(false);
  const [copilotPolling, setCopilotPolling] = useState(false);
  const [copilotCopied, setCopilotCopied] = useState(false);
  const [revision, setRevision] = useState(editProvider?.updated_at ?? '');
  const [saveNote, setSaveNote] = useState<string | null>(null);
  const [saveConflict, setSaveConflict] = useState(false);
  const [saveRecovery, setSaveRecovery] = useState<string | null>(null);
  const [nowMs, setNowMs] = useState(() => Date.now());
  const attemptRef = useRef(0);
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const countdownRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const abortRef = useRef<AbortController | null>(null);
  const flowRef = useRef<CopilotDeviceFlowStatus | null>(null);
  flowRef.current = copilotFlow;
  const pinnedRef = useRef<typeof copilotPinned>(null);
  pinnedRef.current = copilotPinned;
  const sourceRef = useRef(copilotSource);
  sourceRef.current = copilotSource;

  const clearDeviceTimer = () => {
    if (timerRef.current) {
      clearTimeout(timerRef.current);
      timerRef.current = null;
    }
  };
  const abortDeviceRequest = () => {
    abortRef.current?.abort();
    abortRef.current = null;
  };
  const bestEffortCancel = (flowId: string | null | undefined, generation: number | null) => {
    if (!flowId) return;
    if (generation !== null && generation !== authLifecycle.generation) return;
    void cancelCopilotDeviceFlow(flowId).catch(() => undefined);
  };

  useEffect(() => {
    if (!deviceEnabled || copilotSource !== 'device' || !copilotFlow) return;
    if (copilotFlow.status !== 'pending' && copilotFlow.status !== 'starting' && copilotFlow.status !== 'ready') return;
    countdownRef.current = setInterval(() => setNowMs(Date.now()), 1000);
    return () => {
      if (countdownRef.current) clearInterval(countdownRef.current);
      countdownRef.current = null;
    };
  }, [deviceEnabled, copilotSource, copilotFlow?.id, copilotFlow?.status]);

  const installFlowStatus = (attempt: number, generation: number, status: CopilotDeviceFlowStatus) => {
    if (attempt !== attemptRef.current) return;
    if (generation !== authLifecycle.generation) return;
    setCopilotFlow(status);
    setCopilotError(null);
  };

  const doPoll = (flowId: string, attempt: number, generation: number) => {
    if (copilotPolling) return;
    setCopilotPolling(true);
    const controller = new AbortController();
    abortRef.current = controller;
    void (async () => {
      try {
        const status = await pollCopilotDeviceFlow(flowId, { signal: controller.signal });
        installFlowStatus(attempt, generation, status);
      } catch (err) {
        if (axios.isCancel(err)) return;
        if (attempt !== attemptRef.current || generation !== authLifecycle.generation) return;
        if (axios.isAxiosError(err) && err.response?.status === 409 && err.response?.data) {
          try {
            installFlowStatus(attempt, generation, copilotDeviceFlowStatusSchema.parse(err.response.data));
            return;
          } catch {
            // fall through to generic error
          }
        }
        setCopilotError(errorMessage(err));
      } finally {
        if (attempt === attemptRef.current) setCopilotPolling(false);
      }
    })();
  };

  useEffect(() => {
    if (!deviceEnabled || copilotSource !== 'device' || !copilotFlow) return;
    if (copilotFlow.status !== 'pending' && copilotFlow.status !== 'starting') return;
    if (copilotPolling) return;
    const delay = Math.max(Number(copilotFlow.poll_after_ms ?? 0) || 5000, 10);
    const attempt = attemptRef.current;
    const generation = copilotPinned?.generation ?? authLifecycle.generation;
    timerRef.current = setTimeout(() => {
      const current = flowRef.current;
      if (!current || current.id !== copilotFlow.id) return;
      if (attempt !== attemptRef.current || generation !== authLifecycle.generation) return;
      doPoll(copilotFlow.id, attempt, generation);
    }, delay);
    return () => clearDeviceTimer();
  }, [deviceEnabled, copilotSource, copilotFlow, copilotPolling, copilotPinned]);

  useEffect(
    () => () => {
      attemptRef.current++;
      clearDeviceTimer();
      if (countdownRef.current) clearInterval(countdownRef.current);
      abortDeviceRequest();
      const live = flowRef.current;
      const pinned = pinnedRef.current;
      if (live && (live.status === 'starting' || live.status === 'pending' || live.status === 'ready')) {
        bestEffortCancel(live.id, pinned?.generation ?? null);
      }
    },
    [],
  );

  const handleSourceChange = (next: 'device' | 'sidecar' | 'keep') => {
    const attempt = ++attemptRef.current;
    void attempt;
    clearDeviceTimer();
    abortDeviceRequest();
    const live = flowRef.current;
    const pinned = pinnedRef.current;
    if (live && (live.status === 'starting' || live.status === 'pending' || live.status === 'ready')) {
      bestEffortCancel(live.id, pinned?.generation ?? null);
    }
    setCopilotFlow(null);
    setCopilotPinned(null);
    setCopilotError(null);
    setCopilotCopied(false);
    setCopilotSource(next);
  };

  useEffect(() => {
    if (!deviceEnabled && (flowRef.current || copilotPinned)) {
      handleSourceChange(lockName ? 'keep' : 'sidecar');
    }
  }, [deviceEnabled]);

  const startDeviceFlow = () => {
    const clientId = copilotClientId.trim();
    if (!clientId) {
      setCopilotError('Enter the public OAuth client ID.');
      return;
    }
    const attempt = ++attemptRef.current;
    clearDeviceTimer();
    abortDeviceRequest();
    const live = flowRef.current;
    const pinned = pinnedRef.current;
    if (live && (live.status === 'starting' || live.status === 'pending' || live.status === 'ready')) {
      bestEffortCancel(live.id, pinned?.generation ?? null);
    }
    const generation = authLifecycle.generation;
    const session = authLifecycle.session;
    const pinnedWorkspace = ambientWorkspaceId ?? adminWorkspaceStore.workspaceId ?? '';
    const providerName = lockName ? initial.name.trim() : '';
    const controller = new AbortController();
    abortRef.current = controller;
    setCopilotStarting(true);
    setCopilotError(null);
    setCopilotCopied(false);
    const body: Record<string, unknown> = { client_id: clientId };
    if (lockName) {
      if (providerName) body.provider_name = providerName;
    } else if (pinnedWorkspace) {
      body.workspace_id = pinnedWorkspace;
    }
    void (async () => {
      try {
        const status = await startCopilotDeviceFlow(body, { signal: controller.signal });
        if (attempt !== attemptRef.current || generation !== authLifecycle.generation) return;
        setCopilotPinned({ workspaceId: status.workspace_id ?? pinnedWorkspace, providerName, generation, session });
        setCopilotFlow(status);
      } catch (err) {
        if (axios.isCancel(err)) return;
        if (attempt !== attemptRef.current || generation !== authLifecycle.generation) return;
        setCopilotError(errorMessage(err));
      } finally {
        if (attempt === attemptRef.current) setCopilotStarting(false);
      }
    })();
  };

  const cancelDeviceFlow = () => {
    const live = flowRef.current;
    if (!live) return;
    const attempt = attemptRef.current;
    const generation = copilotPinned?.generation ?? authLifecycle.generation;
    clearDeviceTimer();
    const controller = new AbortController();
    abortRef.current = controller;
    void (async () => {
      try {
        const status = await cancelCopilotDeviceFlow(live.id, { signal: controller.signal });
        installFlowStatus(attempt, generation, status);
      } catch (err) {
        if (axios.isCancel(err)) return;
        if (attempt !== attemptRef.current || generation !== authLifecycle.generation) return;
        setCopilotError(errorMessage(err));
      }
    })();
  };

  const retryDeviceFlow = () => {
    const live = flowRef.current;
    const pinned = pinnedRef.current;
    if (live && (live.status === 'starting' || live.status === 'pending')) {
      bestEffortCancel(live.id, pinned?.generation ?? null);
    }
    clearDeviceTimer();
    setCopilotFlow(null);
    setCopilotPinned(null);
    startDeviceFlow();
  };

  const copyUserCode = () => {
    const code = copilotFlow?.user_code ?? '';
    if (!code) return;
    try {
      const done = () => {
        if (attemptRef.current === attemptRef.current) setCopilotCopied(true);
      };
      const result = navigator.clipboard?.writeText(code) as Promise<void> | undefined;
      if (result?.then) void result.then(done).catch(() => setCopilotCopied(false));
      else done();
    } catch {
      setCopilotCopied(false);
    }
  };

  const pinnedMismatch =
    copilotPinned &&
    (ambientWorkspaceId ?? '') !== '' &&
    (copilotPinned.workspaceId ?? '') !== '' &&
    (ambientWorkspaceId ?? '') !== (copilotPinned.workspaceId ?? '');
  const authDrifted =
    copilotPinned && (copilotPinned.generation !== authSnap.generation || copilotPinned.session !== authSnap.session);
  const deviceReady =
    deviceEnabled && copilotSource === 'device' && copilotFlow?.status === 'ready' && !pinnedMismatch && !authDrifted;
  const deviceBlocksSave = deviceEnabled && copilotSource === 'device' && !deviceReady;

  const submitMutation = useMutation({
    mutationFn: async (values: ProviderFormValues) => {
      const info = providerTypeInfos?.find((t) => t.type === values.type);
      const kind = info?.credential ?? 'api_key';
      const copilotDevice = kind === 'credential_ref' && values.type === 'github-copilot';
      if (copilotDevice && copilotSource === 'device' && !deviceReady) {
        throw new Error('Authorize with GitHub before saving.');
      }
      if (copilotDevice && copilotSource === 'device' && (pinnedMismatch || authDrifted)) {
        throw new Error('Authorization scope changed; restart Connect GitHub.');
      }
      const flowId = copilotDevice && copilotSource === 'device' && deviceReady ? copilotFlow?.id : undefined;
      const suppressCopilotRef = copilotDevice ? copilotSource !== 'sidecar' : false;
      const body = buildProviderBody(values, kind, lockName ? initial : undefined, {
        flowId,
        expectedUpdatedAt: lockName && revision ? revision : undefined,
        suppressCopilotRef,
        workspaceId: !lockName && flowId && copilotPinned?.workspaceId ? copilotPinned.workspaceId : undefined,
      });
      setSaveNote(null);
      setSaveConflict(false);
      setSaveRecovery(null);
      return onSubmit(body);
    },
    onError: (err) => {
      if (axios.isAxiosError(err)) {
        const status = err.response?.status;
        const headers = (err.response?.headers ?? {}) as Record<string, unknown>;
        const savedHeader = headers['x-aiproxy-catalog-saved'] ?? headers['X-Aiproxy-Catalog-Saved'];
        const dataText = typeof err.response?.data === 'string' ? (err.response?.data as string) : '';
        if (status === 409) {
          setSaveConflict(true);
          setSaveNote(
            'Provider changed; reread current state and review before retrying. Your authorization is preserved.',
          );
          return;
        }
        if (status === 500 && (savedHeader === 'true' || dataText.includes('saved'))) {
          setSaveNote(
            'Provider saved, but activation failed. Current saved state was reloaded; verify before retrying.',
          );
          void queryClient.invalidateQueries({ queryKey: ['admin', 'providers'] });
          return;
        }
        if (!err.response) {
          setSaveNote('Save request did not complete; check current state before retrying to avoid duplication.');
        }
      }
    },
  });

  const rereadRevision = async () => {
    setSaveRecovery(null);
    try {
      const providers = await fetchAdminProviders();
      const current = providers.find((p) => p.name === initial.name.trim());
      if (current?.updated_at) {
        setRevision(current.updated_at);
        setSaveRecovery(`Reloaded revision ${current.updated_at}; review and retry save.`);
        setSaveConflict(false);
      } else if (!lockName) {
        setSaveRecovery('Reloaded provider list; review before retrying.');
        setSaveConflict(false);
      } else {
        setSaveRecovery('Provider not found in current state; review before retrying.');
      }
      await queryClient.invalidateQueries({ queryKey: ['admin', 'providers'] });
    } catch (err) {
      setSaveRecovery(errorMessage(err));
    }
  };

  const checkSavedState = async () => {
    setSaveRecovery(null);
    try {
      const providers = await fetchAdminProviders();
      const current = lockName ? providers.find((p) => p.name === initial.name.trim()) : undefined;
      let flowText = '';
      if (copilotFlow?.id) {
        try {
          const status = await fetchCopilotDeviceFlow(copilotFlow.id);
          setCopilotFlow(status);
          if (status.status === 'consumed') flowText = ' Device authorization shows consumed (saved).';
          else if (status.status === 'ready') flowText = ' Device authorization is still ready; safe to retry.';
          else flowText = ` Device authorization is ${status.status}.`;
        } catch (err) {
          if (!axios.isCancel(err)) flowText = ` Flow check failed: ${errorMessage(err)}`;
        }
      }
      if (current?.updated_at && current.updated_at !== revision) {
        setRevision(current.updated_at);
        setSaveRecovery(`Current saved revision is ${current.updated_at}.${flowText}`);
      } else if (!lockName) {
        const created = providers.find((p) => p.name === form.getValues('name').trim());
        setSaveRecovery(
          created
            ? `Provider ${created.name} exists in current state.${flowText}`
            : `No matching provider found.${flowText}`,
        );
      } else {
        setSaveRecovery(`Current state loaded.${flowText}`);
      }
      await queryClient.invalidateQueries({ queryKey: ['admin', 'providers'] });
    } catch (err) {
      setSaveRecovery(errorMessage(err));
    }
  };

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
        {saveNote && (
          <Alert variant={saveConflict ? 'danger' : undefined}>
            <AlertDescription>{saveNote}</AlertDescription>
          </Alert>
        )}
        {saveRecovery && (
          <Alert>
            <AlertDescription>{saveRecovery}</AlertDescription>
          </Alert>
        )}
        {(saveConflict || saveNote?.includes('did not complete')) && (
          <div className="flex flex-wrap gap-2">
            {saveConflict && (
              <Button type="button" appearance="outline" onClick={() => void rereadRevision()}>
                Reload current values
              </Button>
            )}
            <Button type="button" appearance="outline" onClick={() => void checkSavedState()}>
              Check saved state
            </Button>
          </div>
        )}
        {saveNote?.includes('saved, but activation failed') && (
          <div className="flex flex-wrap gap-2">
            <Button type="button" appearance="outline" onClick={() => void checkSavedState()}>
              Check saved state
            </Button>
          </div>
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
          name="extendsName"
          label="Extends (optional, static base provider of the same type)"
          placeholder="base provider name"
        />
        {inherited && (
          <p className="text-sm text-slate-500">
            Models, transport settings and healthcheck are inherited from the base provider. Type must match the base.
            Enabled controls this database provider locally; a blank display name uses the base display name.
          </p>
        )}
        {!inherited && (
          <HookFormTextInput<ProviderFormValues>
            name="baseUrl"
            label={`Base URL${typeInfo?.requires_base_url ? ' (required for this type)' : ' (optional)'}`}
            placeholder="https://..."
          />
        )}
        <HookFormCheckbox<ProviderFormValues> name="enabled" label="Enabled" />
        <Separator />
        {lockName && (
          <p className="text-sm text-slate-500">
            Leave the API key blank to keep the stored secret. Unchanged credential references are preserved without
            being resent.
          </p>
        )}
        {isCopilotForm ? (
          <div className="grid gap-3 rounded-md border p-3">
            <span className="text-sm font-medium">Copilot credential</span>
            {lockName && editProvider && (
              <p className="text-sm text-slate-500" role="status">
                Current credential: {editProvider.copilot_credential_source || 'unknown'}
                {editProvider.has_credential ? '' : ' (no stored credential)'}.
              </p>
            )}
            <label className="grid gap-1 text-sm">
              <span className="font-medium">Credential source</span>
              <select
                aria-label="Credential source"
                value={copilotSource}
                onChange={(e) => handleSourceChange(e.target.value as 'device' | 'sidecar' | 'keep')}
              >
                {!lockName && <option value="device">Authorize with GitHub</option>}
                {!lockName && <option value="sidecar">Existing CLI sidecar</option>}
                {lockName && <option value="keep">Keep current credential</option>}
                {lockName && <option value="device">Reauthorize with GitHub</option>}
                {lockName && <option value="sidecar">Switch to CLI sidecar</option>}
              </select>
            </label>
            {deviceEnabled && copilotSource === 'device' && (
              <div className="grid gap-2">
                <label className="grid gap-1 text-sm">
                  <span className="font-medium">GitHub OAuth client ID (public)</span>
                  <Input
                    aria-label="GitHub OAuth client ID"
                    placeholder="Iv1...."
                    value={copilotClientId}
                    onChange={(e) => setCopilotClientId(e.target.value)}
                    disabled={copilotStarting || copilotFlow?.status === 'pending'}
                  />
                </label>
                {!copilotFlow && (
                  <div>
                    <Button
                      type="button"
                      variant="primary"
                      disabled={copilotStarting || !copilotClientId.trim()}
                      onClick={startDeviceFlow}
                    >
                      {copilotStarting ? 'Starting...' : 'Connect GitHub'}
                    </Button>
                  </div>
                )}
                {copilotError && (
                  <Alert variant="danger">
                    <AlertDescription>{copilotError}</AlertDescription>
                  </Alert>
                )}
                {copilotFlow && (copilotFlow.status === 'pending' || copilotFlow.status === 'starting') && (
                  <div className="grid gap-2" role="status" aria-live="polite">
                    <p className="text-sm">
                      {copilotFlow.status === 'starting'
                        ? 'Starting authorization...'
                        : 'Waiting for GitHub approval...'}
                      {copilotFlow.expires_at && (
                        <span> Expires in {countdownText(copilotFlow.expires_at, nowMs)}.</span>
                      )}
                    </p>
                    {copilotFlow.user_code && (
                      <p className="text-sm">
                        Code: <span className="font-mono font-bold">{copilotFlow.user_code}</span>
                      </p>
                    )}
                    <div className="flex flex-wrap gap-2">
                      {copilotFlow.verification_uri && (
                        <a
                          href={copilotFlow.verification_uri}
                          target="_blank"
                          rel="noreferrer"
                          className="text-sm underline"
                        >
                          Open GitHub to authorize
                        </a>
                      )}
                      {copilotFlow.user_code && (
                        <Button type="button" appearance="outline" onClick={copyUserCode}>
                          {copilotCopied ? 'Copied' : 'Copy code'}
                        </Button>
                      )}
                      <Button type="button" appearance="outline" onClick={cancelDeviceFlow}>
                        Cancel authorization
                      </Button>
                    </div>
                    {copilotPolling && <p className="text-xs text-slate-500">Checking approval...</p>}
                  </div>
                )}
                {copilotFlow && copilotFlow.status === 'ready' && (
                  <div className="grid gap-2" role="status" aria-live="polite">
                    <Alert>
                      <AlertDescription>
                        Authorized; save provider to apply.
                        {copilotFlow.ready_expires_at && (
                          <span> Ready expires in {countdownText(copilotFlow.ready_expires_at, nowMs)}.</span>
                        )}
                      </AlertDescription>
                    </Alert>
                    <div className="flex gap-2">
                      <Button type="button" appearance="outline" onClick={cancelDeviceFlow}>
                        Cancel authorization
                      </Button>
                    </div>
                  </div>
                )}
                {copilotFlow && ['denied', 'expired', 'failed', 'cancelled'].includes(copilotFlow.status) && (
                  <div className="grid gap-2" role="status" aria-live="polite">
                    <Alert variant="danger">
                      <AlertDescription>{terminalFlowMessage(copilotFlow)}</AlertDescription>
                    </Alert>
                    <div>
                      <Button type="button" appearance="outline" onClick={retryDeviceFlow}>
                        Retry authorization
                      </Button>
                    </div>
                  </div>
                )}
                {copilotFlow && copilotFlow.status === 'consumed' && (
                  <Alert>
                    <AlertDescription>
                      Authorization already applied to {copilotFlow.provider_name || 'a provider'}. Reauthorize to
                      change the credential.
                    </AlertDescription>
                  </Alert>
                )}
                {pinnedMismatch ? (
                  <Alert variant="danger">
                    <AlertDescription>
                      Workspace changed since authorization started; restart Connect GitHub.
                    </AlertDescription>
                  </Alert>
                ) : null}
                {authDrifted ? (
                  <Alert variant="danger">
                    <AlertDescription>
                      Account changed since authorization started; restart Connect GitHub.
                    </AlertDescription>
                  </Alert>
                ) : null}
              </div>
            )}
            {copilotSource === 'sidecar' && (
              <>
                <HookFormTextInput<ProviderFormValues>
                  name="credentialName"
                  label="Credential name (required, from `aiproxy login`)"
                  placeholder="copilot-main"
                />
                <HookFormTextInput<ProviderFormValues> name="credentialPath" label="Credential path (optional)" />
              </>
            )}
            {lockName && copilotSource === 'keep' && (
              <p className="text-sm text-slate-500">
                The stored credential stays unchanged unless you reauthorize or switch sources.
              </p>
            )}
          </div>
        ) : credentialKind === 'credential_ref' ? (
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
        {!inherited && (
          <>
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
            <p className="text-sm text-slate-500">
              Blank URL, timeout and user agent use provider/root defaults; a URL is still required for types without a
              default. Turning off forwarding or clearing headers only removes local settings: root forwarding still
              applies.
            </p>
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
                <HookFormCheckbox<ProviderFormValues>
                  name="showHealthcheck"
                  label="Custom healthcheck"
                  disabled={lockName && !!initial.showHealthcheck}
                />
                {lockName && initial.showHealthcheck && (
                  <p className="text-sm text-slate-500">
                    This API cannot remove an existing healthcheck. You can edit its settings; the block is retained.
                  </p>
                )}
                {showHealthcheck && (
                  <>
                    <p className="text-sm text-slate-500">
                      Blank scalar fields reset to defaults: GET, status 200, body *, interval 30s, timeout 5s, failure
                      threshold 2 and success threshold 1. Authorization is sent only when checked.
                    </p>
                    <HookFormTextInput<ProviderFormValues>
                      name="hcPath"
                      label="Path (required)"
                      placeholder="/healthz"
                    />
                    <div className="grid grid-cols-2 gap-2">
                      <HookFormNativeSelect<ProviderFormValues>
                        name="hcMethod"
                        label="Method"
                        data={[
                          { label: 'Default (GET)', value: '' },
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
              </>
            )}
          </>
        )}
        <div className="flex gap-2">
          <Button variant="primary" type="submit" disabled={submitMutation.isPending || deviceBlocksSave}>
            {submitMutation.isPending ? 'Saving...' : submitLabel}
          </Button>
          {deviceEnabled && copilotSource === 'device' && !deviceReady && (
            <span className="text-sm text-slate-500" role="status">
              Save is available after authorization is ready.
            </span>
          )}
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
        editProvider={args.provider}
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

  const openRotate = async (provider?: AdminProvider | string) => {
    const target = typeof provider === 'string' ? rows.find((p) => p.name === provider) : provider;
    if (target?.type === 'github-copilot') {
      await openEdit(target);
      return;
    }
    try {
      const name = typeof provider === 'string' ? provider : provider?.name;
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
        id: 'workspace',
        header: 'Workspace',
        accessorFn: (row) => row.workspace_name || row.workspace_id,
        cell: ({ row }) => (
          <WorkspaceBadge workspaceId={row.original.workspace_id} workspaceName={row.original.workspace_name} />
        ),
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
              trigger={
                <Button variant="secondary" appearance="outline-filled" size="sm" aria-label={`Actions for ${p.name}`}>
                  Actions
                </Button>
              }
              items={[
                { label: 'Edit', onSelect: () => openEdit(p), disabled: busy || !providerTypes.data },
                { label: 'Rotate credential', onSelect: () => openRotate(p), disabled: busy },
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
    [busy, providerTypes.data],
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
          <Button variant="primary" type="button" onClick={onCreate} disabled={!providerTypes.data}>
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
