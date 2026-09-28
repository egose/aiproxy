import { Badge } from '@egose/shadcn-theme/components/ui/badge';
import { Label } from '@egose/shadcn-theme/components/ui/label';
import { NativeSelect, NativeSelectOption } from '@egose/shadcn-theme/components/ui/native-select';
import type { AdminWorkspace } from '../types';

export function WorkspaceBadge({ workspaceId, workspaceName }: { workspaceId?: string; workspaceName?: string }) {
  if (!workspaceId) return null;
  return (
    <Badge variant="secondary" size="sm" title={`Workspace: ${workspaceName || workspaceId}`}>
      {workspaceName || workspaceId.slice(0, 8)}
    </Badge>
  );
}

export function KindBadge({ kind, isSystem }: { kind?: string; isSystem?: boolean }) {
  if (isSystem) {
    return (
      <Badge variant="warning" size="sm">
        System
      </Badge>
    );
  }
  if (kind === 'personal') {
    return (
      <Badge variant="secondary" size="sm" title="Personal workspace">
        Personal
      </Badge>
    );
  }
  return (
    <Badge variant="secondary" size="sm" title="Organization workspace">
      Organization
    </Badge>
  );
}

export function WorkspaceSelect({
  workspaces,
  value,
  onChange,
  id,
}: {
  workspaces: AdminWorkspace[] | undefined;
  value: string;
  onChange: (v: string) => void;
  id: string;
}) {
  if (!workspaces || workspaces.length < 2) return null;
  return (
    <div className="grid gap-2">
      <Label htmlFor={id}>Workspace</Label>
      <NativeSelect id={id} value={value} onChange={(e) => onChange(e.target.value)}>
        <NativeSelectOption value="">Select workspace...</NativeSelectOption>
        {workspaces.map((o) => (
          <NativeSelectOption key={o.id} value={o.id}>
            {o.display_name || o.name}
          </NativeSelectOption>
        ))}
      </NativeSelect>
    </div>
  );
}
