import { useState } from 'react';
import { Alert, AlertDescription } from '@egose/shadcn-theme/components/ui/alert';
import { Button } from '@egose/shadcn-theme/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@egose/shadcn-theme/components/ui/card';
import { Label } from '@egose/shadcn-theme/components/ui/label';
import { usePayload, usePayloads } from '../hooks';
import { errorMessage } from '../services/dashboard';

export function PayloadsPage() {
  const [errorsOnly, setErrorsOnly] = useState(false);
  const [selected, setSelected] = useState<string | null>(null);
  const list = usePayloads(100, errorsOnly);
  const detail = usePayload(selected);

  return (
    <div className="grid w-full gap-6 p-6 lg:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
      <Card>
        <CardHeader>
          <CardTitle>Payloads</CardTitle>
        </CardHeader>
        <CardContent className="grid gap-2 pt-0 text-sm">
          <Label className="flex items-center gap-2">
            <input type="checkbox" checked={errorsOnly} onChange={(e) => setErrorsOnly(e.target.checked)} />
            Errors only
          </Label>
          {list.error && (
            <Alert variant="danger">
              <AlertDescription>{errorMessage(list.error)}</AlertDescription>
            </Alert>
          )}
          {list.data && !list.data.enabled && <span>Payload log is not enabled on this server.</span>}
          {(list.data?.payloads ?? []).map((p) => (
            <Button
              key={p.request_id}
              variant={selected === p.request_id ? 'primary' : undefined}
              appearance={selected === p.request_id ? undefined : 'outline'}
              className="justify-start font-mono"
              onClick={() => setSelected(p.request_id)}
            >
              {p.status ?? '?'} · {p.method ?? ''} {p.path ?? ''} · {p.request_id}
            </Button>
          ))}
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>Payload detail</CardTitle>
        </CardHeader>
        <CardContent className="grid gap-2 pt-0 text-sm">
          {!selected && <span>Select a payload to inspect.</span>}
          {detail.data ? (
            <pre className="overflow-auto rounded bg-black/40 p-3 font-mono text-xs">
              {JSON.stringify(detail.data, null, 2)}
            </pre>
          ) : null}
        </CardContent>
      </Card>
    </div>
  );
}
