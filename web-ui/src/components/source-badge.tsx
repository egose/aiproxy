import { Badge } from '@egose/shadcn-theme/components/ui/badge';

export function SourceBadge({ source }: { source: string }) {
  const database = source === 'database';
  return (
    <Badge
      variant={database ? 'info' : 'secondary'}
      size="sm"
      title={database ? 'Stored in the database, managed here' : 'Defined in the static config file, read-only here'}
    >
      {database ? 'database' : 'config'}
    </Badge>
  );
}
