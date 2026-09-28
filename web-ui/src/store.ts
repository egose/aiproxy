import { proxy, subscribe } from 'valtio';
import { advanceAuthGeneration, assertAdminSession } from './auth-lifecycle';

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
  writeAdminSession(accessToken, refreshToken);
  writeAdminWorkspaceId('');
  advanceAuthGeneration(true);
}

function writeAdminSession(accessToken: string, refreshToken: string) {
  adminStore.accessToken = accessToken;
  adminStore.refreshToken = refreshToken;
  persistAdmin(adminAccessKey, accessToken);
  persistAdmin(adminRefreshKey, refreshToken);
}

export function refreshAdminSession(session: number, accessToken: string, refreshToken: string) {
  assertAdminSession(session);
  writeAdminSession(accessToken, refreshToken);
}

export function clearAdminSession() {
  setAdminSession('', '');
}

const adminWorkspaceKey = 'aiproxy.admin-workspace-id';
const adminOrgKey = 'aiproxy.admin-org-id';

function loadAdminWorkspace(): string {
  try {
    return window.localStorage.getItem(adminWorkspaceKey) ?? window.localStorage.getItem(adminOrgKey) ?? '';
  } catch {
    return '';
  }
}

interface AdminWorkspaceState {
  workspaceId: string;
}

export const adminWorkspaceStore = proxy<AdminWorkspaceState>({ workspaceId: loadAdminWorkspace() });

export function setAdminWorkspaceId(workspaceId: string) {
  if (adminWorkspaceStore.workspaceId === workspaceId) return;
  writeAdminWorkspaceId(workspaceId);
  advanceAuthGeneration();
}

function writeAdminWorkspaceId(workspaceId: string) {
  adminWorkspaceStore.workspaceId = workspaceId;
  try {
    if (workspaceId) {
      window.localStorage.setItem(adminWorkspaceKey, workspaceId);
    } else {
      window.localStorage.removeItem(adminWorkspaceKey);
    }
    window.localStorage.removeItem(adminOrgKey);
  } catch {
    // ignore persistence failures
  }
}

export const dashboardStore = proxy<DashboardState>({ token: loadToken() });

export function setDashboardToken(token: string) {
  dashboardStore.token = token;
  advanceAuthGeneration();
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
