import { Button } from '@egose/shadcn-theme/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@egose/shadcn-theme/components/ui/card';
import { Empty, EmptyTitle } from '@egose/shadcn-theme/components/ui/empty';
import { Spinner } from '@egose/shadcn-theme/components/ui/spinner';
import { useDialog } from '@egose/shadcn-theme/components/widgets/dialog-manager';
import { useAdminMe, useCurrentWorkspace } from '../hooks';
import { InviteDialog, MemberSection } from './workspaces-page';

export function ManageMembersPage() {
  const { workspace, workspaces } = useCurrentWorkspace();
  const me = useAdminMe(true);
  const { openDialog } = useDialog();

  if (workspaces.isLoading) {
    return (
      <div className="p-6 text-sm text-slate-500">
        <Spinner size="small">Loading members...</Spinner>
      </div>
    );
  }
  if (!workspace) {
    return (
      <div className="p-6">
        <Empty>
          <EmptyTitle>No workspace selected</EmptyTitle>
        </Empty>
      </div>
    );
  }
  const canManage = me.data?.is_admin === true || workspace.role === 'admin';
  const isPersonal = workspace.kind === 'personal';
  const openInvite = async () => {
    try {
      await openDialog(InviteDialog, { workspaceId: workspace.id, workspaceName: workspace.name });
    } catch {
      return;
    }
  };
  return (
    <div className="grid w-full gap-6 p-6">
      <Card>
        <CardHeader className="flex flex-row items-center justify-between gap-2">
          <CardTitle>Members</CardTitle>
          {canManage && !isPersonal && (
            <Button variant="primary" type="button" onClick={openInvite}>
              Invite
            </Button>
          )}
        </CardHeader>
        <CardContent>
          {isPersonal ? (
            <p className="text-sm text-slate-600">
              Personal workspaces are for a single user. Members and invites are not available for personal workspaces.
            </p>
          ) : (
            <MemberSection workspace={workspace} canManage={canManage} />
          )}
        </CardContent>
      </Card>
    </div>
  );
}
