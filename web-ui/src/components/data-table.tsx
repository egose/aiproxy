import { Fragment, useState, type ReactNode } from 'react';
import {
  columnFilteringFeature,
  createFilteredRowModel,
  createSortedRowModel,
  filterFn_includesString,
  flexRender,
  globalFilteringFeature,
  rowSortingFeature,
  tableFeatures,
  useTable,
  type ColumnDef,
  type SortingState,
} from '@tanstack/react-table';
import { Input } from '@egose/shadcn-theme/components/ui/input';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@egose/shadcn-theme/components/ui/table';

export const managementTableFeatures = tableFeatures({
  rowSortingFeature,
  columnFilteringFeature,
  globalFilteringFeature,
  sortedRowModel: createSortedRowModel(),
  filteredRowModel: createFilteredRowModel(),
  filterFns: { includesString: filterFn_includesString },
});

export type ManagementColumnDef<TData> = ColumnDef<typeof managementTableFeatures, TData>;

export function DataTable<TData>({
  columns,
  data,
  filterPlaceholder = 'Filter...',
  getRowId,
  expandedIds,
  renderExpanded,
  emptyText = 'No rows.',
}: {
  columns: ManagementColumnDef<TData>[];
  data: TData[];
  filterPlaceholder?: string;
  getRowId?: (originalRow: TData, index: number) => string;
  expandedIds?: Set<string>;
  renderExpanded?: (row: TData) => ReactNode | null;
  emptyText?: string;
}) {
  const [sorting, setSorting] = useState<SortingState>([]);
  const [globalFilter, setGlobalFilter] = useState('');

  const table = useTable({
    features: managementTableFeatures,
    columns,
    data,
    state: { sorting, globalFilter },
    onSortingChange: setSorting,
    onGlobalFilterChange: setGlobalFilter,
    globalFilterFn: 'includesString',
    getRowId,
  });

  const rows = table.getRowModel().rows;
  const colCount = table.getAllLeafColumns().length;

  return (
    <div className="grid gap-2">
      <Input placeholder={filterPlaceholder} value={globalFilter} onChange={(e) => setGlobalFilter(e.target.value)} />
      <div className="overflow-x-auto">
        <Table>
          <TableHeader>
            {table.getHeaderGroups().map((hg) => (
              <TableRow key={hg.id}>
                {hg.headers.map((h) => (
                  <TableHead
                    key={h.id}
                    className={h.column.getCanSort() ? 'cursor-pointer select-none' : ''}
                    onClick={h.column.getCanSort() ? h.column.getToggleSortingHandler() : undefined}
                  >
                    {flexRender(h.column.columnDef.header, h.getContext())}
                    {h.column.getIsSorted() === 'asc' ? ' ▲' : h.column.getIsSorted() === 'desc' ? ' ▼' : ''}
                  </TableHead>
                ))}
              </TableRow>
            ))}
          </TableHeader>
          <TableBody>
            {rows.map((row) => (
              <Fragment key={row.id}>
                <TableRow>
                  {row.getAllCells().map((cell) => (
                    <TableCell key={cell.id}>{flexRender(cell.column.columnDef.cell, cell.getContext())}</TableCell>
                  ))}
                </TableRow>
                {expandedIds?.has(row.id) && renderExpanded ? (
                  <TableRow>
                    <TableCell colSpan={colCount}>{renderExpanded(row.original)}</TableCell>
                  </TableRow>
                ) : null}
              </Fragment>
            ))}
            {rows.length === 0 && (
              <TableRow>
                <TableCell colSpan={colCount} className="text-sm text-slate-500">
                  {emptyText}
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </div>
    </div>
  );
}
