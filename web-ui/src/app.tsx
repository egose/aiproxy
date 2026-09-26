import { Navigate, Route, Routes } from 'react-router';
import { Spinner } from '@egose/shadcn-theme/components/ui/spinner';
import { Shell } from './components/shell';
import { useAdminMe, useAdminSession, useAdminStatus, useCurrentOrg } from './hooks';
import { AdminOidcPage } from './pages/admin-oidc-page';
import { AdminOrganizationsPage } from './pages/admin-organizations-page';
import { AliasesPage } from './pages/aliases-page';
import { BlocksPage } from './pages/blocks-page';
import { LoginPage } from './pages/login-page';
import { LogoutPage } from './pages/logout-page';
import { ManageKeysPage } from './pages/manage-keys-page';
import { ManageMembersPage } from './pages/manage-members-page';
import { ManageOrganizationPage } from './pages/manage-organization-page';
import { ManageTeamsPage } from './pages/manage-teams-page';
import { OidcCallbackPage } from './pages/oidc-callback-page';
import { OverviewPage } from './pages/overview-page';
import { PayloadsPage } from './pages/payloads-page';
import { ProvidersPage } from './pages/providers-page';
import { RegisterPage } from './pages/register-page';
import { RequestsPage } from './pages/requests-page';
import { TokenPage } from './pages/token-page';
import { UsersPage } from './pages/users-page';

function RequireAdmin({ children }: { children: React.JSX.Element }) {
  const session = useAdminSession();
  if (!session.accessToken) {
    return <Navigate to="/login" replace />;
  }
  return children;
}

function RequireSystemAdmin({ children }: { children: React.JSX.Element }) {
  const session = useAdminSession();
  const me = useAdminMe(true);
  const { org, orgs } = useCurrentOrg();
  if (!session.accessToken) {
    return <Navigate to="/login" replace />;
  }
  if (me.isPending || orgs.isLoading) {
    return (
      <div className="p-6 text-sm text-slate-500">
        <Spinner size="small">Loading...</Spinner>
      </div>
    );
  }
  if (!me.data?.is_admin || !org || !org.is_system) {
    return <Navigate to="/" replace />;
  }
  return children;
}

export function App() {
  const status = useAdminStatus();
  const session = useAdminSession();
  const multi = status.data?.multi_tenancy_enabled === true;
  const signedIn = !!session.accessToken;
  if (multi && !signedIn) {
    return (
      <div className="min-h-screen" data-booted="true">
        <Shell>
          <Routes>
            <Route path="/login" element={<LoginPage />} />
            <Route path="/register" element={<RegisterPage />} />
            <Route path="/admin/oidc/callback" element={<OidcCallbackPage />} />
            <Route path="*" element={<Navigate to="/login" replace />} />
          </Routes>
        </Shell>
      </div>
    );
  }
  return (
    <div className="min-h-screen" data-booted="true">
      <Shell>
        <Routes>
          <Route path="/" element={<OverviewPage />} />
          <Route path="/providers" element={<ProvidersPage />} />
          <Route path="/aliases" element={<AliasesPage />} />
          <Route path="/requests" element={<RequestsPage />} />
          <Route path="/payloads" element={<PayloadsPage />} />
          <Route path="/blocks" element={<BlocksPage />} />
          {!multi && <Route path="/token" element={<TokenPage />} />}
          {multi && <Route path="/token" element={<Navigate to="/" replace />} />}
          {multi && <Route path="/login" element={<LoginPage />} />}
          {multi && <Route path="/register" element={<RegisterPage />} />}
          {multi && <Route path="/logout" element={<LogoutPage />} />}
          {multi && <Route path="/admin/oidc/callback" element={<OidcCallbackPage />} />}
          {multi && (
            <Route
              path="/manage/organization"
              element={
                <RequireAdmin>
                  <ManageOrganizationPage />
                </RequireAdmin>
              }
            />
          )}
          {multi && (
            <Route
              path="/manage/members"
              element={
                <RequireAdmin>
                  <ManageMembersPage />
                </RequireAdmin>
              }
            />
          )}
          {multi && (
            <Route
              path="/manage/teams"
              element={
                <RequireAdmin>
                  <ManageTeamsPage />
                </RequireAdmin>
              }
            />
          )}
          {multi && (
            <Route
              path="/manage/keys"
              element={
                <RequireAdmin>
                  <ManageKeysPage />
                </RequireAdmin>
              }
            />
          )}
          {multi && <Route path="/admin/keys" element={<Navigate to="/manage/keys" replace />} />}
          {multi && (
            <Route
              path="/admin/oidc"
              element={
                <RequireSystemAdmin>
                  <AdminOidcPage />
                </RequireSystemAdmin>
              }
            />
          )}
          {multi && (
            <Route
              path="/admin/users"
              element={
                <RequireSystemAdmin>
                  <UsersPage />
                </RequireSystemAdmin>
              }
            />
          )}
          {multi && (
            <Route
              path="/admin/organizations"
              element={
                <RequireSystemAdmin>
                  <AdminOrganizationsPage />
                </RequireSystemAdmin>
              }
            />
          )}
          {multi && <Route path="/admin/orgs" element={<Navigate to="/admin/organizations" replace />} />}
          <Route path="/admin/providers" element={<Navigate to="/providers" replace />} />
          <Route path="/admin/aliases" element={<Navigate to="/aliases" replace />} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </Shell>
    </div>
  );
}
