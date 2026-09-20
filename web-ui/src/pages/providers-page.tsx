import { useMemo, useState } from 'react';
import { Card, CardContent, CardHeader, CardTitle } from '@egose/shadcn-theme/components/ui/card';
import { Input } from '@egose/shadcn-theme/components/ui/input';
import {
  createColumnHelper,
  flexRender,
  getCoreRowModel,
  getFilteredRowModel,
  getSortedRowModel,
  useReactTable,
  type SortingState,
} from '@tanstack/react-table';
import { useSnapshot } from '../hooks';
import type { Provider } from '../types';

const columnHelper = createColumnHelper<Provider>();

export function ProvidersPage() {
  const snapshot = useSnapshot();
  const [sorting, setSorting] = useState<SortingState>([]);
  const [filter, setFilter] = useState('');

  const columns = useMemo(
    () => [
      columnHelper.accessor('name', { header: 'Provider' }),
      columnHelper.accessor('type', { header: 'Type' }),
      columnHelper.accessor((row) => row.models.length, { id: 'models', header: 'Models' }),
      columnHelper.accessor((row) => row.display_name ?? '', { id: 'display', header: 'Display name' }),
    ],
    [],
  );

  const rows = useMemo(() => snapshot.data?.providers ?? [], [snapshot.data]);

  const table = useReactTable({
    data: rows,
    columns,
    state: { sorting, globalFilter: filter },
    onSortingChange: setSorting,
    onGlobalFilterChange: setFilter,
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
  });

  return (
    <div className="grid w-full gap-6 p-6">
      <Card>
        <CardHeader>
          <CardTitle>Providers ({rows.length})</CardTitle>
        </CardHeader>
        <CardContent className="grid gap-4 pt-0">
          <Input placeholder="Filter providers..." value={filter} onChange={(e) => setFilter(e.target.value)} />
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                {table.getHeaderGroups().map((group) => (
                  <tr key={group.id}>
                    {group.headers.map((header) => (
                      <th
                        key={header.id}
                        className="cursor-pointer px-2 py-1 text-left font-medium"
                        onClick={header.column.getToggleSortingHandler()}
                      >
                        {flexRender(header.column.columnDef.header, header.getContext())}
                      </th>
                    ))}
                  </tr>
                ))}
              </thead>
              <tbody>
                {table.getRowModel().rows.map((row) => (
                  <tr key={row.id} className="border-t">
                    {row.getVisibleCells().map((cell) => (
                      <td key={cell.id} className="px-2 py-1 font-mono">
                        {flexRender(cell.column.columnDef.cell, cell.getContext())}
                      </td>
                    ))}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <div className="grid gap-2">
            {(snapshot.data?.aliases ?? []).map((alias) => (
              <div key={alias.name} className="text-sm">
                <span className="font-mono">alias/{alias.name}</span>
                <span> ({alias.algorithm}) → </span>
                <span className="font-mono">{alias.targets.map((t) => `${t.provider}/${t.model}`).join(', ')}</span>
              </div>
            ))}
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
