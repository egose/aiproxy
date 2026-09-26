import { Empty, EmptyTitle } from '@egose/shadcn-theme/components/ui/empty';
import { Spinner } from '@egose/shadcn-theme/components/ui/spinner';
import { useQueryClient } from '@tanstack/react-query';
import { useAdminMe, useCurrentOrg } from '../hooks';
import { OrgDetail } from './orgs-page';

export function ManageOrganizationPage() {
  const { org, orgs } = useCurrentOrg();
  const me = useAdminMe(true);
  const queryClient = useQueryClient();

  if (orgs.isLoading) {
    return (
      <div className="p-6 text-sm text-slate-500">
        <Spinner size="small">Loading organization...</Spinner>
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
  return (
    <div className="grid w-full gap-6 p-6">
      <OrgDetail
        key={org.id}
        org={org}
        compact
        isGlobalAdmin={me.data?.is_admin === true}
        onChanged={() => {
          queryClient.invalidateQueries({ queryKey: ['admin', 'orgs'] });
        }}
      />
    </div>
  );
}
