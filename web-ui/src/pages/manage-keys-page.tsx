import { Card, CardContent, CardHeader, CardTitle } from '@egose/shadcn-theme/components/ui/card';
import { Empty, EmptyDescription, EmptyTitle } from '@egose/shadcn-theme/components/ui/empty';
import { Spinner } from '@egose/shadcn-theme/components/ui/spinner';
import { KeyQuotaLine } from '../components/quota';
import { useAdminKeys, useAdminMe, useCurrentOrg, useOrgTeams } from '../hooks';
import { AdminKeysPage } from './admin-keys-page';

function MemberKeysView() {
  const keys = useAdminKeys();

  const rows = (keys.data ?? []).filter((k) => k.source === 'database');
  return (
    <div className="grid w-full gap-6 p-6">
      <Card>
        <CardHeader>
          <CardTitle>Shared API keys ({rows.length})</CardTitle>
        </CardHeader>
        <CardContent className="grid gap-2">
          {rows.map((k) => (
            <div key={k.id || k.name} className="grid gap-1 border-t py-2 text-sm">
              <div className="flex flex-wrap items-center gap-2">
                <span className="font-mono font-medium">{k.name}</span>
                {k.token_prefix && <span className="font-mono text-xs text-slate-500">{k.token_prefix}…</span>}
                {k.owner_name && (
                  <span className="text-slate-500">
                    {k.owner_team_id ? `team:${k.owner_name}` : `personal:${k.owner_name}`}
                  </span>
                )}
              </div>
              {k.description && <div className="text-slate-500">{k.description}</div>}
              {k.expires_at && <div className="font-mono text-xs text-slate-500">expires {k.expires_at}</div>}
              {(k.allowed_models ?? []).length > 0 && (
                <div className="font-mono text-xs text-slate-500">{k.allowed_models.join(', ')}</div>
              )}
              <div className="flex flex-wrap items-center gap-2">
                <KeyQuotaLine usage={k.usage} quota={k.quota} />
              </div>
            </div>
          ))}
          {keys.isLoading && (
            <div className="text-sm text-slate-500">
              <Spinner size="small">Loading API keys...</Spinner>
            </div>
          )}
          {!keys.isLoading && rows.length === 0 && (
            <Empty>
              <EmptyTitle>No API keys</EmptyTitle>
              <EmptyDescription>No API keys have been shared with you yet.</EmptyDescription>
            </Empty>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

export function ManageKeysPage() {
  const { org, orgs } = useCurrentOrg();
  const me = useAdminMe(true);
  const teams = useOrgTeams(org?.id ?? null, true);

  if (orgs.isLoading) {
    return (
      <div className="p-6 text-sm text-slate-500">
        <Spinner size="small">Loading API keys...</Spinner>
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
  const orgAdmin = me.data?.is_admin === true || org.role === 'admin';
  const teamAdmin = (teams.data ?? []).some((t) => t.my_role === 'admin');
  if (!orgAdmin && !teamAdmin) {
    return <MemberKeysView />;
  }
  return <AdminKeysPage key={org.id} fixedOrgId={org.id} canAdminOrg={orgAdmin} />;
}
