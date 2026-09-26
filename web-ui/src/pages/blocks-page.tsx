import { zodResolver } from '@hookform/resolvers/zod';
import { HookFormSelect } from '@egose/shadcn-theme/components/form/hook-select';
import { Alert, AlertDescription } from '@egose/shadcn-theme/components/ui/alert';
import { Button } from '@egose/shadcn-theme/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@egose/shadcn-theme/components/ui/card';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import { FormProvider, useForm } from 'react-hook-form';
import { z } from 'zod';
import { useBlock, useBlocks } from '../hooks';
import { decideBlock, errorMessage, type BlockDecisionAction } from '../services/dashboard';

const decisionSchema = z.object({
  action: z.enum(['allow', 'redact', 'deny']),
});

type DecisionForm = z.infer<typeof decisionSchema>;

export function BlocksPage() {
  const [selected, setSelected] = useState<string | null>(null);
  const [result, setResult] = useState<string | null>(null);
  const queryClient = useQueryClient();
  const list = useBlocks();
  const detail = useBlock(selected);
  const form = useForm<DecisionForm>({ resolver: zodResolver(decisionSchema), defaultValues: { action: 'redact' } });

  const decision = useMutation({
    mutationFn: async (action: BlockDecisionAction) => {
      if (!detail.data) throw new Error('Load a block first.');
      const shas = detail.data.findings.map((f) => f.secret_sha256).filter((s): s is string => !!s);
      if (shas.length === 0) throw new Error('Block has no secret fingerprints to decide on.');
      return decideBlock(detail.data.block_id, action, shas);
    },
    onSuccess: (res) => {
      setResult(`Recorded ${res.action} for ${res.count} finding(s).`);
      setSelected(null);
      queryClient.invalidateQueries({ queryKey: ['dashboard', 'blocks'] });
      queryClient.invalidateQueries({ queryKey: ['dashboard', 'block'] });
    },
    onError: (err) => setResult(errorMessage(err)),
  });

  return (
    <div className="grid w-full gap-6 p-6 lg:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
      <Card>
        <CardHeader>
          <CardTitle>Guardrail blocks</CardTitle>
        </CardHeader>
        <CardContent className="grid gap-2 pt-0 text-sm">
          {list.error && (
            <Alert variant="danger">
              <AlertDescription>{errorMessage(list.error)}</AlertDescription>
            </Alert>
          )}
          {list.data && !list.data.enabled && <span>Guardrail quarantine is not enabled on this server.</span>}
          {(list.data?.blocks ?? []).map((b) => (
            <Button
              key={b.block_id}
              variant={selected === b.block_id ? 'primary' : undefined}
              appearance={selected === b.block_id ? undefined : 'outline'}
              className="justify-start font-mono"
              onClick={() => {
                setSelected(b.block_id);
                setResult(null);
              }}
            >
              {b.block_id} · {b.finding_count} finding(s)
            </Button>
          ))}
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>Block decision</CardTitle>
        </CardHeader>
        <CardContent className="grid gap-4 pt-0 text-sm">
          {!detail.data && <span>Select a block. Opening it consumes the quarantine capture server-side.</span>}
          {detail.data && (
            <>
              <div className="font-mono text-xs">{JSON.stringify(detail.data, null, 2).slice(0, 4000)}</div>
              <FormProvider {...form}>
                <form className="grid gap-2" onSubmit={form.handleSubmit((values) => decision.mutate(values.action))}>
                  <HookFormSelect<DecisionForm>
                    name="action"
                    label="Action"
                    data={[
                      { label: 'allow', value: 'allow' },
                      { label: 'redact', value: 'redact' },
                      { label: 'deny', value: 'deny' },
                    ]}
                  />
                  <Button variant="primary" type="submit" disabled={decision.isPending}>
                    {decision.isPending ? 'Recording...' : 'Record decision'}
                  </Button>
                </form>
              </FormProvider>
            </>
          )}
          {result && (
            <Alert variant="info">
              <AlertDescription>{result}</AlertDescription>
            </Alert>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
