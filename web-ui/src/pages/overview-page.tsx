import { Alert, AlertDescription } from '@egose/shadcn-theme/components/ui/alert';
import { Button } from '@egose/shadcn-theme/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@egose/shadcn-theme/components/ui/card';
import { useQueryClient } from '@tanstack/react-query';
import { Link } from 'react-router';
import { useDashboardToken, useSnapshot } from '../hooks';
import { errorMessage, isUnauthorized } from '../services/dashboard';

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-sm font-medium">{label}</CardTitle>
      </CardHeader>
      <CardContent className="pt-0 text-2xl font-semibold">{value}</CardContent>
    </Card>
  );
}

export function OverviewPage() {
  const token = useDashboardToken();
  const queryClient = useQueryClient();
  const snapshot = useSnapshot();

  if (!token) {
    return (
      <div className="mx-auto grid w-full max-w-2xl gap-6 p-6">
        <Card>
          <CardHeader>
            <CardTitle>Connect the dashboard</CardTitle>
          </CardHeader>
          <CardContent className="grid gap-4">
            <p className="text-sm">Paste the dashboard bearer token to load live state.</p>
            <Link to="/token">
              <Button variant="primary">Open token settings</Button>
            </Link>
          </CardContent>
        </Card>
      </div>
    );
  }

  if (snapshot.isPending) {
    return <div className="p-6 text-sm">Loading snapshot...</div>;
  }

  if (snapshot.error) {
    return (
      <div className="mx-auto grid w-full max-w-2xl gap-6 p-6">
        <Alert variant="danger">
          <AlertDescription>{errorMessage(snapshot.error)}</AlertDescription>
        </Alert>
        <div className="flex gap-2">
          <Button variant="outline" onClick={() => queryClient.invalidateQueries({ queryKey: ['dashboard'] })}>
            Retry
          </Button>
          {isUnauthorized(snapshot.error) && (
            <Link to="/token">
              <Button variant="primary">Fix token</Button>
            </Link>
          )}
        </div>
      </div>
    );
  }

  const data = snapshot.data;
  const healthy = Object.values(data.health ?? {}).filter(Boolean).length;
  const total = Object.keys(data.health ?? {}).length;
  const cooling = data.cooldowns?.length ?? 0;

  return (
    <div className="grid w-full gap-6 p-6">
      <div className="grid gap-4 md:grid-cols-4">
        <Stat label="Providers" value={String(data.providers.length)} />
        <Stat label="Aliases" value={String(data.aliases.length)} />
        <Stat label="Healthy" value={total ? `${healthy}/${total}` : 'n/a'} />
        <Stat label="Cooling" value={String(cooling)} />
      </div>

      <div className="grid gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle>Healthchecks</CardTitle>
          </CardHeader>
          <CardContent className="grid gap-2 pt-0 text-sm">
            {(data.healthchecks ?? []).length === 0 && <span>No healthcheck state.</span>}
            {(data.healthchecks ?? []).map((h) => (
              <div key={h.provider} className="flex items-center justify-between gap-2">
                <span className="font-mono">{h.provider}</span>
                <span>
                  {!h.configured
                    ? 'not configured'
                    : !h.checked
                      ? 'not checked'
                      : h.healthy
                        ? 'healthy'
                        : `unhealthy${h.status_code ? ` (${h.status_code})` : ''}`}
                </span>
              </div>
            ))}
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle>Cooldowns</CardTitle>
          </CardHeader>
          <CardContent className="grid gap-2 pt-0 text-sm">
            {(data.cooldowns ?? []).length === 0 && <span>No active cooldowns.</span>}
            {(data.cooldowns ?? []).map((c) => (
              <div key={`${c.alias}/${c.provider}/${c.model}`} className="flex items-center justify-between gap-2">
                <span className="font-mono">
                  {c.alias}/{c.provider}/{c.model}
                </span>
                <span>{Math.ceil(c.remaining_ms / 1000)}s</span>
              </div>
            ))}
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Usage</CardTitle>
        </CardHeader>
        <CardContent className="grid gap-2 pt-0 text-sm">
          {(data.usage ?? []).length === 0 && <span>No usage recorded yet.</span>}
          {(data.usage ?? []).slice(0, 20).map((u) => (
            <div
              key={`${u.Model}/${u.Operation ?? ''}/${u.StatusCode ?? ''}`}
              className="flex items-center justify-between gap-2"
            >
              <span className="font-mono">
                {u.Model} {u.Operation ? `· ${u.Operation}` : ''}
              </span>
              <span>
                {u.Count ?? 0} req · {u.TotalTokens ?? 0} tok
              </span>
            </div>
          ))}
        </CardContent>
      </Card>
    </div>
  );
}
