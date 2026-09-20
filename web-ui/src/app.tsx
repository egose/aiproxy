import { Navigate, Route, Routes } from 'react-router';
import { Shell } from './components/shell';
import { BlocksPage } from './pages/blocks-page';
import { OverviewPage } from './pages/overview-page';
import { PayloadsPage } from './pages/payloads-page';
import { ProvidersPage } from './pages/providers-page';
import { RequestsPage } from './pages/requests-page';
import { TokenPage } from './pages/token-page';

export function App() {
  return (
    <div className="min-h-screen" data-booted="true">
      <Shell>
        <Routes>
          <Route path="/" element={<OverviewPage />} />
          <Route path="/providers" element={<ProvidersPage />} />
          <Route path="/requests" element={<RequestsPage />} />
          <Route path="/payloads" element={<PayloadsPage />} />
          <Route path="/blocks" element={<BlocksPage />} />
          <Route path="/token" element={<TokenPage />} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </Shell>
    </div>
  );
}
