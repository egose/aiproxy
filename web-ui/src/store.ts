import { proxy, subscribe } from 'valtio';

const tokenKey = 'aiproxy.dashboard-token';

function loadToken(): string {
  try {
    return window.localStorage.getItem(tokenKey) ?? '';
  } catch {
    return '';
  }
}

interface DashboardState {
  token: string;
}

const adminAccessKey = 'aiproxy.admin-access-token';
const adminRefreshKey = 'aiproxy.admin-refresh-token';

function loadAdmin(key: string): string {
  try {
    return window.localStorage.getItem(key) ?? '';
  } catch {
    return '';
  }
}

interface AdminState {
  accessToken: string;
  refreshToken: string;
}

export const adminStore = proxy<AdminState>({
  accessToken: loadAdmin(adminAccessKey),
  refreshToken: loadAdmin(adminRefreshKey),
});

function persistAdmin(key: string, value: string) {
  try {
    if (value) {
      window.localStorage.setItem(key, value);
    } else {
      window.localStorage.removeItem(key);
    }
  } catch {
    // ignore persistence failures
  }
}

export function setAdminSession(accessToken: string, refreshToken: string) {
  adminStore.accessToken = accessToken;
  adminStore.refreshToken = refreshToken;
  persistAdmin(adminAccessKey, accessToken);
  persistAdmin(adminRefreshKey, refreshToken);
}

export function clearAdminSession() {
  adminStore.accessToken = '';
  adminStore.refreshToken = '';
  persistAdmin(adminAccessKey, '');
  persistAdmin(adminRefreshKey, '');
  setAdminOrgId('');
}

const adminOrgKey = 'aiproxy.admin-org-id';

function loadAdminOrg(): string {
  try {
    return window.localStorage.getItem(adminOrgKey) ?? '';
  } catch {
    return '';
  }
}

interface AdminOrgState {
  orgId: string;
}

export const adminOrgStore = proxy<AdminOrgState>({ orgId: loadAdminOrg() });

export function setAdminOrgId(orgId: string) {
  adminOrgStore.orgId = orgId;
  try {
    if (orgId) {
      window.localStorage.setItem(adminOrgKey, orgId);
    } else {
      window.localStorage.removeItem(adminOrgKey);
    }
  } catch {
    // ignore persistence failures
  }
}

export const dashboardStore = proxy<DashboardState>({ token: loadToken() });

export function setDashboardToken(token: string) {
  dashboardStore.token = token;
  try {
    if (token) {
      window.localStorage.setItem(tokenKey, token);
    } else {
      window.localStorage.removeItem(tokenKey);
    }
  } catch {
    // localStorage unavailable (private mode); keep token in memory only.
  }
}

subscribe(dashboardStore, () => {
  try {
    if (dashboardStore.token) {
      window.localStorage.setItem(tokenKey, dashboardStore.token);
    } else {
      window.localStorage.removeItem(tokenKey);
    }
  } catch {
    // ignore persistence failures
  }
});
