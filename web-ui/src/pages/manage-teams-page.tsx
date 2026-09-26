import { Button } from '@egose/shadcn-theme/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@egose/shadcn-theme/components/ui/card';
import { Empty, EmptyTitle } from '@egose/shadcn-theme/components/ui/empty';
import { Spinner } from '@egose/shadcn-theme/components/ui/spinner';
import { useDialog } from '@egose/shadcn-theme/components/widgets/dialog-manager';
import { useQueryClient } from '@tanstack/react-query';
import { useAdminMe, useCurrentOrg } from '../hooks';
import { CreateTeamDialog, TeamSection } from './orgs-page';

export function ManageTeamsPage() {
  const { org, orgs } = useCurrentOrg();
  const me = useAdminMe(true);
  const { openDialog } = useDialog();
  const queryClient = useQueryClient();

  if (orgs.isLoading) {
    return (
      <div className="p-6 text-sm text-slate-500">
        <Spinner size="small">Loading teams...</Spinner>
      </div>
    );
  }
  if (!org) {
    return (
      <div className="p-6">
        <Empty>
          <EmptyTitle>No organization selected</EmptyTitle>
        </Empty>
      </div>
    );
  }
  const canManage = me.data?.is_admin === true || org.role === 'admin';
  const openCreate = async () => {
    try {
      const created = await openDialog(CreateTeamDialog, { orgId: org.id });
      if (created) {
        await queryClient.invalidateQueries({ queryKey: ['admin', 'orgs', org.id, 'teams'] });
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
          {canManage && (
            <Button variant="primary" type="button" onClick={openCreate}>
              Create team
            </Button>
          )}
        </CardHeader>
        <CardContent>
          <TeamSection org={org} canManage={canManage} />
        </CardContent>
      </Card>
    </div>
  );
}
