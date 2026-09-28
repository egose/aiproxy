import { cleanup, fireEvent, render, screen, within } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';

import { DataTable, type ManagementColumnDef } from './data-table';

afterEach(cleanup);

type Row = { name: string; type: string };

const columns: ManagementColumnDef<Row>[] = [
  { accessorKey: 'name', header: 'Name' },
  { accessorKey: 'type', header: 'Type' },
];

const data: Row[] = [
  { name: 'bravo', type: 'openai' },
  { name: 'alpha', type: 'gemini' },
  { name: 'charlie', type: 'openai' },
];

function renderTable() {
  return render(<DataTable columns={columns} data={data} filterPlaceholder="Filter..." getRowId={(row) => row.name} />);
}

function bodyNames(): string[] {
  const rows = screen.getAllByRole('row').slice(1);
  return rows.map((r) => within(r).queryAllByRole('cell')[0]?.textContent ?? '').filter((t) => t !== 'No rows.');
}

describe('DataTable', () => {
  it('renders all rows unsorted by default', () => {
    renderTable();
    expect(bodyNames()).toEqual(['bravo', 'alpha', 'charlie']);
  });

  it('sorts ascending then descending when the header is clicked', () => {
    renderTable();
    const header = screen.getByRole('columnheader', { name: /Name/ });
    fireEvent.click(header);
    expect(bodyNames()).toEqual(['alpha', 'bravo', 'charlie']);
    fireEvent.click(header);
    expect(bodyNames()).toEqual(['charlie', 'bravo', 'alpha']);
  });

  it('filters rows by the global filter input', () => {
    renderTable();
    fireEvent.change(screen.getByPlaceholderText('Filter...'), { target: { value: 'openai' } });
    expect(bodyNames()).toEqual(['bravo', 'charlie']);
  });
});
