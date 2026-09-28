import { Card, CardContent, CardHeader, CardTitle } from '@egose/shadcn-theme/components/ui/card';
import { Spinner } from '@egose/shadcn-theme/components/ui/spinner';
import { useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import { useAdminWorkspaces } from '../hooks';
import { WorkspaceDetail, RoleBadge } from './workspaces-page';
import { KindBadge } from '../components/workspace-select';

export function AdminWorkspacesPage() {
  const workspaces = useAdminWorkspaces(true);
  const queryClient = useQueryClient();
  const [selected, setSelected] = useState<string | null>(null);

  const rows = workspaces.data ?? [];
  const current = rows.find((o) => o.id === selected) ?? rows[0];

  return (
    <div className="grid w-full gap-6 p-6">
      <Card>
        <CardHeader>
          <CardTitle>Workspaces ({rows.length})</CardTitle>
        </CardHeader>
        <CardContent className="grid gap-2">
          {rows.map((o) => (
            <div key={o.id} className="flex flex-wrap items-center gap-2 border-t py-2 text-sm">
              <button type="button" className="font-mono font-medium underline" onClick={() => setSelected(o.id)}>
                {o.display_name || o.name}
              </button>
              <span className="font-mono text-xs text-slate-500">{o.name}</span>
              <KindBadge kind={o.kind} isSystem={o.is_system} />
              <RoleBadge role={o.role} />
            </div>
          ))}
          {workspaces.isLoading && (
            <div className="text-sm text-slate-500">
              <Spinner size="small">Loading workspaces...</Spinner>
            </div>
          )}
        </CardContent>
      </Card>
      {current && (
        <WorkspaceDetail
          key={current.id}
          workspace={current}
          readOnly
          isGlobalAdmin={false}
          onChanged={() => {
            queryClient.invalidateQueries({ queryKey: ['admin', 'workspaces'] });
            setSelected(null);
          }}
        />
      )}
    </div>
  );
}
