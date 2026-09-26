import { render, screen } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { DialogManagerProvider } from '@egose/shadcn-theme/components/widgets/dialog-manager';
import { MemoryRouter } from 'react-router';
import { describe, expect, it } from 'vitest';

import { App } from './app';

function renderApp(path = '/') {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <DialogManagerProvider>
        <MemoryRouter initialEntries={[path]}>
          <App />
        </MemoryRouter>
      </DialogManagerProvider>
    </QueryClientProvider>,
  );
}

describe('dashboard shell', () => {
  it('renders navigation and overview', async () => {
    renderApp('/');
    expect(await screen.findByText('Providers')).toBeInTheDocument();
  });

  it('renders the token page', async () => {
    renderApp('/token');
    expect(await screen.findByText('Dashboard token')).toBeInTheDocument();
  });
});
