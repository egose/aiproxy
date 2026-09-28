import { Empty, EmptyTitle } from '@egose/shadcn-theme/components/ui/empty';
import { Spinner } from '@egose/shadcn-theme/components/ui/spinner';
import { useQueryClient } from '@tanstack/react-query';
import { useAdminMe, useCurrentWorkspace } from '../hooks';
import { WorkspaceDetail } from './workspaces-page';

export function ManageWorkspacePage() {
  const { workspace, workspaces } = useCurrentWorkspace();
  const me = useAdminMe(true);
  const queryClient = useQueryClient();

  if (workspaces.isLoading) {
    return (
      <div className="p-6 text-sm text-slate-500">
        <Spinner size="small">Loading workspace...</Spinner>
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
  return (
    <div className="grid w-full gap-6 p-6">
      <WorkspaceDetail
        key={workspace.id}
        workspace={workspace}
        compact
        isGlobalAdmin={me.data?.is_admin === true}
        onChanged={() => {
          queryClient.invalidateQueries({ queryKey: ['admin', 'workspaces'] });
        }}
      />
    </div>
  );
}
