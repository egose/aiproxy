import { zodResolver } from '@hookform/resolvers/zod';
import { HookFormCheckbox } from '@egose/shadcn-theme/components/form/hook-checkbox';
import { HookFormNativeSelect } from '@egose/shadcn-theme/components/form/hook-native-select';
import { HookFormTextInput } from '@egose/shadcn-theme/components/form/hook-text-input';
import { HookFormTextarea } from '@egose/shadcn-theme/components/form/hook-textarea';
import { Alert, AlertDescription } from '@egose/shadcn-theme/components/ui/alert';
import { Button } from '@egose/shadcn-theme/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@egose/shadcn-theme/components/ui/card';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@egose/shadcn-theme/components/ui/dialog';
import { Separator } from '@egose/shadcn-theme/components/ui/separator';
import { Spinner } from '@egose/shadcn-theme/components/ui/spinner';
import { createTypedDialog, useDialog } from '@egose/shadcn-theme/components/widgets/dialog-manager';
import { ActionMenu } from '@egose/shadcn-theme/components/widgets/action-menu';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useMemo, useState } from 'react';
import { Controller, FormProvider, useForm, useFormContext } from 'react-hook-form';
import { Link } from 'react-router';
import { z } from 'zod';
import { useSnapshot as useStateSnapshot } from 'valtio';
import { DataTable, type ManagementColumnDef } from '../components/data-table';
import { useConfirm } from '../components/dialogs';
import { EncryptedReasoningControl, ReasoningPolicyReadOnly } from '../components/encrypted-reasoning-control';
import { SourceBadge } from '../components/source-badge';
import { WorkspaceBadge } from '../components/workspace-select';
import { WorkspaceAccessNotice } from '../components/workspace-access';
import {
  useAdminAliases,
  useAdminProviders,
  useAdminSession,
  useAdminStatus,
  useCurrentWorkspace,
  useSnapshot,
} from '../hooks';
import { reasoningDeclaration, reasoningDraft, reasoningDraftSchema } from '../lib/encrypted-reasoning-form';
import { createAdminAlias, deleteAdminAlias, updateAdminAlias } from '../services/admin';
import { errorMessage } from '../services/dashboard';
import type { AdminAlias, AdminProvider } from '../types';
import { authLifecycle } from '../auth-lifecycle';
import { assertWorkspaceContext, resourceInWorkspace, type WorkspaceContext } from '../workspace-selection';

function SnapshotAliases() {
  const snapshot = useSnapshot();
  const rows = snapshot.data?.aliases ?? [];
  return (
    <Card>
      <CardHeader>
        <CardTitle>Aliases ({rows.length})</CardTitle>
      </CardHeader>

      <CardContent className="grid gap-2">
        {rows.map((alias) => (
          <div key={alias.name} className="text-sm">
            <span className="font-mono">alias/{alias.name}</span>
            <span> ({alias.algorithm}) → </span>

            <span className="font-mono">{alias.targets.map((t) => `${t.provider}/${t.model}`).join(', ')}</span>
          </div>
        ))}
      </CardContent>
    </Card>
  );
}

const isNameSegment = (name: string) => /^[a-z0-9][a-z0-9._-]*$/.test(name) && !/\s/.test(name);
const isProviderName = (name: string) => isNameSegment(name) && name !== 'alias';
const isModelName = (name: string) => name.split('/').every(isNameSegment);
const providerNameError =
  'Provider must start with a lowercase letter or digit and contain only lowercase letters, digits, dots, underscores or hyphens; "alias" is reserved.';
const modelNameError =
  'Model must contain nonempty slash-separated segments, each starting with a lowercase letter or digit and containing only lowercase letters, digits, dots, underscores or hyphens. Do not put spaces inside the model name.';

function parseTargetLines(raw: string) {
  const targets = raw.split('\n').flatMap((line, index) => {
    const text = line.trim();
    if (!text) return [];
    const separator = text.indexOf('/');
    if (separator < 0) throw new Error(`Line ${index + 1}: Enter provider/model, for example gateway/z-ai/glm-5.2.`);
    const provider = text.slice(0, separator).trim();
    const model = text.slice(separator + 1).trim();
    if (!isProviderName(provider)) throw new Error(`Line ${index + 1}: ${providerNameError}`);
    if (!isModelName(model)) throw new Error(`Line ${index + 1}: ${modelNameError}`);
    return [{ provider, model }];
  });
  if (!targets.length) throw new Error('Enter at least one target or use providers/model shorthand.');
  return targets;
}

function parseShorthandProviders(raw: string) {
  if (!raw.trim()) throw new Error('Enter at least one provider, separated by commas.');
  const providers = raw.split(',').map((provider) => provider.trim());
  const seen = new Set<string>();
  providers.forEach((provider, index) => {
    if (!isProviderName(provider)) throw new Error(`Provider ${index + 1}: ${providerNameError}`);
    if (seen.has(provider)) throw new Error(`Duplicate provider "${provider}". List each provider only once.`);
    seen.add(provider);
  });
  return providers;
}

const aliasFormSchema = z
  .object({
    name: z.string().trim().min(1, 'Enter an alias name.'),
    algorithm: z.enum(['round_robin', 'least_connections']).optional(),
    retryCodes: z.string().optional(),
    targets: z.string().optional(),
    shorthandProviders: z.string().optional(),
    shorthandModel: z.string().optional(),
    useAffinity: z.boolean().optional(),
    affinityHeaders: z.string().optional(),
    reasoning: reasoningDraftSchema,
  })
  .superRefine((values, ctx) => {
    const shorthand = (values.shorthandProviders ?? '').trim() !== '' || (values.shorthandModel ?? '').trim() !== '';
    if (shorthand) {
      if ((values.targets ?? '').trim()) {
        ctx.addIssue({
          code: 'custom',
          path: ['targets'],
          message:
            'Use targets or shorthand, not both. Clear targets to use shorthand, or clear both shorthand fields.',
        });
      }
      try {
        parseShorthandProviders(values.shorthandProviders ?? '');
      } catch (error) {
        ctx.addIssue({ code: 'custom', path: ['shorthandProviders'], message: errorMessage(error) });
      }
      if (!isModelName((values.shorthandModel ?? '').trim())) {
        ctx.addIssue({ code: 'custom', path: ['shorthandModel'], message: modelNameError });
      }
    } else {
      try {
        parseTargetLines(values.targets ?? '');
      } catch (error) {
        ctx.addIssue({ code: 'custom', path: ['targets'], message: errorMessage(error) });
      }
    }
  });

type AliasFormValues = z.infer<typeof aliasFormSchema>;

const emptyAliasForm = (): AliasFormValues => ({
  name: '',
  algorithm: 'round_robin',
  retryCodes: '',
  targets: '',
  shorthandProviders: '',
  shorthandModel: '',
  useAffinity: false,
  affinityHeaders: '',
  reasoning: reasoningDraft(),
});

function aliasToFormValues(a: AdminAlias): AliasFormValues {
  return {
    name: a.name,
    algorithm: a.algorithm === 'least_connections' ? 'least_connections' : 'round_robin',
    retryCodes: (a.retry_status_codes ?? []).join(','),
    targets: (a.targets ?? []).map((t) => `${t.provider}/${t.model}`).join('\n'),
    shorthandProviders: '',
    shorthandModel: '',
    useAffinity: a.session_affinity != null,
    affinityHeaders: (a.session_affinity?.headers ?? []).join(', '),
    reasoning: reasoningDraft(a.encrypted_reasoning),
  };
}

function buildAliasBody(v: AliasFormValues, initial?: AliasFormValues): Record<string, unknown> {
  const body: Record<string, unknown> = { name: v.name.trim(), algorithm: v.algorithm ?? 'round_robin' };
  const codes = (v.retryCodes ?? '')
    .split(',')
    .map((c) => c.trim())
    .filter(Boolean)
    .map((c) => Number(c));
  if (codes.length > 0) {
    if (codes.some((c) => !Number.isInteger(c))) throw new Error('retry status codes must be integers');
    body.retry_status_codes = codes;
  }
  if ((v.shorthandProviders ?? '').trim() !== '' || (v.shorthandModel ?? '').trim() !== '') {
    body.providers = parseShorthandProviders(v.shorthandProviders ?? '');
    body.model = (v.shorthandModel ?? '').trim();
  } else {
    body.targets = parseTargetLines(v.targets ?? '');
  }
  if (v.useAffinity) {
    const headers = (v.affinityHeaders ?? '')
      .split(',')
      .map((h) => h.trim())
      .filter(Boolean);
    body.session_affinity = { headers };
  }
  const local = reasoningDeclaration(v.reasoning);
  if (initial ? JSON.stringify(local) !== JSON.stringify(reasoningDeclaration(initial.reasoning)) : local !== null) {
    body.encrypted_reasoning = local;
  }
  return body;
}

function AliasFields({
  lockName,
  alias,
  disabled,
  inventory,
}: {
  lockName: boolean;
  alias?: AdminAlias;
  disabled: boolean;
  inventory: AdminProvider[];
}) {
  const form = useFormContext<AliasFormValues>();
  const [target, setTarget] = useState('');
  const models = inventory
    .filter((provider) => provider.enabled)
    .flatMap((provider) => provider.models.map((model) => `${provider.name}/${model.name}`));
  return (
    <>
      <HookFormTextInput<AliasFormValues>
        name="name"
        label="Name (lowercase, no spaces)"
        placeholder="fast"
        disabled={lockName}
      />

      <HookFormNativeSelect<AliasFormValues>
        name="algorithm"
        label="Algorithm"
        data={[
          { label: 'round_robin', value: 'round_robin' },
          { label: 'least_connections', value: 'least_connections' },
        ]}
      />

      <HookFormTextInput<AliasFormValues>
        name="retryCodes"
        label="Retry status codes (optional, default 500,502,503,504)"
        placeholder="500,502,503,504"
        inputMode="numeric"
      />

      <Separator />

      <label className="grid gap-1 text-sm">
        <span>Available targets in this workspace</span>
        <select
          aria-label="Available targets in this workspace"
          value={target}
          onChange={(event) => setTarget(event.target.value)}
          disabled={disabled}
        >
          <option value="">Select a target...</option>
          {models.map((model) => (
            <option key={model} value={model}>
              {model}
            </option>
          ))}
        </select>
      </label>
      <Button
        type="button"
        appearance="outline"
        disabled={disabled || !target}
        onClick={() => {
          form.setValue('targets', [form.getValues('targets')?.trim(), target].filter(Boolean).join('\n'), {
            shouldValidate: true,
          });
          setTarget('');
        }}
      >
        Add target
      </Button>

      <HookFormTextarea<AliasFormValues>
        name="targets"
        label="Targets (one provider/model per line)"
        placeholder={'db-openai/gpt-4o-mini\nbackup/gpt-4o-mini'}
        rows={3}
      />

      <p className="text-sm text-slate-500">
        The first slash separates the provider from the full model name: gateway/z-ai/glm-5.2 targets model
        z-ai/glm-5.2. Blank lines and whitespace around the provider and full model are ignored. Each name segment must
        start with a lowercase letter or digit; dots, underscores and hyphens are also allowed after the first
        character. Empty segments and spaces inside names are invalid. The same naming rules apply to shorthand;
        provider name alias is reserved.
      </p>

      <div className="grid gap-2">
        <span className="text-sm font-medium">
          ...or shorthand: same model across providers (cannot be combined with targets)
        </span>

        <div className="grid grid-cols-2 gap-2">
          <HookFormTextInput<AliasFormValues>
            name="shorthandProviders"
            label="Providers"
            placeholder="providers, comma separated"
          />

          <HookFormTextInput<AliasFormValues> name="shorthandModel" label="Model" placeholder="model" />
        </div>
      </div>

      <Separator />

      <HookFormCheckbox<AliasFormValues> name="useAffinity" label="Session affinity" />

      <HookFormTextInput<AliasFormValues>
        name="affinityHeaders"
        label="Headers (optional, comma separated; empty uses defaults)"
        placeholder="x-session-id"
      />

      <Controller
        control={form.control}
        name="reasoning"
        render={({ field, fieldState }) => (
          <EncryptedReasoningControl
            label="Alias encrypted reasoning"
            value={field.value}
            onChange={field.onChange}
            view={alias}
            scope="alias"
            error={fieldState.error?.message}
            disabled={disabled}
          />
        )}
      />
      <p className="text-xs text-slate-500">
        Unchanged policy is omitted on save to preserve its exact saved declaration. Inherit removes an existing alias
        override; safe reset writes an explicit empty block. An explicit alias policy wins over every selected target
        and keeps a stripped body through normal failover. Without one, each target uses its own policy against the
        original admitted body. Recovery allows at most one extra attempt across the entire request, not one per target.
        Root policy remains static configuration.
      </p>
    </>
  );
}

function AliasForm({
  initial,
  submitLabel,
  onSubmit,
  onCancel,
  lockName,
  alias,
  inventory,
  workspaceContext,
}: {
  initial: AliasFormValues;
  submitLabel: string;
  onSubmit: (body: Record<string, unknown>) => Promise<void>;
  onCancel?: () => void;
  lockName: boolean;
  alias?: AdminAlias;
  inventory: AdminProvider[];
  workspaceContext: WorkspaceContext;
}) {
  const form = useForm<AliasFormValues>({ resolver: zodResolver(aliasFormSchema), defaultValues: initial });
  const lifecycle = useStateSnapshot(authLifecycle);
  const contextChanged =
    workspaceContext.generation !== lifecycle.generation || workspaceContext.session !== lifecycle.session;
  const scopedInventory = inventory.filter((p) => resourceInWorkspace(p, workspaceContext.workspaceId));
  const submitMutation = useMutation({
    mutationFn: async (values: AliasFormValues) => {
      assertWorkspaceContext(workspaceContext);
      const body = buildAliasBody(values, lockName ? initial : undefined);
      const targets =
        'targets' in body
          ? (body.targets as Array<{ provider: string; model: string }>)
          : (body.providers as string[]).map((provider) => ({ provider, model: body.model as string }));
      if (
        targets.some(
          (target) =>
            !scopedInventory.some(
              (provider) =>
                provider.enabled &&
                provider.name === target.provider &&
                provider.models.some((model) => model.name === target.model),
            ),
        )
      ) {
        throw new Error('Choose targets from the selected workspace inventory.');
      }
      return onSubmit(body);
    },
  });

  return (
    <FormProvider {...form}>
      <form className="grid gap-4" onSubmit={form.handleSubmit((values) => submitMutation.mutate(values))}>
        {contextChanged && (
          <Alert variant="danger">
            <AlertDescription>
              Workspace or account changed. Reopen this form in the selected workspace.
            </AlertDescription>
          </Alert>
        )}
        {submitMutation.error && (
          <Alert variant="danger">
            <AlertDescription>{errorMessage(submitMutation.error)}</AlertDescription>
          </Alert>
        )}

        <AliasFields
          lockName={lockName}
          alias={alias}
          disabled={submitMutation.isPending || contextChanged}
          inventory={scopedInventory}
        />

        <div className="flex gap-2">
          <Button variant="primary" type="submit" disabled={submitMutation.isPending || contextChanged}>
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

const CreateAliasDialog = createTypedDialog<
  { inventory: AdminProvider[]; workspaceContext: WorkspaceContext },
  boolean
>(({ open, args, onClose }) => (
  <Dialog open={open} onOpenChange={(o) => !o && onClose(false)}>
    <DialogContent className="max-h-[calc(100dvh-2rem)] overflow-auto sm:max-w-2xl">
      <DialogHeader>
        <DialogTitle>Create alias</DialogTitle>

        <DialogDescription>
          Aliases are virtual models: clients call <span className="font-mono">alias/&lt;name&gt;</span>
          and the proxy routes each request to one of the targets. The new alias belongs to the currently selected
          workspace.
        </DialogDescription>
      </DialogHeader>

      <AliasForm
        initial={emptyAliasForm()}
        submitLabel="Create alias"
        lockName={false}
        inventory={args.inventory}
        workspaceContext={args.workspaceContext}
        onSubmit={async (body) => {
          await createAdminAlias(body, args.workspaceContext);
          onClose(true);
        }}
        onCancel={() => onClose(false)}
      />
    </DialogContent>
  </Dialog>
));

const EditAliasDialog = createTypedDialog<
  { alias: AdminAlias; inventory: AdminProvider[]; workspaceContext: WorkspaceContext },
  boolean
>(({ open, args, onClose }) => (
  <Dialog open={open} onOpenChange={(o) => !o && onClose(false)}>
    <DialogContent className="max-h-[calc(100dvh-2rem)] overflow-auto sm:max-w-2xl">
      <DialogHeader>
        <DialogTitle>Edit alias alias/{args.alias.name}</DialogTitle>
        <DialogDescription>Saves update managed configuration; activation failures are reported.</DialogDescription>
      </DialogHeader>

      <AliasForm
        key={`edit-${args.alias.name}`}
        initial={aliasToFormValues(args.alias)}
        alias={args.alias}
        submitLabel="Save changes"
        lockName
        inventory={args.inventory}
        workspaceContext={args.workspaceContext}
        onSubmit={async (body) => {
          await updateAdminAlias(args.alias.name, body, args.workspaceContext);
          onClose(true);
        }}
        onCancel={() => onClose(false)}
      />
    </DialogContent>
  </Dialog>
));

function ManagedAliases() {
  const selection = useCurrentWorkspace();
  const aliases = useAdminAliases(true);
  const providers = useAdminProviders(true);
  const queryClient = useQueryClient();
  const { openDialog } = useDialog();
  const confirm = useConfirm();
  const [error, setError] = useState<string | null>(null);

  const rows = (aliases.data ?? []).filter(
    (a) => selection.context && resourceInWorkspace(a, selection.context.workspaceId),
  );
  const inventory = (providers.data ?? []).filter(
    (p) => selection.context && resourceInWorkspace(p, selection.context.workspaceId),
  );
  const refresh = () => queryClient.invalidateQueries({ queryKey: ['admin', 'aliases'] });

  const deleteMutation = useMutation({
    mutationFn: ({ name, context }: { name: string; context: WorkspaceContext }) => deleteAdminAlias(name, context),
    onSuccess: refresh,
    onError: (err) => setError(errorMessage(err)),
  });

  const onCreate = async () => {
    try {
      const context = selection.context;
      assertWorkspaceContext(context);
      if (!selection.canManage) return;
      const created = await openDialog(CreateAliasDialog, { inventory, workspaceContext: context });
      if (created) {
        setError(null);
        await refresh();
      }
    } catch {
      return;
    }
  };

  const openEdit = async (alias: AdminAlias) => {
    try {
      const context = selection.context;
      assertWorkspaceContext(context);
      if (!selection.canManage || !resourceInWorkspace(alias, context.workspaceId)) return;
      const saved = await openDialog(EditAliasDialog, { alias, inventory, workspaceContext: context });
      if (saved) {
        setError(null);
        await refresh();
      }
    } catch {
      return;
    }
  };

  const askDelete = async (alias: string) => {
    const context = selection.context;
    if (!context || !selection.canManage) return;
    const ok = await confirm({
      title: 'Delete alias',
      description: `Delete alias "${alias}"?`,
      confirmText: 'Delete',
    });
    if (ok) {
      setError(null);
      deleteMutation.mutate({ name: alias, context });
    }
  };

  const busy = deleteMutation.isPending;

  const columns: ManagementColumnDef<AdminAlias>[] = useMemo(
    () => [
      {
        accessorKey: 'name',
        header: 'Alias',
        cell: ({ row }) => <span className="font-mono font-medium">alias/{row.original.name}</span>,
      },
      {
        accessorKey: 'algorithm',
        header: 'Algorithm',
        cell: ({ row }) => <span className="text-slate-500">{row.original.algorithm}</span>,
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
        id: 'targets',
        header: 'Targets',
        accessorFn: (row) => row.targets.map((t) => `${t.provider}/${t.model}`).join(', '),
        cell: ({ row }) => (
          <span className="font-mono text-xs text-slate-500">
            {row.original.targets.map((t) => `${t.provider}/${t.model}`).join(', ')}
          </span>
        ),
      },
      {
        id: 'reasoning',
        header: 'Encrypted reasoning',
        enableGlobalFilter: false,
        cell: ({ row }) => (
          <details className="grid gap-2">
            <summary className="cursor-pointer text-sm">Policy (read-only)</summary>
            <ReasoningPolicyReadOnly label="Alias policy" view={row.original} scope="alias" />
          </details>
        ),
      },
      {
        id: 'actions',
        header: '',
        enableSorting: false,
        enableGlobalFilter: false,
        cell: ({ row }) => {
          const a = row.original;
          return a.source === 'database' && selection.canManage ? (
            <ActionMenu
              trigger={
                <Button variant="secondary" appearance="outline-filled" size="sm" aria-label={`Actions for ${a.name}`}>
                  Actions
                </Button>
              }
              items={[
                { label: 'Edit', onSelect: () => openEdit(a), disabled: busy || !providers.isSuccess },
                { label: 'Delete', onSelect: () => askDelete(a.name), disabled: busy, variant: 'destructive' },
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
    [
      busy,
      selection.canManage,
      selection.context?.workspaceId,
      selection.context?.generation,
      providers.data,
      providers.isSuccess,
    ],
  );

  if (!selection.context) return <WorkspaceAccessNotice pending={selection.pending} error={selection.error} />;

  return (
    <>
      {aliases.error && (
        <Alert variant="danger">
          <AlertDescription>Unable to load aliases for this workspace.</AlertDescription>
        </Alert>
      )}
      {error && (
        <Alert variant="danger">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}

      <Card>
        <CardHeader className="flex flex-row items-center justify-between gap-2">
          <CardTitle>Aliases ({rows.length})</CardTitle>

          <Button
            variant="primary"
            type="button"
            onClick={onCreate}
            disabled={
              !selection.canManage ||
              !aliases.isSuccess ||
              !providers.isSuccess ||
              !inventory.some((p) => p.enabled && p.models.length > 0)
            }
          >
            Create alias
          </Button>
        </CardHeader>

        <CardContent>
          <DataTable
            columns={columns}
            data={rows}
            filterPlaceholder="Filter aliases..."
            getRowId={(row) => row.name}
            emptyText={aliases.error ? 'Alias inventory unavailable.' : 'No aliases in this workspace.'}
          />
          {selection.canManage && providers.isSuccess && !inventory.some((p) => p.enabled && p.models.length > 0) && (
            <p className="text-sm text-slate-500">
              Add your first provider and models before creating an alias.{' '}
              <Link to="/providers" className="underline">
                Create a provider
              </Link>
            </p>
          )}

          {aliases.isLoading && (
            <div className="text-sm text-slate-500">
              <Spinner size="small">Loading aliases...</Spinner>
            </div>
          )}
        </CardContent>
      </Card>
    </>
  );
}

export function AliasesPage() {
  const status = useAdminStatus();
  const session = useAdminSession();
  const multi = status.data?.multi_tenancy_enabled === true;
  const signedIn = !!session.accessToken;

  return (
    <div className="grid w-full gap-6 p-6">
      {multi && !signedIn && (
        <Alert>
          <AlertDescription>
            Multi-tenancy is enabled.
            <Link className="underline" to="/login">
              Sign in
            </Link>
            to add or delete database aliases.
          </AlertDescription>
        </Alert>
      )}
      {multi && signedIn ? <ManagedAliases /> : <SnapshotAliases />}
    </div>
  );
}
