import { zodResolver } from '@hookform/resolvers/zod';
import { HookFormTextInput } from '@egose/shadcn-theme/components/form/hook-text-input';
import { Alert, AlertDescription } from '@egose/shadcn-theme/components/ui/alert';
import { Button } from '@egose/shadcn-theme/components/ui/button';
import { CopyableButton } from '@egose/shadcn-theme/components/ui/copy-button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@egose/shadcn-theme/components/ui/dialog';
import { ConfirmationDialog } from '@egose/shadcn-theme/components/widgets/confirmation-dialog';
import { createTypedDialog, useDialog } from '@egose/shadcn-theme/components/widgets/dialog-manager';
import { useMutation } from '@tanstack/react-query';
import { useCallback } from 'react';
import { FormProvider, useForm } from 'react-hook-form';
import { z } from 'zod';
import { createAdminWorkspace } from '../services/admin';
import { errorMessage } from '../services/dashboard';
import type { AdminWorkspace } from '../types';

interface SecretArgs {
  title: string;
  description?: string;
  secret: string;
  hint?: string;
}

export const SecretDialog = createTypedDialog<SecretArgs, void>(({ open, args, onClose }) => (
  <Dialog open={open} onOpenChange={(o) => !o && onClose(undefined)}>
    <DialogContent className="sm:max-w-lg">
      <DialogHeader>
        <DialogTitle>{args.title}</DialogTitle>
        {args.description && <DialogDescription>{args.description}</DialogDescription>}
      </DialogHeader>
      <div className="grid gap-2">
        <div className="rounded bg-slate-100 p-3 font-mono text-xs break-all">{args.secret}</div>
        {args.hint && <div className="text-xs text-slate-500">{args.hint}</div>}
      </div>
      <DialogFooter>
        <CopyableButton value={args.secret}>Copy</CopyableButton>
        <Button variant="primary" type="button" onClick={() => onClose(undefined)}>
          Done
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
));

export function useConfirm() {
  const { openDialog } = useDialog();
  return useCallback(
    async (args: { title?: string; description?: string; confirmText?: string }): Promise<boolean> => {
      try {
        const res = await openDialog(ConfirmationDialog, {
          title: args.title ?? 'Confirm',
          description: args.description ?? 'Are you sure?',
          confirmText: args.confirmText ?? 'Confirm',
          confirmVariant: 'danger',
        });
        return res.confirmed;
      } catch {
        return false;
      }
    },
    [openDialog],
  );
}

export async function showSecret(
  openDialog: ReturnType<typeof useDialog>['openDialog'],
  args: SecretArgs,
): Promise<void> {
  try {
    await openDialog(SecretDialog, args);
  } catch {
    return;
  }
}

const newWorkspaceSchema = z.object({
  name: z.string().trim().min(1, 'Enter a workspace name.'),
  display_name: z.string().optional(),
});

type NewWorkspaceForm = z.infer<typeof newWorkspaceSchema>;

export const NewWorkspaceDialog = createTypedDialog<object, AdminWorkspace | null>(({ open, onClose }) => {
  const form = useForm<NewWorkspaceForm>({
    resolver: zodResolver(newWorkspaceSchema),
    defaultValues: { name: '', display_name: '' },
  });

  const createMutation = useMutation({
    mutationFn: async (values: NewWorkspaceForm) =>
      createAdminWorkspace({ name: values.name.trim(), display_name: (values.display_name ?? '').trim() }),
    onSuccess: (created) => onClose(created),
  });

  return (
    <Dialog open={open} onOpenChange={(o) => !o && onClose(null)}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>New workspace</DialogTitle>
          <DialogDescription>
            Your first workspace is personal and created automatically. New workspaces you create here are organization
            workspaces for team collaboration. You will be the initial admin of this workspace.
          </DialogDescription>
        </DialogHeader>
        <FormProvider {...form}>
          <form className="grid gap-4" onSubmit={form.handleSubmit((values) => createMutation.mutate(values))}>
            <HookFormTextInput<NewWorkspaceForm> name="name" label="Name (lowercase, no spaces)" placeholder="acme" />
            <HookFormTextInput<NewWorkspaceForm> name="display_name" label="Display name (optional)" />
            {createMutation.error && (
              <Alert variant="danger">
                <AlertDescription>{errorMessage(createMutation.error)}</AlertDescription>
              </Alert>
            )}
            <DialogFooter>
              <Button appearance="outline" type="button" onClick={() => onClose(null)}>
                Cancel
              </Button>
              <Button variant="primary" type="submit" disabled={createMutation.isPending}>
                {createMutation.isPending ? 'Creating...' : 'Create workspace'}
              </Button>
            </DialogFooter>
          </form>
        </FormProvider>
      </DialogContent>
    </Dialog>
  );
});
