import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useEffect, useState } from 'react';
import { Alert, AlertDescription } from '@egose/shadcn-theme/components/ui/alert';
import { Button } from '@egose/shadcn-theme/components/ui/button';
import { Input } from '@egose/shadcn-theme/components/ui/input';
import { Label } from '@egose/shadcn-theme/components/ui/label';
import { Spinner } from '@egose/shadcn-theme/components/ui/spinner';
import { useScopeQuota } from '../hooks';
import { updateScopeQuota } from '../services/admin';
import { errorMessage } from '../services/dashboard';
import type { AdminKeyQuota } from '../types';
import { useConfirm } from './dialogs';

export function formatMicros(micros: number): string {
  if (!Number.isFinite(micros) || micros <= 0) return '$0';
  const dollars = micros / 1e6;
  return '$' + (dollars >= 0.01 ? dollars.toFixed(2) : dollars.toFixed(6));
}

export function KeyQuotaLine({
  usage,
  quota,
}: {
  usage: { requests: number; tokens: number; spend_micros: number };
  quota?: AdminKeyQuota | null;
}) {
  const hasUsage = usage.requests > 0 || usage.tokens > 0 || usage.spend_micros > 0;
  const tpm = quota?.tpm ?? [];
  if (!hasUsage && (!quota || (quota.budget_micros <= 0 && tpm.length === 0))) return null;
  return (
    <>
      {hasUsage && (
        <span className="font-mono text-xs text-slate-500">
          {usage.requests} req · {usage.tokens} tok · {formatMicros(usage.spend_micros)}
        </span>
      )}
      {quota && quota.budget_micros > 0 && (
        <span
          className="font-mono text-xs text-slate-500"
          title={`Budget spend ${formatMicros(quota.spend_micros)} of ${formatMicros(quota.budget_micros)}`}
        >
          budget {formatMicros(quota.spend_micros)} / {formatMicros(quota.budget_micros)}
        </span>
      )}
      {tpm.map((t) => {
        const limit = t.effective > 0 ? t.effective : t.ceiling;
        return (
          <span
            key={t.model}
            className="font-mono text-xs text-slate-500"
            title={`${t.model}: ${t.used_tokens} tokens in the last minute${limit > 0 ? `, limit ${limit}` : ''}`}
          >
            {t.model}: {t.used_tokens}
            {limit > 0 ? `/${limit}` : ''} tpm
          </span>
        );
      })}
    </>
  );
}

type TPMRow = { model: string; ceiling: string; effective: string; isNew: boolean };

function parsePositiveInt(raw: string): number | null {
  const trimmed = raw.trim();
  if (trimmed === '') return null;
  const value = Number(trimmed);
  if (!Number.isInteger(value) || value < 0) return null;
  return value;
}

export function QuotaEditor({
  orgId,
  scope,
  scopeId,
  canEditPolicy,
  canEditEffective,
}: {
  orgId: string;
  scope: 'users' | 'teams';
  scopeId: string;
  canEditPolicy: boolean;
  canEditEffective: boolean;
}) {
  const quota = useScopeQuota(orgId, scope, scopeId, true);
  const queryClient = useQueryClient();
  const confirm = useConfirm();
  const [budget, setBudget] = useState('');
  const [rows, setRows] = useState<TPMRow[]>([]);
  const [ready, setReady] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (quota.data && !ready) {
      setBudget(quota.data.budget_micros > 0 ? String(quota.data.budget_micros / 1e6) : '');
      setRows(
        quota.data.tpm.map((t) => ({
          model: t.model,
          ceiling: t.ceiling > 0 ? String(t.ceiling) : '',
          effective: t.effective > 0 ? String(t.effective) : '',
          isNew: false,
        })),
      );
      setReady(true);
    }
  }, [quota.data, ready]);

  const invalidate = () =>
    queryClient.invalidateQueries({ queryKey: ['admin', 'orgs', orgId, scope, scopeId, 'quota'] });

  const saveMutation = useMutation({
    mutationFn: async () => {
      const body: Record<string, unknown> = {};
      if (canEditPolicy && budget.trim() !== '') {
        const dollars = Number(budget.trim());
        if (!Number.isFinite(dollars) || dollars < 0) throw new Error('budget must be a non-negative dollar amount');
        body.budget_micros = Math.round(dollars * 1e6);
      }
      body.tpm = rows
        .filter((r) => r.model.trim() !== '')
        .map((r) => {
          const entry: Record<string, unknown> = { model: r.model.trim() };
          if (canEditPolicy && r.ceiling.trim() !== '') {
            const ceiling = parsePositiveInt(r.ceiling);
            if (ceiling === null) throw new Error(`ceiling for ${r.model.trim()} must be a non-negative integer`);
            entry.ceiling = ceiling;
          }
          if (canEditEffective && r.effective.trim() !== '') {
            const effective = parsePositiveInt(r.effective);
            if (effective === null) throw new Error(`limit for ${r.model.trim()} must be a non-negative integer`);
            entry.effective = effective;
          }
          return entry;
        });
      return updateScopeQuota(orgId, scope, scopeId, body);
    },
    onSuccess: async () => {
      setError(null);
      await invalidate();
    },
    onError: (err) => setError(errorMessage(err)),
  });

  const resetMutation = useMutation({
    mutationFn: async () => updateScopeQuota(orgId, scope, scopeId, { reset_spend: true }),
    onSuccess: invalidate,
    onError: (err) => setError(errorMessage(err)),
  });

  const askReset = async () => {
    const ok = await confirm({
      title: 'Reset spend',
      description: 'Reset the recorded spend for this budget to zero? Quota rows are kept.',
      confirmText: 'Reset',
    });
    if (ok) resetMutation.mutate();
  };

  const setRow = (index: number, patch: Partial<TPMRow>) => {
    setRows((prev) => prev.map((r, i) => (i === index ? { ...r, ...patch } : r)));
  };

  if (quota.isLoading) {
    return (
      <div className="text-sm text-slate-500">
        <Spinner size="small">Loading quotas...</Spinner>
      </div>
    );
  }
  if (quota.error) {
    return (
      <Alert variant="danger">
        <AlertDescription>{errorMessage(quota.error)}</AlertDescription>
      </Alert>
    );
  }

  const spent = quota.data?.spend_micros ?? 0;
  const budgetMicros = quota.data?.budget_micros ?? 0;
  const usedByModel = new Map((quota.data?.tpm ?? []).map((t) => [t.model, t.used_tokens]));

  return (
    <div className="grid gap-3 rounded-md border p-3">
      {error && (
        <Alert variant="danger">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}
      <div className="grid max-w-xs gap-1">
        <Label>Budget (USD, empty = unlimited)</Label>
        <Input
          value={budget}
          disabled={!canEditPolicy || saveMutation.isPending}
          onChange={(e) => setBudget(e.target.value)}
          placeholder="e.g. 25"
          inputMode="decimal"
        />
        <span className="text-xs text-slate-500">
          Spent {formatMicros(spent)}
          {budgetMicros > 0 ? ` of ${formatMicros(budgetMicros)}` : ''}. Requests over budget are blocked.
        </span>
      </div>
      <div className="grid gap-2">
        <span className="text-sm font-medium">Tokens per minute by model (empty = unlimited)</span>
        {rows.map((row, i) => (
          <div key={i} className="grid grid-cols-[1fr_1fr_1fr_auto] items-end gap-2">
            <div className="grid gap-1">
              <Label>Model</Label>
              <Input
                value={row.model}
                disabled={!row.isNew || saveMutation.isPending}
                onChange={(e) => setRow(i, { model: e.target.value })}
                placeholder="alias/fast"
                className="font-mono"
              />
            </div>
            <div className="grid gap-1">
              <Label>Admin ceiling</Label>
              <Input
                value={row.ceiling}
                disabled={!canEditPolicy || saveMutation.isPending}
                onChange={(e) => setRow(i, { ceiling: e.target.value })}
                placeholder="60000"
                inputMode="numeric"
                className="font-mono"
              />
            </div>
            <div className="grid gap-1">
              <Label>Limit</Label>
              <Input
                value={row.effective}
                disabled={!canEditEffective || saveMutation.isPending}
                onChange={(e) => setRow(i, { effective: e.target.value })}
                placeholder="30000"
                inputMode="numeric"
                className="font-mono"
              />
            </div>
            <div className="pb-1 text-xs text-slate-500">
              {usedByModel.has(row.model.trim()) && row.model.trim() !== '' ? (
                `${usedByModel.get(row.model.trim())} used`
              ) : row.isNew ? (
                <Button
                  appearance="outline"
                  type="button"
                  disabled={saveMutation.isPending}
                  onClick={() => setRows((prev) => prev.filter((_, j) => j !== i))}
                >
                  Remove
                </Button>
              ) : (
                ''
              )}
            </div>
          </div>
        ))}
        {(canEditPolicy || canEditEffective) && (
          <div>
            <Button
              appearance="outline"
              type="button"
              disabled={saveMutation.isPending}
              onClick={() => setRows((prev) => [...prev, { model: '', ceiling: '', effective: '', isNew: true }])}
            >
              Add model
            </Button>
          </div>
        )}
      </div>
      <div className="flex gap-2">
        {(canEditPolicy || canEditEffective) && (
          <Button
            variant="primary"
            type="button"
            disabled={saveMutation.isPending}
            onClick={() => saveMutation.mutate()}
          >
            {saveMutation.isPending ? 'Saving...' : 'Save quotas'}
          </Button>
        )}
        {canEditPolicy && (
          <Button appearance="outline" type="button" disabled={resetMutation.isPending} onClick={askReset}>
            {resetMutation.isPending ? 'Resetting...' : 'Reset spend'}
          </Button>
        )}
      </div>
    </div>
  );
}
