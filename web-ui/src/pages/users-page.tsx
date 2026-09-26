import { zodResolver } from '@hookform/resolvers/zod';
import { HookFormTextInput } from '@egose/shadcn-theme/components/form/hook-text-input';
import { HookFormNativeSelect } from '@egose/shadcn-theme/components/form/hook-native-select';
import { Alert, AlertDescription } from '@egose/shadcn-theme/components/ui/alert';
import { Badge } from '@egose/shadcn-theme/components/ui/badge';
import { Button } from '@egose/shadcn-theme/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@egose/shadcn-theme/components/ui/card';
import { Spinner } from '@egose/shadcn-theme/components/ui/spinner';
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@egose/shadcn-theme/components/ui/dialog';
import { createTypedDialog, useDialog } from '@egose/shadcn-theme/components/widgets/dialog-manager';
import { ActionMenu } from '@egose/shadcn-theme/components/widgets/action-menu';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import { FormProvider, useForm } from 'react-hook-form';
import { z } from 'zod';
import { Separator } from '@egose/shadcn-theme/components/ui/separator';
import { NativeSelect, NativeSelectOption } from '@egose/shadcn-theme/components/ui/native-select';
import { showSecret, useConfirm } from '../components/dialogs';
import { useAdminInvites, useAdminMe, useAdminUsers } from '../hooks';
import {
  createAdminInvite,
  deleteAdminInvite,
  deleteAdminUser,
  resetUserPassword,
  setUserDisabled,
  setUserRole,
} from '../services/admin';
import { errorMessage } from '../services/dashboard';

function RoleBadge({ role }: { role: string }) {
  return (
    <Badge variant={role === 'admin' ? 'info' : 'secondary'} size="sm">
      {role}
    </Badge>
  );
}

const inviteSchema = z.object({
  email: z.string().trim().min(1, 'Enter an email.').email('Enter a valid email.'),
  role: z.enum(['admin', 'user']).optional(),
});

type InviteForm = z.infer<typeof inviteSchema>;

const passwordSchema = z.object({
  password: z.string().min(8, 'Password must be at least 8 characters.'),
});

type PasswordForm = z.infer<typeof passwordSchema>;

const ResetPasswordDialog = createTypedDialog<{ email: string }, string | null>(({ open, args, onClose }) => {
  const form = useForm<PasswordForm>({
    resolver: zodResolver(passwordSchema),
    defaultValues: { password: '' },
  });
  return (
    <Dialog open={open} onOpenChange={(o) => !o && onClose(null)}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Reset password for {args.email}</DialogTitle>
        </DialogHeader>
        <FormProvider {...form}>
          <form className="grid gap-4" onSubmit={form.handleSubmit((values) => onClose(values.password))}>
            <HookFormTextInput<PasswordForm>
              name="password"
              label="New password (min 8 characters)"
              type="password"
              autoComplete="new-password"
            />
            <DialogFooter>
              <Button appearance="outline" type="button" onClick={() => onClose(null)}>
                Cancel
              </Button>
              <Button variant="primary" type="submit">
                Save
              </Button>
            </DialogFooter>
          </form>
        </FormProvider>
      </DialogContent>
    </Dialog>
  );
});

export function UsersPage() {
  const users = useAdminUsers(true);
  const invites = useAdminInvites(true);
  const me = useAdminMe(true);
  const queryClient = useQueryClient();
  const { openDialog } = useDialog();
  const confirm = useConfirm();
  const [error, setError] = useState<string | null>(null);

  const inviteForm = useForm<InviteForm>({
    resolver: zodResolver(inviteSchema),
    defaultValues: { email: '', role: 'user' },
  });

  const invalidate = () =>
    Promise.all([
      queryClient.invalidateQueries({ queryKey: ['admin', 'users'] }),
      queryClient.invalidateQueries({ queryKey: ['admin', 'invites'] }),
    ]);

  const inviteMutation = useMutation({
    mutationFn: async (values: InviteForm) =>
      createAdminInvite({ email: values.email.trim(), role: values.role ?? 'user' }),
    onSuccess: async ({ token }) => {
      inviteForm.reset({ email: '', role: inviteForm.getValues('role') });
      setError(null);
      await queryClient.invalidateQueries({ queryKey: ['admin', 'invites'] });
      await showSecret(openDialog, {
        title: 'Invite token',
        description: 'Copy it now. It is shown once and expires after 7 days.',
        secret: token,
      });
    },
    onError: (err) => setError(errorMessage(err)),
  });

  const rowMutation = useMutation({
    mutationFn: async (
      op:
        | { kind: 'role'; id: string; role: string }
        | { kind: 'toggle'; id: string; disabled: boolean }
        | { kind: 'delete'; id: string }
        | { kind: 'revoke'; id: string }
        | { kind: 'reset'; id: string; password: string },
    ) => {
      if (op.kind === 'role') await setUserRole(op.id, op.role);
      else if (op.kind === 'toggle') await setUserDisabled(op.id, op.disabled);
      else if (op.kind === 'delete') await deleteAdminUser(op.id);
      else if (op.kind === 'revoke') await deleteAdminInvite(op.id);
      else await resetUserPassword(op.id, op.password);
    },
    onSuccess: async () => {
      setError(null);
      await invalidate();
    },
    onError: (err) => setError(errorMessage(err)),
  });

  const askDeleteUser = async (email: string, id: string) => {
    const ok = await confirm({ title: 'Delete user', description: `Delete user "${email}"?`, confirmText: 'Delete' });
    if (ok) rowMutation.mutate({ kind: 'delete', id });
  };

  const askResetPassword = async (email: string, id: string) => {
    try {
      const password = await openDialog(ResetPasswordDialog, { email });
      if (password) rowMutation.mutate({ kind: 'reset', id, password });
    } catch {
      return;
    }
  };

  const rows = users.data ?? [];
  const pending = invites.data ?? [];
  const selfId = me.data?.user_id ?? '';
  const busy = rowMutation.isPending;

  return (
    <div className="grid w-full gap-6 p-6">
      {error && (
        <Alert variant="danger">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}
      <Card>
        <CardHeader>
          <CardTitle>Users ({rows.length})</CardTitle>
          <CardDescription>
            Global roles: admins manage providers, aliases, keys, users and sign-on settings; users can view the
            dashboard and use the APIs allowed to them.
          </CardDescription>
        </CardHeader>
        <CardContent className="grid gap-2">
          {rows.map((u) => (
            <div key={u.id} className="grid gap-2 border-t py-2 text-sm">
              <div className="flex flex-wrap items-center gap-2">
                <span className="font-mono font-medium">{u.email}</span>
                <RoleBadge role={u.role} />
                {(u.orgs ?? []).map((o) => (
                  <Badge key={o.id} variant="secondary" size="sm" title={`Organization role: ${o.role}`}>
                    {o.name} · {o.role}
                  </Badge>
                ))}
                {u.disabled && (
                  <Badge variant="warning" size="sm">
                    disabled
                  </Badge>
                )}
                {u.id === selfId && (
                  <span className="text-xs text-slate-400" title="Your own account">
                    (you)
                  </span>
                )}
              </div>
              {u.id !== selfId && (
                <div className="flex flex-wrap items-center gap-2">
                  <NativeSelect
                    aria-label={`Role for ${u.email}`}
                    value={u.role}
                    disabled={busy}
                    onChange={(e) => rowMutation.mutate({ kind: 'role', id: u.id, role: e.target.value })}
                  >
                    <NativeSelectOption value="admin">admin</NativeSelectOption>
                    <NativeSelectOption value="user">user</NativeSelectOption>
                  </NativeSelect>
                  <ActionMenu
                    items={[
                      {
                        label: u.disabled ? 'Enable' : 'Disable',
                        onSelect: () => rowMutation.mutate({ kind: 'toggle', id: u.id, disabled: !u.disabled }),
                        disabled: busy,
                      },
                      { label: 'Reset password', onSelect: () => askResetPassword(u.email, u.id), disabled: busy },
                      {
                        label: 'Delete',
                        onSelect: () => askDeleteUser(u.email, u.id),
                        disabled: busy,
                        variant: 'destructive',
                      },
                    ]}
                  />
                </div>
              )}
            </div>
          ))}
          {users.isLoading && (
            <div className="text-sm text-slate-500">
              <Spinner size="small">Loading users...</Spinner>
            </div>
          )}
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>Invite user</CardTitle>
          <CardDescription>
            Invited users set their own password via the invite token, which is shown once and expires after 7 days.
          </CardDescription>
        </CardHeader>
        <CardContent className="grid max-w-2xl gap-4">
          <FormProvider {...inviteForm}>
            <form
              className="grid max-w-2xl gap-4"
              onSubmit={inviteForm.handleSubmit((values) => inviteMutation.mutate(values))}
            >
              <HookFormTextInput<InviteForm>
                name="email"
                label="Email"
                placeholder="teammate@example.com"
                autoComplete="off"
              />
              <HookFormNativeSelect<InviteForm>
                name="role"
                label="Role"
                data={[
                  { label: 'user', value: 'user' },
                  { label: 'admin', value: 'admin' },
                ]}
              />
              <div>
                <Button variant="primary" type="submit" disabled={inviteMutation.isPending}>
                  {inviteMutation.isPending ? 'Creating...' : 'Create invite'}
                </Button>
              </div>
            </form>
          </FormProvider>
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>Pending invites ({pending.length})</CardTitle>
        </CardHeader>
        <CardContent className="grid gap-2">
          {pending.map((inv) => (
            <div key={inv.id} className="flex flex-wrap items-center gap-2 border-t py-2 text-sm">
              <span className="font-mono font-medium">{inv.email}</span>
              <RoleBadge role={inv.role} />
              <span className="text-xs text-slate-500">expires {new Date(inv.expires_at).toLocaleString()}</span>
              <Button
                appearance="outline"
                type="button"
                disabled={busy}
                onClick={() => rowMutation.mutate({ kind: 'revoke', id: inv.id })}
              >
                Revoke
              </Button>
            </div>
          ))}
          {pending.length === 0 && !invites.isLoading && (
            <div className="text-sm text-slate-500">No pending invites.</div>
          )}
          {invites.isLoading && (
            <div className="text-sm text-slate-500">
              <Spinner size="small">Loading invites...</Spinner>
            </div>
          )}
        </CardContent>
      </Card>
      <Separator />
    </div>
  );
}
