import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { BrowserRouter } from 'react-router';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { TooltipProvider } from '@egose/shadcn-theme/components/ui/tooltip';
import { DialogManagerProvider } from '@egose/shadcn-theme/components/widgets/dialog-manager';

import './index.css';

import { App } from './app';

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 5_000,
      refetchOnWindowFocus: false,
    },
  },
});

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <TooltipProvider>
        <DialogManagerProvider>
          <BrowserRouter>
            <App />
          </BrowserRouter>
        </DialogManagerProvider>
      </TooltipProvider>
    </QueryClientProvider>
  </StrictMode>,
);
