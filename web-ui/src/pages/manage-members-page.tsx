import { Button } from '@egose/shadcn-theme/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@egose/shadcn-theme/components/ui/card';
import { Empty, EmptyTitle } from '@egose/shadcn-theme/components/ui/empty';
import { Spinner } from '@egose/shadcn-theme/components/ui/spinner';
import { useDialog } from '@egose/shadcn-theme/components/widgets/dialog-manager';
import { useAdminMe, useCurrentOrg } from '../hooks';
import { InviteDialog, MemberSection } from './orgs-page';

export function ManageMembersPage() {
  const { org, orgs } = useCurrentOrg();
  const me = useAdminMe(true);
  const { openDialog } = useDialog();

  if (orgs.isLoading) {
    return (
      <div className="p-6 text-sm text-slate-500">
        <Spinner size="small">Loading members...</Spinner>
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
  const openInvite = async () => {
    try {
      await openDialog(InviteDialog, { orgId: org.id, orgName: org.name });
    } catch {
      return;
    }
  };
  return (
    <div className="grid w-full gap-6 p-6">
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
    </div>
  );
}
