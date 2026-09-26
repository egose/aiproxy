import { zodResolver } from '@hookform/resolvers/zod';
import { HookFormNativeSelect } from '@egose/shadcn-theme/components/form/hook-native-select';
import { HookFormTagPicker } from '@egose/shadcn-theme/components/form/hook-tag-picker';
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
import { Label } from '@egose/shadcn-theme/components/ui/label';
import { Spinner } from '@egose/shadcn-theme/components/ui/spinner';
import { Tabs, TabsList, TabsTrigger } from '@egose/shadcn-theme/components/ui/tabs';
import { ActionMenu } from '@egose/shadcn-theme/components/widgets/action-menu';
import { useDialog, createTypedDialog } from '@egose/shadcn-theme/components/widgets/dialog-manager';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useEffect, useMemo, useState } from 'react';
import { FormProvider, useForm } from 'react-hook-form';
import { z } from 'zod';
import { DataTable, type ManagementColumnDef } from '../components/data-table';
import { showSecret, useConfirm } from '../components/dialogs';
import { KeyQuotaLine } from '../components/quota';
import { SourceBadge } from '../components/source-badge';
import { OrgBadge, OrgSelect } from '../components/org-select';
import { useAdminKeys, useAdminOrg, useAdminOrgs, useOrgMembers, useOrgTeams } from '../hooks';
import { createAdminKey, deleteAdminKey, revokeAdminKey, rotateAdminKey, updateAdminKey } from '../services/admin';
import { errorMessage } from '../services/dashboard';
import type { AdminKey } from '../types';

const issueKeySchema = z.object({
  org_id: z.string().optional(),
  owner_type: z.enum(['personal', 'team']).optional(),
  team_id: z.string().optional(),
  name: z.string().trim().min(1, 'Enter a key name.'),
  tenant: z.string().optional(),
  allowed_models: z.array(z.string()).optional().default([]),
  description: z.string().optional(),
  expires_at: z.string().optional(),
});

type IssueKeyForm = z.infer<typeof issueKeySchema>;

const editKeySchema = z.object({
  description: z.string().optional(),
  expires_at: z.string().optional(),
  tenant: z.string().optional(),
  allowed_models: z.array(z.string()).optional().default([]),
});

type EditKeyForm = z.infer<typeof editKeySchema>;

const EditKeyDialog = createTypedDialog<{ key: AdminKey }, boolean>(({ open, args, onClose }) => {
  const key = args.key;
  const members = useOrgMembers(key.org_id || null, true);
  const teams = useOrgTeams(key.org_id || null, true);
  const [userIds, setUserIds] = useState<string[]>(key.user_ids ?? []);
  const [teamIds, setTeamIds] = useState<string[]>(key.team_ids ?? []);
  const [error, setError] = useState<string | null>(null);

  const form = useForm<EditKeyForm>({
    resolver: zodResolver(editKeySchema),
    defaultValues: {
      description: key.description ?? '',
      expires_at: key.expires_at ?? '',
      tenant: key.tenant ?? '',
      allowed_models: key.allowed_models ?? [],
    },
  });

  const toggle = (list: string[], id: string, set: (v: string[]) => void) => {
    set(list.includes(id) ? list.filter((v) => v !== id) : [...list, id]);
  };

  const saveMutation = useMutation({
    mutationFn: async (values: EditKeyForm) =>
      updateAdminKey(key.id, {
        description: (values.description ?? '').trim(),
        expires_at: (values.expires_at ?? '').trim(),
        tenant: (values.tenant ?? '').trim(),
        allowed_models: values.allowed_models ?? [],
        user_ids: userIds,
        team_ids: teamIds,
      }),
    onSuccess: () => onClose(true),
    onError: (err) => setError(errorMessage(err)),
  });

  return (
    <Dialog open={open} onOpenChange={(o) => !o && onClose(false)}>
      <DialogContent className="max-h-[calc(100dvh-2rem)] overflow-auto sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>Edit key {key.name}</DialogTitle>
        </DialogHeader>
        <FormProvider {...form}>
          <form className="grid gap-4" onSubmit={form.handleSubmit((values) => saveMutation.mutate(values))}>
            <HookFormTextInput<EditKeyForm> name="description" label="Description (optional)" />
            <HookFormTextInput<EditKeyForm>
              name="expires_at"
              label="Expiry (optional, RFC3339, empty = never)"
              placeholder="2026-12-31T00:00:00Z"
            />
            <HookFormTextInput<EditKeyForm> name="tenant" label="Tenant (optional)" />
            <HookFormTagPicker<EditKeyForm>
              name="allowed_models"
              label="Allowed models (empty = all)"
              placeholder="Type a model and press Enter"
            />
            <div className="grid gap-2">
              <Label>Visible to users (optional, empty = org admins only)</Label>
              {(members.data ?? []).map((m) => (
                <label key={m.user_id} className="flex items-center gap-2 text-sm">
                  <Checkbox
                    checked={userIds.includes(m.user_id)}
                    onCheckedChange={() => toggle(userIds, m.user_id, setUserIds)}
                  />
                  <span className="font-mono">{m.email || m.user_id}</span>
                </label>
              ))}
              {members.isLoading && (
                <div className="text-sm text-slate-500">
                  <Spinner size="small">Loading members...</Spinner>
                </div>
              )}
            </div>
            <div className="grid gap-2">
              <Label>Visible to teams (optional)</Label>
              {(teams.data ?? []).map((t) => (
                <label key={t.id} className="flex items-center gap-2 text-sm">
                  <Checkbox
                    checked={teamIds.includes(t.id)}
                    onCheckedChange={() => toggle(teamIds, t.id, setTeamIds)}
                  />
                  <span className="font-mono">{t.name}</span>
                </label>
              ))}
              {teams.isLoading && (
                <div className="text-sm text-slate-500">
                  <Spinner size="small">Loading teams...</Spinner>
                </div>
              )}
            </div>
            {error && (
              <Alert variant="danger">
                <AlertDescription>{error}</AlertDescription>
              </Alert>
            )}
            <DialogFooter>
              <Button appearance="outline" type="button" onClick={() => onClose(false)}>
                Cancel
              </Button>
              <Button variant="primary" type="submit" disabled={saveMutation.isPending}>
                {saveMutation.isPending ? 'Saving...' : 'Save changes'}
              </Button>
            </DialogFooter>
          </form>
        </FormProvider>
      </DialogContent>
    </Dialog>
  );
});

const IssueKeyDialog = createTypedDialog<{ fixedOrgId?: string; canAdminOrg?: boolean }, boolean>(
  ({ open, args, onClose }) => {
    const { openDialog } = useDialog();
    const orgs = useAdminOrgs(true);
    const adminOrg = useAdminOrg();
    const [error, setError] = useState<string | null>(null);
    const form = useForm<IssueKeyForm>({
      resolver: zodResolver(issueKeySchema),
      defaultValues: {
        org_id: '',
        owner_type: 'personal',
        team_id: '',
        name: '',
        tenant: '',
        allowed_models: [],
        description: '',
        expires_at: '',
      },
    });
    const effectiveOrgId = args.fixedOrgId ?? adminOrg.orgId;
    useEffect(() => {
      form.setValue('org_id', effectiveOrgId);
    }, [effectiveOrgId, form]);
    const manageableTeams = (useOrgTeams(effectiveOrgId || null, true).data ?? []).filter(
      (t) => args.canAdminOrg === true || t.my_role === 'admin',
    );

    const issueMutation = useMutation({
      mutationFn: async (values: IssueKeyForm) => {
        const body: Record<string, unknown> = {
          name: values.name.trim(),
          tenant: (values.tenant ?? '').trim(),
          allowed_models: values.allowed_models ?? [],
          description: (values.description ?? '').trim(),
          expires_at: (values.expires_at ?? '').trim(),
        };
        const orgId = args.fixedOrgId ?? values.org_id ?? '';
        if (orgId !== '') body.org_id = orgId;
        if ((values.owner_type ?? 'personal') === 'team') {
          body.owner_type = 'team';
          body.owner_id = values.team_id ?? '';
        } else {
          body.owner_type = 'user';
        }
        return createAdminKey(body);
      },
      onSuccess: async ({ token }) => {
        setError(null);
        onClose(true);
        await showSecret(openDialog, {
          title: 'New token',
          description: 'Copy it now. It is shown only once.',
          secret: token,
        });
      },
      onError: (err) => setError(errorMessage(err)),
    });

    const orgId = form.watch('org_id');

    return (
      <Dialog open={open} onOpenChange={(o) => !o && onClose(false)}>
        <DialogContent className="max-h-[calc(100dvh-2rem)] overflow-auto sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>Create API key</DialogTitle>
          </DialogHeader>
          <FormProvider {...form}>
            <form className="grid gap-4" onSubmit={form.handleSubmit((values) => issueMutation.mutate(values))}>
              {error && (
                <Alert variant="danger">
                  <AlertDescription>{error}</AlertDescription>
                </Alert>
              )}
              {args.fixedOrgId === undefined && (orgs.data ?? []).length >= 2 && (
                <OrgSelect
                  orgs={orgs.data}
                  value={orgId ?? ''}
                  onChange={(v) => form.setValue('org_id', v)}
                  id="ak-org"
                />
              )}
              <HookFormNativeSelect<IssueKeyForm>
                name="owner_type"
                label="Owner"
                data={[
                  { label: 'Personal (just me, no team)', value: 'personal' },
                  ...(manageableTeams.length > 0 ? [{ label: 'Team key', value: 'team' }] : []),
                ]}
              />
              {form.watch('owner_type') === 'team' && (
                <HookFormNativeSelect<IssueKeyForm>
                  name="team_id"
                  label="Team"
                  data={[
                    { label: 'Select team...', value: '' },
                    ...manageableTeams.map((t) => ({ label: t.name, value: t.id })),
                  ]}
                />
              )}
              <HookFormTextInput<IssueKeyForm> name="name" label="Name" placeholder="ci-key" />
              <HookFormTextInput<IssueKeyForm> name="description" label="Description (optional)" />
              <HookFormTextInput<IssueKeyForm>
                name="expires_at"
                label="Expiry (optional, RFC3339, empty = never)"
                placeholder="2026-12-31T00:00:00Z"
              />
              <HookFormTextInput<IssueKeyForm> name="tenant" label="Tenant (optional)" placeholder="team-a" />
              <HookFormTagPicker<IssueKeyForm>
                name="allowed_models"
                label="Allowed models (empty = all)"
                placeholder="Type a model and press Enter"
              />
              <DialogFooter>
                <Button appearance="outline" type="button" onClick={() => onClose(false)}>
                  Cancel
                </Button>
                <Button variant="primary" type="submit" disabled={issueMutation.isPending}>
                  {issueMutation.isPending ? 'Issuing...' : 'Create API key'}
                </Button>
              </DialogFooter>
            </form>
          </FormProvider>
        </DialogContent>
      </Dialog>
    );
  },
);

export function AdminKeysPage({ fixedOrgId, canAdminOrg }: { fixedOrgId?: string; canAdminOrg?: boolean }) {
  const keys = useAdminKeys();
  const queryClient = useQueryClient();
  const { openDialog } = useDialog();
  const confirm = useConfirm();
  const [tab, setTab] = useState<'inbound' | 'upstream'>('inbound');
  const [error, setError] = useState<string | null>(null);

  const invalidate = () => queryClient.invalidateQueries({ queryKey: ['admin', 'keys'] });

  const rowMutation = useMutation({
    mutationFn: async (op: { kind: 'rotate' | 'revoke' | 'delete'; id: string }) => {
      if (op.kind === 'rotate') return { kind: op.kind, token: await rotateAdminKey(op.id) } as const;
      if (op.kind === 'revoke') {
        await revokeAdminKey(op.id);
        return { kind: op.kind } as const;
      }
      await deleteAdminKey(op.id);
      return { kind: op.kind } as const;
    },
    onSuccess: async (res) => {
      setError(null);
      await invalidate();
      if (res.kind === 'rotate') {
        await showSecret(openDialog, {
          title: 'Rotated token',
          description: 'Copy it now. It is shown only once.',
          secret: res.token,
        });
      }
    },
    onError: (err) => setError(errorMessage(err)),
  });

  const askDelete = async (name: string, id: string) => {
    const ok = await confirm({
      title: 'Delete key',
      description: `Delete inbound key "${name}"? This cannot be undone.`,
      confirmText: 'Delete',
    });
    if (ok) rowMutation.mutate({ kind: 'delete', id });
  };

  const openEdit = async (key: AdminKey) => {
    try {
      const saved = await openDialog(EditKeyDialog, { key });
      if (saved) {
        setError(null);
        await invalidate();
      }
    } catch {
      return;
    }
  };

  const rows = keys.data ?? [];
  const busy = rowMutation.isPending;

  const openCreate = async () => {
    try {
      const created = await openDialog(IssueKeyDialog, { fixedOrgId, canAdminOrg });
      if (created) {
        setError(null);
        await invalidate();
      }
    } catch {
      return;
    }
  };

  const columns: ManagementColumnDef<AdminKey>[] = useMemo(
    () => [
      {
        accessorKey: 'name',
        header: 'Key',
        cell: ({ row }) => <span className="font-mono font-medium">{row.original.name}</span>,
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
        id: 'owner',
        header: 'Owner',
        accessorFn: (row) => (row.owner_name ? `${row.owner_team_id ? 'team' : 'personal'}:${row.owner_name}` : ''),
        cell: ({ row }) => {
          const k = row.original;
          return k.owner_name ? (
            <span
              className="text-slate-500"
              title={k.owner_team_id ? `Team key owned by ${k.owner_name}` : `Personal key owned by ${k.owner_name}`}
            >
              {k.owner_team_id ? `team:${k.owner_name}` : `personal:${k.owner_name}`}
            </span>
          ) : (
            <span className="text-slate-400">—</span>
          );
        },
      },
      {
        id: 'details',
        header: 'Details',
        accessorFn: (row) =>
          [row.description, row.expires_at, row.tenant, row.token_prefix, row.allowed_models.join(',')]
            .filter(Boolean)
            .join(' '),
        cell: ({ row }) => {
          const k = row.original;
          return (
            <span className="grid gap-0.5 text-xs">
              {k.description && <span className="text-slate-500">{k.description}</span>}
              {k.expires_at && <span className="font-mono text-slate-500">expires {k.expires_at}</span>}
              {k.tenant && <span className="text-slate-500">tenant={k.tenant}</span>}
              {k.token_prefix && <span className="font-mono text-slate-500">{k.token_prefix}…</span>}
              {k.allowed_models.length > 0 && (
                <span className="font-mono text-slate-500">{k.allowed_models.join(', ')}</span>
              )}
            </span>
          );
        },
      },
      {
        id: 'quota',
        header: 'Quota',
        enableSorting: false,
        enableGlobalFilter: false,
        cell: ({ row }) => <KeyQuotaLine usage={row.original.usage} quota={row.original.quota} />,
      },
      {
        id: 'status',
        header: 'Status',
        accessorFn: (row) => (row.enabled ? 'active' : 'revoked'),
        cell: ({ row }) => (!row.original.enabled ? <span className="text-xs text-red-500">revoked</span> : null),
      },
      {
        id: 'actions',
        header: '',
        enableSorting: false,
        enableGlobalFilter: false,
        cell: ({ row }) => {
          const k = row.original;
          if (k.source === 'database' && k.id && k.can_manage) {
            return (
              <ActionMenu
                items={[
                  { label: 'Edit', onSelect: () => openEdit(k), disabled: busy },
                  { label: 'Rotate', onSelect: () => rowMutation.mutate({ kind: 'rotate', id: k.id }), disabled: busy },
                  {
                    label: 'Revoke',
                    onSelect: () => rowMutation.mutate({ kind: 'revoke', id: k.id }),
                    disabled: busy,
                    variant: 'destructive',
                  },
                  { label: 'Delete', onSelect: () => askDelete(k.name, k.id), disabled: busy, variant: 'destructive' },
                ]}
              />
            );
          }
          if (k.source === 'config') {
            return (
              <span className="text-xs text-slate-400" title="Defined in the static config file">
                read-only
              </span>
            );
          }
          return null;
        },
      },
    ],
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [busy],
  );

  return (
    <div className="grid w-full gap-6 p-6">
      <Tabs value={tab} onValueChange={(v) => setTab(v as 'inbound' | 'upstream')}>
        <TabsList>
          <TabsTrigger value="inbound">Inbound client keys</TabsTrigger>
          <TabsTrigger value="upstream">Provider credentials</TabsTrigger>
        </TabsList>
      </Tabs>
      {error && (
        <Alert variant="danger">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}
      {tab === 'inbound' ? (
        <Card>
          <CardHeader className="flex flex-row items-center justify-between gap-2">
            <CardTitle>API keys ({rows.length})</CardTitle>
            <Button variant="primary" type="button" onClick={openCreate}>
              Create API key
            </Button>
          </CardHeader>
          <CardContent>
            <DataTable
              columns={columns}
              data={rows}
              filterPlaceholder="Filter API keys..."
              getRowId={(row) => `${row.source}-${row.name}`}
              emptyText="No API keys."
            />
            {keys.isLoading && (
              <div className="text-sm text-slate-500">
                <Spinner size="small">Loading keys...</Spinner>
              </div>
            )}
          </CardContent>
        </Card>
      ) : (
        <Card>
          <CardHeader>
            <CardTitle>Provider credentials</CardTitle>
          </CardHeader>
          <CardContent className="grid gap-2 text-sm text-slate-600">
            <p>
              Upstream provider credentials are managed per provider on the{' '}
              <a className="underline" href="/providers">
                providers page
              </a>{' '}
              (rotate upstream credential). Secrets are stored encrypted and never displayed.
            </p>
          </CardContent>
        </Card>
      )}
    </div>
  );
}
