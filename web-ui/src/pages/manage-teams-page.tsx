import { Button } from '@egose/shadcn-theme/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@egose/shadcn-theme/components/ui/card';
import { Empty, EmptyTitle } from '@egose/shadcn-theme/components/ui/empty';
import { Spinner } from '@egose/shadcn-theme/components/ui/spinner';
import { useDialog } from '@egose/shadcn-theme/components/widgets/dialog-manager';
import { useQueryClient } from '@tanstack/react-query';
import { useAdminMe, useCurrentWorkspace } from '../hooks';
import { CreateTeamDialog, TeamSection } from './workspaces-page';

export function ManageTeamsPage() {
  const { workspace, workspaces } = useCurrentWorkspace();
  const me = useAdminMe(true);
  const { openDialog } = useDialog();
  const queryClient = useQueryClient();

  if (workspaces.isLoading) {
    return (
      <div className="p-6 text-sm text-slate-500">
        <Spinner size="small">Loading teams...</Spinner>
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
  const openCreate = async () => {
    try {
      const created = await openDialog(CreateTeamDialog, { workspaceId: workspace.id });
      if (created) {
        await queryClient.invalidateQueries({ queryKey: ['admin', 'workspaces', workspace.id, 'teams'] });
      }
    } catch {
      return;
    }
  };
  return (
    <div className="grid w-full gap-6 p-6">
      <Card>
        <CardHeader className="flex flex-row items-center justify-between gap-2">
          <CardTitle>Teams</CardTitle>
          {canManage && !isPersonal && (
            <Button variant="primary" type="button" onClick={openCreate}>
              Create team
            </Button>
          )}
        </CardHeader>
        <CardContent>
          {isPersonal ? (
            <p className="text-sm text-slate-600">
              Personal workspaces are for a single user. Teams are not available for personal workspaces.
            </p>
          ) : (
            <TeamSection workspace={workspace} canManage={canManage} />
          )}
        </CardContent>
      </Card>
    </div>
  );
}
