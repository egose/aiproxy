import { zodResolver } from '@hookform/resolvers/zod';
import { HookFormNativeSelect } from '@egose/shadcn-theme/components/form/hook-native-select';
import { HookFormTextInput } from '@egose/shadcn-theme/components/form/hook-text-input';
import { Alert, AlertDescription } from '@egose/shadcn-theme/components/ui/alert';
import { Badge } from '@egose/shadcn-theme/components/ui/badge';
import { Button } from '@egose/shadcn-theme/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@egose/shadcn-theme/components/ui/card';
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@egose/shadcn-theme/components/ui/dialog';
import { NativeSelect, NativeSelectOption } from '@egose/shadcn-theme/components/ui/native-select';
import { createTypedDialog, useDialog } from '@egose/shadcn-theme/components/widgets/dialog-manager';
import { Spinner } from '@egose/shadcn-theme/components/ui/spinner';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useMemo, useState } from 'react';
import { FormProvider, useForm } from 'react-hook-form';
import { z } from 'zod';
import { DataTable, type ManagementColumnDef } from '../components/data-table';
import { showSecret, useConfirm } from '../components/dialogs';
import { QuotaEditor } from '../components/quota';
import { useAdminMe, useOrgMembers, useOrgTeams, useTeamMembers } from '../hooks';
import {
  addOrgMember,
  addTeamMember,
  createAdminInvite,
  createOrgTeam,
  deleteAdminOrg,
  deleteOrgTeam,
  removeOrgMember,
  removeTeamMember,
  setOrgMemberRole,
  setTeamMemberRole,
  updateAdminOrg,
} from '../services/admin';
import { errorMessage } from '../services/dashboard';
import type { AdminOrg, AdminOrgMember, AdminTeam } from '../types';

export function RoleBadge({ role }: { role: string }) {
  return (
    <Badge variant={role === 'admin' ? 'info' : 'secondary'} size="sm">
      {role}
    </Badge>
  );
}

const teamSchema = z.object({
  name: z.string().trim().min(1, 'Enter a team name.'),
  description: z.string().optional(),
});

type TeamForm = z.infer<typeof teamSchema>;

export const CreateTeamDialog = createTypedDialog<{ orgId: string }, boolean>(({ open, args, onClose }) => {
  const [error, setError] = useState<string | null>(null);
  const form = useForm<TeamForm>({
    resolver: zodResolver(teamSchema),
    defaultValues: { name: '', description: '' },
  });
  const createMutation = useMutation({
    mutationFn: async (values: TeamForm) =>
      createOrgTeam(args.orgId, { name: values.name.trim(), description: (values.description ?? '').trim() }),
    onSuccess: () => onClose(true),
    onError: (err) => setError(errorMessage(err)),
  });
  return (
    <Dialog open={open} onOpenChange={(o) => !o && onClose(false)}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Create team</DialogTitle>
        </DialogHeader>
        <FormProvider {...form}>
          <form className="grid gap-4" onSubmit={form.handleSubmit((values) => createMutation.mutate(values))}>
            {error && (
              <Alert variant="danger">
                <AlertDescription>{error}</AlertDescription>
              </Alert>
            )}
            <HookFormTextInput<TeamForm> name="name" label="Team name" placeholder="backend" />
            <HookFormTextInput<TeamForm> name="description" label="Description (optional)" />
            <DialogFooter>
              <Button appearance="outline" type="button" onClick={() => onClose(false)}>
                Cancel
              </Button>
              <Button variant="primary" type="submit" disabled={createMutation.isPending}>
                {createMutation.isPending ? 'Creating...' : 'Create team'}
              </Button>
            </DialogFooter>
          </form>
        </FormProvider>
      </DialogContent>
    </Dialog>
  );
});

const teamMemberSchema = z.object({
  email: z.string().trim().min(1, 'Enter an email.').email('Enter a valid email.'),
  role: z.enum(['admin', 'member']).optional(),
});

type TeamMemberForm = z.infer<typeof teamMemberSchema>;

const orgMemberSchema = z.object({
  email: z.string().trim().min(1, 'Enter an email.').email('Enter a valid email.'),
  role: z.enum(['admin', 'member']).optional(),
});

type OrgMemberForm = z.infer<typeof orgMemberSchema>;

const inviteSchema = z.object({
  email: z.string().trim().min(1, 'Enter an email.').email('Enter a valid email.'),
  role: z.enum(['admin', 'member']).optional(),
});

type InviteForm = z.infer<typeof inviteSchema>;

const displayNameSchema = z.object({
  display_name: z.string().optional(),
});

type DisplayNameForm = z.infer<typeof displayNameSchema>;

export function TeamSection({ org, canManage }: { org: AdminOrg; canManage: boolean }) {
  const teams = useOrgTeams(org.id, true);
  const queryClient = useQueryClient();
  const [openTeam, setOpenTeam] = useState<string | null>(null);
  const [openQuota, setOpenQuota] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const confirm = useConfirm();

  const invalidate = () => queryClient.invalidateQueries({ queryKey: ['admin', 'orgs', org.id, 'teams'] });

  const deleteMutation = useMutation({
    mutationFn: async (teamId: string) => deleteOrgTeam(org.id, teamId),
    onSuccess: invalidate,
    onError: (err) => setError(errorMessage(err)),
  });

  const askDelete = async (name: string, teamId: string) => {
    const ok = await confirm({ title: 'Delete team', description: `Delete team "${name}"?`, confirmText: 'Delete' });
    if (ok) deleteMutation.mutate(teamId);
  };

  const busy = deleteMutation.isPending;

  const columns: ManagementColumnDef<AdminTeam>[] = useMemo(
    () => [
      {
        accessorKey: 'name',
        header: 'Team',
        cell: ({ row }) => {
          const t = row.original;
          return (
            <span className="flex flex-wrap items-center gap-2 text-sm">
              <button
                type="button"
                className="font-mono font-medium underline"
                onClick={() => setOpenTeam(openTeam === t.id ? null : t.id)}
              >
                {t.name}
              </button>
              {t.my_role && <RoleBadge role={t.my_role} />}
              {t.description && <span className="text-slate-500">{t.description}</span>}
            </span>
          );
        },
      },
      {
        accessorKey: 'members',
        header: 'Members',
        cell: ({ row }) => <span className="text-slate-500">{row.original.members}</span>,
      },
      {
        id: 'actions',
        header: '',
        enableSorting: false,
        enableGlobalFilter: false,
        cell: ({ row }) => {
          const t = row.original;
          const manageTeam = canManage || t.my_role === 'admin';
          return (
            <span className="flex flex-wrap items-center gap-2">
              {manageTeam && (
                <button
                  type="button"
                  className="text-xs text-slate-500 underline"
                  onClick={() => setOpenQuota(openQuota === t.id ? null : t.id)}
                >
                  Quotas
                </button>
              )}
              {canManage && (
                <Button appearance="outline" type="button" disabled={busy} onClick={() => askDelete(t.name, t.id)}>
                  Delete team
                </Button>
              )}
            </span>
          );
        },
      },
    ],
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [busy, canManage, openTeam, openQuota],
  );

  const expandedIds = useMemo(() => {
    const ids = new Set<string>();
    if (openTeam) ids.add(openTeam);
    if (openQuota) ids.add(openQuota);
    return ids;
  }, [openTeam, openQuota]);

  return (
    <div className="grid gap-2">
      {error && (
        <Alert variant="danger">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}
      <DataTable
        columns={columns}
        data={teams.data ?? []}
        filterPlaceholder="Filter teams..."
        getRowId={(row) => row.id}
        emptyText="No teams."
        expandedIds={expandedIds}
        renderExpanded={(t) => {
          const manageTeam = canManage || t.my_role === 'admin';
          return (
            <div className="grid gap-2 pl-2">
              {openTeam === t.id && <TeamMembers orgId={org.id} teamId={t.id} canManage={manageTeam} />}
              {openQuota === t.id && manageTeam && (
                <QuotaEditor
                  key={t.id}
                  orgId={org.id}
                  scope="teams"
                  scopeId={t.id}
                  canEditPolicy={canManage}
                  canEditEffective={manageTeam}
                />
              )}
            </div>
          );
        }}
      />
      {teams.isLoading && (
        <div className="text-sm text-slate-500">
          <Spinner size="small">Loading teams...</Spinner>
        </div>
      )}
    </div>
  );
}

export function TeamMembers({ orgId, teamId, canManage }: { orgId: string; teamId: string; canManage: boolean }) {
  const members = useTeamMembers(orgId, teamId, true);
  const queryClient = useQueryClient();
  const [error, setError] = useState<string | null>(null);
  const form = useForm<TeamMemberForm>({
    resolver: zodResolver(teamMemberSchema),
    defaultValues: { email: '', role: 'member' },
  });

  const invalidate = () =>
    queryClient.invalidateQueries({ queryKey: ['admin', 'orgs', orgId, 'teams', teamId, 'members'] });

  const addMutation = useMutation({
    mutationFn: async (values: TeamMemberForm) =>
      addTeamMember(orgId, teamId, { email: values.email.trim(), role: values.role ?? 'member' }),
    onSuccess: async () => {
      form.reset({ email: '', role: form.getValues('role') });
      setError(null);
      await invalidate();
    },
    onError: (err) => setError(errorMessage(err)),
  });

  const rowMutation = useMutation({
    mutationFn: async (op: { kind: 'role'; userId: string; role: string } | { kind: 'remove'; userId: string }) => {
      if (op.kind === 'role') await setTeamMemberRole(orgId, teamId, op.userId, op.role);
      else await removeTeamMember(orgId, teamId, op.userId);
    },
    onSuccess: invalidate,
    onError: (err) => setError(errorMessage(err)),
  });

  const busy = addMutation.isPending || rowMutation.isPending;

  return (
    <div className="grid gap-2 pl-2">
      {error && (
        <Alert variant="danger">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}
      {(members.data ?? []).map((m) => (
        <div key={m.user_id} className="flex flex-wrap items-center gap-2 text-sm">
          <span className="font-mono">{m.email || m.user_id}</span>
          <RoleBadge role={m.role} />
          {canManage && (
            <>
              <NativeSelect
                aria-label={`Team role for ${m.email || m.user_id}`}
                value={m.role}
                disabled={busy}
                onChange={(e) => rowMutation.mutate({ kind: 'role', userId: m.user_id, role: e.target.value })}
              >
                <NativeSelectOption value="admin">admin</NativeSelectOption>
                <NativeSelectOption value="member">member</NativeSelectOption>
              </NativeSelect>
              <Button
                appearance="outline"
                type="button"
                disabled={busy}
                onClick={() => rowMutation.mutate({ kind: 'remove', userId: m.user_id })}
              >
                Remove
              </Button>
            </>
          )}
        </div>
      ))}
      {canManage && (
        <FormProvider {...form}>
          <form
            className="flex flex-wrap items-end gap-2"
            onSubmit={form.handleSubmit((values) => addMutation.mutate(values))}
          >
            <div className="max-w-xs flex-1">
              <HookFormTextInput<TeamMemberForm>
                name="email"
                label="Add member (must already belong to the org)"
                placeholder="teammate@example.com"
              />
            </div>
            <div className="max-w-[10rem] flex-1">
              <HookFormNativeSelect<TeamMemberForm>
                name="role"
                label="Role"
                data={[
                  { label: 'member', value: 'member' },
                  { label: 'admin', value: 'admin' },
                ]}
              />
            </div>
            <Button appearance="outline" type="submit" disabled={busy}>
              {addMutation.isPending ? 'Adding...' : 'Add member'}
            </Button>
          </form>
        </FormProvider>
      )}
    </div>
  );
}

export function MemberSection({ org, canManage }: { org: AdminOrg; canManage: boolean }) {
  const members = useOrgMembers(org.id, true);
  const me = useAdminMe(true);
  const queryClient = useQueryClient();
  const [error, setError] = useState<string | null>(null);
  const [openQuota, setOpenQuota] = useState<string | null>(null);
  const confirm = useConfirm();
  const selfId = me.data?.user_id ?? '';

  const form = useForm<OrgMemberForm>({
    resolver: zodResolver(orgMemberSchema),
    defaultValues: { email: '', role: 'member' },
  });

  const invalidate = () => queryClient.invalidateQueries({ queryKey: ['admin', 'orgs', org.id, 'members'] });

  const addMutation = useMutation({
    mutationFn: async (values: OrgMemberForm) =>
      addOrgMember(org.id, { email: values.email.trim(), role: values.role ?? 'member' }),
    onSuccess: async () => {
      form.reset({ email: '', role: form.getValues('role') });
      setError(null);
      await invalidate();
    },
    onError: (err) => setError(errorMessage(err)),
  });

  const rowMutation = useMutation({
    mutationFn: async (op: { kind: 'role'; userId: string; role: string } | { kind: 'remove'; userId: string }) => {
      if (op.kind === 'role') await setOrgMemberRole(org.id, op.userId, op.role);
      else await removeOrgMember(org.id, op.userId);
    },
    onSuccess: invalidate,
    onError: (err) => setError(errorMessage(err)),
  });

  const askRemove = async (email: string, userId: string) => {
    const ok = await confirm({
      title: 'Remove member',
      description: `Remove "${email}" from this organization?`,
      confirmText: 'Remove',
    });
    if (ok) rowMutation.mutate({ kind: 'remove', userId });
  };

  const busy = addMutation.isPending || rowMutation.isPending;

  const columns: ManagementColumnDef<AdminOrgMember>[] = useMemo(
    () => [
      {
        accessorKey: 'email',
        header: 'Member',
        cell: ({ row }) => <span className="font-mono font-medium">{row.original.email}</span>,
      },
      {
        accessorKey: 'role',
        header: 'Role',
        cell: ({ row }) => {
          const m = row.original;
          return canManage ? (
            <NativeSelect
              aria-label={`Role for ${m.email}`}
              value={m.role}
              disabled={busy}
              onChange={(e) => rowMutation.mutate({ kind: 'role', userId: m.user_id, role: e.target.value })}
            >
              <NativeSelectOption value="admin">admin</NativeSelectOption>
              <NativeSelectOption value="member">member</NativeSelectOption>
            </NativeSelect>
          ) : (
            <RoleBadge role={m.role} />
          );
        },
      },
      {
        id: 'actions',
        header: '',
        enableSorting: false,
        enableGlobalFilter: false,
        cell: ({ row }) => {
          const m = row.original;
          const isSelf = selfId !== '' && m.user_id === selfId;
          const showQuota = canManage || isSelf;
          return (
            <span className="flex flex-wrap items-center gap-2">
              {canManage && (
                <Button
                  appearance="outline"
                  type="button"
                  disabled={busy}
                  onClick={() => askRemove(m.email, m.user_id)}
                >
                  Remove
                </Button>
              )}
              {showQuota && (
                <button
                  type="button"
                  className="text-xs text-slate-500 underline"
                  onClick={() => setOpenQuota(openQuota === m.user_id ? null : m.user_id)}
                >
                  Quotas
                </button>
              )}
            </span>
          );
        },
      },
    ],
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [busy, canManage, openQuota, selfId],
  );

  const expandedIds = useMemo(() => {
    const ids = new Set<string>();
    if (openQuota) ids.add(openQuota);
    return ids;
  }, [openQuota]);

  return (
    <div className="grid gap-2">
      {error && (
        <Alert variant="danger">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}
      <DataTable
        columns={columns}
        data={members.data ?? []}
        filterPlaceholder="Filter members..."
        getRowId={(row) => row.user_id}
        emptyText="No members."
        expandedIds={expandedIds}
        renderExpanded={(m) => {
          const isSelf = selfId !== '' && m.user_id === selfId;
          const showQuota = canManage || isSelf;
          if (!showQuota || openQuota !== m.user_id) return null;
          return (
            <QuotaEditor
              key={m.user_id}
              orgId={org.id}
              scope="users"
              scopeId={m.user_id}
              canEditPolicy={canManage}
              canEditEffective={showQuota}
            />
          );
        }}
      />
      {members.isLoading && (
        <div className="text-sm text-slate-500">
          <Spinner size="small">Loading members...</Spinner>
        </div>
      )}
      {canManage && (
        <div className="grid max-w-2xl gap-2 rounded-md border p-3">
          <FormProvider {...form}>
            <form className="grid gap-2" onSubmit={form.handleSubmit((values) => addMutation.mutate(values))}>
              <HookFormTextInput<OrgMemberForm>
                name="email"
                label="Add member by email (must already have an account)"
                placeholder="teammate@example.com"
              />
              <HookFormNativeSelect<OrgMemberForm>
                name="role"
                label="Role"
                data={[
                  { label: 'member', value: 'member' },
                  { label: 'admin', value: 'admin' },
                ]}
              />
              <div>
                <Button appearance="outline" type="submit" disabled={busy}>
                  {addMutation.isPending ? 'Adding...' : 'Add'}
                </Button>
              </div>
            </form>
          </FormProvider>
        </div>
      )}
    </div>
  );
}

export const InviteDialog = createTypedDialog<{ orgId: string; orgName: string }, boolean>(
  ({ open, args, onClose }) => {
    const { openDialog } = useDialog();
    const [error, setError] = useState<string | null>(null);
    const form = useForm<InviteForm>({
      resolver: zodResolver(inviteSchema),
      defaultValues: { email: '', role: 'member' },
    });

    const inviteMutation = useMutation({
      mutationFn: async (values: InviteForm) =>
        createAdminInvite({ email: values.email.trim(), org_id: args.orgId, org_role: values.role ?? 'member' }),
      onSuccess: async ({ token }) => {
        setError(null);
        onClose(true);
        await showSecret(openDialog, {
          title: `Invite to ${args.orgName}`,
          description: 'New users set their own password with this token.',
          secret: token,
          hint: 'Shown once. Copy it now.',
        });
      },
      onError: (err) => setError(errorMessage(err)),
    });

    return (
      <Dialog open={open} onOpenChange={(o) => !o && onClose(false)}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>Invite to {args.orgName}</DialogTitle>
          </DialogHeader>
          <FormProvider {...form}>
            <form className="grid gap-4" onSubmit={form.handleSubmit((values) => inviteMutation.mutate(values))}>
              {error && (
                <Alert variant="danger">
                  <AlertDescription>{error}</AlertDescription>
                </Alert>
              )}
              <HookFormTextInput<InviteForm>
                name="email"
                label="Email (new users set their own password)"
                placeholder="teammate@example.com"
              />
              <HookFormNativeSelect<InviteForm>
                name="role"
                label="Role"
                data={[
                  { label: 'member', value: 'member' },
                  { label: 'admin', value: 'admin' },
                ]}
              />
              <DialogFooter>
                <Button appearance="outline" type="button" onClick={() => onClose(false)}>
                  Cancel
                </Button>
                <Button variant="primary" type="submit" disabled={inviteMutation.isPending}>
                  {inviteMutation.isPending ? 'Inviting...' : 'Invite'}
                </Button>
              </DialogFooter>
            </form>
          </FormProvider>
        </DialogContent>
      </Dialog>
    );
  },
);

export function OrgDetail({
  org,
  isGlobalAdmin,
  onChanged,
  readOnly,
  compact,
}: {
  org: AdminOrg;
  isGlobalAdmin: boolean;
  onChanged: () => void;
  readOnly?: boolean;
  compact?: boolean;
}) {
  const canManage = !readOnly && (isGlobalAdmin || org.role === 'admin');
  const [error, setError] = useState<string | null>(null);
  const confirm = useConfirm();
  const { openDialog } = useDialog();
  const queryClient = useQueryClient();
  const form = useForm<DisplayNameForm>({
    resolver: zodResolver(displayNameSchema),
    defaultValues: { display_name: org.display_name ?? '' },
  });

  const saveMutation = useMutation({
    mutationFn: async (values: DisplayNameForm) => updateAdminOrg(org.id, { display_name: values.display_name ?? '' }),
    onSuccess: onChanged,
    onError: (err) => setError(errorMessage(err)),
  });

  const deleteMutation = useMutation({
    mutationFn: async () => deleteAdminOrg(org.id),
    onSuccess: onChanged,
    onError: (err) => setError(errorMessage(err)),
  });

  const askDelete = async () => {
    const ok = await confirm({
      title: 'Delete organization',
      description: `Delete organization ${org.name}? This cannot be undone.`,
      confirmText: 'Delete',
    });
    if (ok) deleteMutation.mutate();
  };

  const busy = saveMutation.isPending || deleteMutation.isPending;

  const openInvite = async () => {
    try {
      await openDialog(InviteDialog, { orgId: org.id, orgName: org.name });
    } catch {
      return;
    }
  };

  const openCreateTeam = async () => {
    try {
      const created = await openDialog(CreateTeamDialog, { orgId: org.id });
      if (created) {
        await queryClient.invalidateQueries({ queryKey: ['admin', 'orgs', org.id, 'teams'] });
        onChanged();
      }
    } catch {
      return;
    }
  };

  return (
    <div className="grid gap-6">
      {error && (
        <Alert variant="danger">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            {org.display_name || org.name}
            {org.is_system && (
              <Badge variant="warning" size="sm">
                system
              </Badge>
            )}
            <RoleBadge role={org.role} />
          </CardTitle>
        </CardHeader>
        <CardContent className="grid max-w-2xl gap-2">
          <div className="text-sm text-slate-500">
            Name: <span className="font-mono">{org.name}</span>
          </div>
          {canManage && !org.is_system && (
            <FormProvider {...form}>
              <form
                className="flex flex-wrap items-end gap-2"
                onSubmit={form.handleSubmit((values) => saveMutation.mutate(values))}
              >
                <div className="max-w-xs flex-1">
                  <HookFormTextInput<DisplayNameForm> name="display_name" label="Display name" />
                </div>
                <Button appearance="outline" type="submit" disabled={busy}>
                  Save
                </Button>
                <Button variant="danger" appearance="outline" type="button" disabled={busy} onClick={askDelete}>
                  Delete organization
                </Button>
              </form>
            </FormProvider>
          )}
        </CardContent>
      </Card>
      {!compact && (
        <Card>
          <CardHeader className="flex flex-row items-center justify-between gap-2">
            <CardTitle>Teams</CardTitle>
            {canManage && (
              <Button variant="primary" type="button" onClick={openCreateTeam}>
                Create team
              </Button>
            )}
          </CardHeader>
          <CardContent>
            <TeamSection org={org} canManage={canManage} />
          </CardContent>
        </Card>
      )}
      {!compact && (
        <Card>
          <CardHeader className="flex flex-row items-center justify-between gap-2">
            <CardTitle>Members</CardTitle>
            {canManage && (
              <Button variant="primary" type="button" onClick={openInvite}>
                Invite
              </Button>
            )}
          </CardHeader>
          <CardContent>
            <MemberSection org={org} canManage={canManage} />
          </CardContent>
        </Card>
      )}
    </div>
  );
}
