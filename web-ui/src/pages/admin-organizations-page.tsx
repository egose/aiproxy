import { Badge } from '@egose/shadcn-theme/components/ui/badge';
import { Card, CardContent, CardHeader, CardTitle } from '@egose/shadcn-theme/components/ui/card';
import { Spinner } from '@egose/shadcn-theme/components/ui/spinner';
import { useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import { useAdminOrgs } from '../hooks';
import { OrgDetail, RoleBadge } from './orgs-page';

export function AdminOrganizationsPage() {
  const orgs = useAdminOrgs(true);
  const queryClient = useQueryClient();
  const [selected, setSelected] = useState<string | null>(null);

  const rows = orgs.data ?? [];
  const current = rows.find((o) => o.id === selected) ?? rows[0];

  return (
    <div className="grid w-full gap-6 p-6">
      <Card>
        <CardHeader>
          <CardTitle>Organizations ({rows.length})</CardTitle>
        </CardHeader>
        <CardContent className="grid gap-2">
          {rows.map((o) => (
            <div key={o.id} className="flex flex-wrap items-center gap-2 border-t py-2 text-sm">
              <button type="button" className="font-mono font-medium underline" onClick={() => setSelected(o.id)}>
                {o.display_name || o.name}
              </button>
              <span className="font-mono text-xs text-slate-500">{o.name}</span>
              {o.is_system && (
                <Badge variant="warning" size="sm">
                  system
                </Badge>
              )}
              <RoleBadge role={o.role} />
            </div>
          ))}
          {orgs.isLoading && (
            <div className="text-sm text-slate-500">
              <Spinner size="small">Loading organizations...</Spinner>
            </div>
          )}
        </CardContent>
      </Card>
      {current && (
        <OrgDetail
          key={current.id}
          org={current}
          readOnly
          isGlobalAdmin={false}
          onChanged={() => {
            queryClient.invalidateQueries({ queryKey: ['admin', 'orgs'] });
            setSelected(null);
          }}
        />
      )}
    </div>
  );
}
