import { Alert, AlertDescription } from '@egose/shadcn-theme/components/ui/alert';
import { Spinner } from '@egose/shadcn-theme/components/ui/spinner';

export function WorkspaceAccessNotice({ pending, error }: { pending: boolean; error: unknown }) {
  if (pending) return <Spinner size="small">Resolving workspace access...</Spinner>;
  return (
    <Alert>
      <AlertDescription>
        {error
          ? 'Unable to verify workspace access. Sign in again or retry.'
          : 'No authorized workspace selected. Create a workspace or ask an administrator for membership.'}
      </AlertDescription>
    </Alert>
  );
}
