import { useMemo, useState } from 'react';
import { Card, CardContent, CardHeader, CardTitle } from '@egose/shadcn-theme/components/ui/card';
import { Input } from '@egose/shadcn-theme/components/ui/input';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@egose/shadcn-theme/components/ui/table';
import { useSnapshot } from '../hooks';
import type { Recent } from '../types';

type RequestSortKey = 'Model' | 'Operation' | 'StatusCode' | 'Provider' | 'TotalTokens' | 'Client';
type RequestSort = { key: RequestSortKey; desc: boolean };

const requestHeaders: Array<{ key: RequestSortKey; label: string }> = [
  { key: 'Model', label: 'Model' },
  { key: 'Operation', label: 'Op' },
  { key: 'StatusCode', label: 'Status' },
  { key: 'Provider', label: 'Provider' },
  { key: 'TotalTokens', label: 'Tokens' },
  { key: 'Client', label: 'Client' },
];

function cellValue(row: Recent, key: RequestSortKey): string {
  switch (key) {
    case 'Model':
      return row.Model ?? '';
    case 'Operation':
      return row.Operation ?? '';
    case 'StatusCode':
      return String(row.StatusCode ?? '');
    case 'Provider':
      return row.Provider ?? '';
    case 'TotalTokens':
      return String(row.TotalTokens ?? '');
    case 'Client':
      return row.Client ?? '';
  }
}

export function RequestsPage() {
  const snapshot = useSnapshot();
  const [sorting, setSorting] = useState<RequestSort>({ key: 'Model', desc: false });
  const [filter, setFilter] = useState('');

  const rows = useMemo(() => {
    const q = filter.trim().toLowerCase();
    const all = snapshot.data?.recent ?? [];
    const filtered =
      q === '' ? all : all.filter((r) => requestHeaders.some((h) => cellValue(r, h.key).toLowerCase().includes(q)));
    const dir = sorting.desc ? -1 : 1;
    return [...filtered].sort((a, b) => {
      const va = cellValue(a, sorting.key);
      const vb = cellValue(b, sorting.key);
      const na = Number(va);
      const nb = Number(vb);
      if (sorting.key === 'StatusCode' || sorting.key === 'TotalTokens') {
        return ((Number.isFinite(na) ? na : 0) - (Number.isFinite(nb) ? nb : 0)) * dir;
      }
      if (va < vb) return -dir;
      if (va > vb) return dir;
      return 0;
    });
  }, [snapshot.data, filter, sorting]);

  const toggleSort = (key: RequestSortKey) => {
    setSorting((prev) => {
      if (prev.key !== key) return { key, desc: false };
      return { key, desc: !prev.desc };
    });
  };

  return (
    <div className="grid w-full gap-6 p-6">
      <Card>
        <CardHeader>
          <CardTitle>Recent requests ({rows.length})</CardTitle>
        </CardHeader>
        <CardContent className="grid gap-4 pt-0">
          <Input placeholder="Filter requests..." value={filter} onChange={(e) => setFilter(e.target.value)} />
          <div className="overflow-x-auto">
            <Table>
              <TableHeader>
                <TableRow>
                  {requestHeaders.map((h) => (
                    <TableHead key={h.key} className="cursor-pointer" onClick={() => toggleSort(h.key)}>
                      {h.label}
                      {sorting.key === h.key ? (sorting.desc ? ' ▼' : ' ▲') : ''}
                    </TableHead>
                  ))}
                </TableRow>
              </TableHeader>
              <TableBody>
                {rows.map((r, i) => (
                  <TableRow key={`${r.Model}-${r.Client}-${i}`}>
                    {requestHeaders.map((h) => (
                      <TableCell key={h.key} className="font-mono">
                        {cellValue(r, h.key)}
                      </TableCell>
                    ))}
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
