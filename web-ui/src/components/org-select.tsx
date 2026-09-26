import { Badge } from '@egose/shadcn-theme/components/ui/badge';
import { Label } from '@egose/shadcn-theme/components/ui/label';
import { NativeSelect, NativeSelectOption } from '@egose/shadcn-theme/components/ui/native-select';
import type { AdminOrg } from '../types';

export function OrgBadge({ orgId, orgName }: { orgId?: string; orgName?: string }) {
  if (!orgId) return null;
  return (
    <Badge variant="secondary" size="sm" title={`Organization: ${orgName || orgId}`}>
      {orgName || orgId.slice(0, 8)}
    </Badge>
  );
}

export function OrgSelect({
  orgs,
  value,
  onChange,
  id,
}: {
  orgs: AdminOrg[] | undefined;
  value: string;
  onChange: (v: string) => void;
  id: string;
}) {
  if (!orgs || orgs.length < 2) return null;
  return (
    <div className="grid gap-2">
      <Label htmlFor={id}>Organization</Label>
      <NativeSelect id={id} value={value} onChange={(e) => onChange(e.target.value)}>
        <NativeSelectOption value="">Select organization...</NativeSelectOption>
        {orgs.map((o) => (
          <NativeSelectOption key={o.id} value={o.id}>
            {o.display_name || o.name}
          </NativeSelectOption>
        ))}
      </NativeSelect>
    </div>
  );
}
