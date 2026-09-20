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
