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
import { FormProvider, useForm } from 'react-hook-form';
import { Link } from 'react-router';
import { z } from 'zod';
import { DataTable, type ManagementColumnDef } from '../components/data-table';
import { useConfirm } from '../components/dialogs';
import { SourceBadge } from '../components/source-badge';
import { OrgBadge } from '../components/org-select';
import { useAdminAliases, useAdminSession, useAdminStatus, useSnapshot } from '../hooks';
import { createAdminAlias, deleteAdminAlias, updateAdminAlias } from '../services/admin';
import { errorMessage } from '../services/dashboard';
import type { AdminAlias } from '../types';

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

function parseTargetLines(raw: string) {
  return raw
    .split('\n')
    .map((t) => t.trim())
    .filter(Boolean)
    .map((t) => {
      const [provider, model] = t.split('/');
      return { provider: (provider ?? '').trim(), model: (model ?? '').trim() };
    });
}

const aliasFormSchema = z.object({
  name: z.string().trim().min(1, 'Enter an alias name.'),
  algorithm: z.enum(['round_robin', 'least_connections']).optional(),
  retryCodes: z.string().optional(),
  targets: z.string().optional(),
  shorthandProviders: z.string().optional(),
  shorthandModel: z.string().optional(),
  useAffinity: z.boolean().optional(),
  affinityHeaders: z.string().optional(),
  useReasoning: z.boolean().optional(),
  reasoningPassthrough: z.boolean().optional(),
  reasoningMismatch: z.string().optional(),
  reasoningMessages: z.string().optional(),
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
  useReasoning: false,
  reasoningPassthrough: true,
  reasoningMismatch: '',
  reasoningMessages: '',
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
    useReasoning: a.encrypted_reasoning != null,
    reasoningPassthrough: a.encrypted_reasoning?.passthrough ?? true,
    reasoningMismatch: a.encrypted_reasoning?.on_caller_mismatch ?? '',
    reasoningMessages: (a.encrypted_reasoning?.match_messages ?? []).join('\n'),
  };
}

function buildAliasBody(v: AliasFormValues): Record<string, unknown> {
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
  const shorthand = (v.shorthandProviders ?? '')
    .split(',')
    .map((p) => p.trim())
    .filter(Boolean);
  if (shorthand.length > 0 || (v.shorthandModel ?? '').trim() !== '') {
    body.providers = shorthand;
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
  if (v.useReasoning) {
    const reasoning: Record<string, unknown> = { passthrough: v.reasoningPassthrough };
    if ((v.reasoningMismatch ?? '').trim() !== '') reasoning.on_caller_mismatch = (v.reasoningMismatch ?? '').trim();
    const messages = (v.reasoningMessages ?? '')
      .split('\n')
      .map((m) => m.trim())
      .filter(Boolean);
    if (messages.length > 0) reasoning.match_messages = messages;
    body.encrypted_reasoning = reasoning;
  }
  return body;
}

function AliasFields({ lockName }: { lockName: boolean }) {
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
      <HookFormTextarea<AliasFormValues>
        name="targets"
        label="Targets (one provider/model per line)"
        placeholder={'db-openai/gpt-4o-mini\nbackup/gpt-4o-mini'}
        rows={3}
      />
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
      <HookFormCheckbox<AliasFormValues> name="useReasoning" label="Encrypted reasoning handling" />
      <HookFormCheckbox<AliasFormValues> name="reasoningPassthrough" label="Passthrough" />
      <HookFormTextInput<AliasFormValues>
        name="reasoningMismatch"
        label="On caller mismatch (fail or strip_and_retry)"
        placeholder="fail"
      />
      <HookFormTextarea<AliasFormValues>
        name="reasoningMessages"
        label="Match messages (one per line, optional)"
        rows={3}
      />
    </>
  );
}

function AliasForm({
  initial,
  submitLabel,
  onSubmit,
  onCancel,
  lockName,
}: {
  initial: AliasFormValues;
  submitLabel: string;
  onSubmit: (body: Record<string, unknown>) => Promise<void>;
  onCancel?: () => void;
  lockName: boolean;
}) {
  const form = useForm<AliasFormValues>({ resolver: zodResolver(aliasFormSchema), defaultValues: initial });
  const submitMutation = useMutation({
    mutationFn: async (values: AliasFormValues) => onSubmit(buildAliasBody(values)),
  });

  return (
    <FormProvider {...form}>
      <form className="grid gap-4" onSubmit={form.handleSubmit((values) => submitMutation.mutate(values))}>
        {submitMutation.error && (
          <Alert variant="danger">
            <AlertDescription>{errorMessage(submitMutation.error)}</AlertDescription>
          </Alert>
        )}
        <AliasFields lockName={lockName} />
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

const CreateAliasDialog = createTypedDialog<object, boolean>(({ open, onClose }) => (
  <Dialog open={open} onOpenChange={(o) => !o && onClose(false)}>
    <DialogContent className="max-h-[calc(100dvh-2rem)] overflow-auto sm:max-w-2xl">
      <DialogHeader>
        <DialogTitle>Create alias</DialogTitle>
        <DialogDescription>
          Aliases are virtual models: clients call <span className="font-mono">alias/&lt;name&gt;</span> and the proxy
          routes each request to one of the targets. The new alias belongs to the currently selected organization.
        </DialogDescription>
      </DialogHeader>
      <AliasForm
        initial={emptyAliasForm()}
        submitLabel="Create alias"
        lockName={false}
        onSubmit={async (body) => {
          await createAdminAlias(body);
          onClose(true);
        }}
        onCancel={() => onClose(false)}
      />
    </DialogContent>
  </Dialog>
));

const EditAliasDialog = createTypedDialog<{ alias: AdminAlias }, boolean>(({ open, args, onClose }) => (
  <Dialog open={open} onOpenChange={(o) => !o && onClose(false)}>
    <DialogContent className="max-h-[calc(100dvh-2rem)] overflow-auto sm:max-w-2xl">
      <DialogHeader>
        <DialogTitle>Edit alias alias/{args.alias.name}</DialogTitle>
        <DialogDescription>Saving applies immediately.</DialogDescription>
      </DialogHeader>
      <AliasForm
        key={`edit-${args.alias.name}`}
        initial={aliasToFormValues(args.alias)}
        submitLabel="Save changes"
        lockName
        onSubmit={async (body) => {
          await updateAdminAlias(args.alias.name, body);
          onClose(true);
        }}
        onCancel={() => onClose(false)}
      />
    </DialogContent>
  </Dialog>
));

function ManagedAliases() {
  const aliases = useAdminAliases(true);
  const queryClient = useQueryClient();
  const { openDialog } = useDialog();
  const confirm = useConfirm();
  const [error, setError] = useState<string | null>(null);

  const rows = aliases.data ?? [];
  const refresh = () => queryClient.invalidateQueries({ queryKey: ['admin', 'aliases'] });

  const deleteMutation = useMutation({
    mutationFn: deleteAdminAlias,
    onSuccess: refresh,
    onError: (err) => setError(errorMessage(err)),
  });

  const onCreate = async () => {
    try {
      const created = await openDialog(CreateAliasDialog, {});
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
      const saved = await openDialog(EditAliasDialog, { alias });
      if (saved) {
        setError(null);
        await refresh();
      }
    } catch {
      return;
    }
  };

  const askDelete = async (alias: string) => {
    const ok = await confirm({
      title: 'Delete alias',
      description: `Delete alias "${alias}"?`,
      confirmText: 'Delete',
    });
    if (ok) {
      setError(null);
      deleteMutation.mutate(alias);
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
        id: 'org',
        header: 'Org',
        accessorFn: (row) => row.org_name || row.org_id,
        cell: ({ row }) => <OrgBadge orgId={row.original.org_id} orgName={row.original.org_name} />,
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
        id: 'actions',
        header: '',
        enableSorting: false,
        enableGlobalFilter: false,
        cell: ({ row }) => {
          const a = row.original;
          return a.source === 'database' ? (
            <ActionMenu
              items={[
                { label: 'Edit', onSelect: () => openEdit(a), disabled: busy },
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
          <CardTitle>Aliases ({rows.length})</CardTitle>
          <Button variant="primary" type="button" onClick={onCreate}>
            Create alias
          </Button>
        </CardHeader>
        <CardContent>
          <DataTable
            columns={columns}
            data={rows}
            filterPlaceholder="Filter aliases..."
            getRowId={(row) => row.name}
            emptyText="No aliases."
          />
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
            Multi-tenancy is enabled.{' '}
            <Link className="underline" to="/login">
              Sign in
            </Link>{' '}
            to add or delete database aliases.
          </AlertDescription>
        </Alert>
      )}
      {multi && signedIn ? <ManagedAliases /> : <SnapshotAliases />}
    </div>
  );
}
