import { afterEach, describe, expect, it, vi } from 'vitest';
import { clearAdminSession, setAdminSession, setDashboardToken } from '../store';
import { adminClient } from './admin';
import { dashboardClient } from './client';
import { errorMessage, fetchPayload, isUnauthorized } from './dashboard';

afterEach(() => {
  vi.restoreAllMocks();
  clearAdminSession();
  setDashboardToken('');
});

describe('dashboard operator credentials', () => {
  it('uses the dashboard secret in token mode even with a saved account session', async () => {
    setAdminSession('saved-session', '');
    setDashboardToken('operator-secret');
    const dashboard = vi.spyOn(dashboardClient, 'get').mockResolvedValue({ data: { request_id: 'request' } });
    const admin = vi.spyOn(adminClient, 'get');
    await fetchPayload('token', 'request');
    expect(dashboard).toHaveBeenCalledWith('/_internal/dashboard/payloads/request');
    expect(admin).not.toHaveBeenCalled();
  });

  it('uses the account session in session mode even with a stored dashboard token', async () => {
    setAdminSession('operator-session', '');
    setDashboardToken('stale-operator-secret');
    const admin = vi.spyOn(adminClient, 'get').mockResolvedValue({ data: { request_id: 'request' } });
    const dashboard = vi.spyOn(dashboardClient, 'get');
    await fetchPayload('session', 'request');
    expect(admin).toHaveBeenCalledWith('/_internal/dashboard/payloads/request');
    expect(dashboard).not.toHaveBeenCalled();
  });

  it('sends no request when access has not been established', async () => {
    const admin = vi.spyOn(adminClient, 'get');
    const dashboard = vi.spyOn(dashboardClient, 'get');
    await expect(fetchPayload(undefined, 'request')).rejects.toThrow('Dashboard access required');
    expect(admin).not.toHaveBeenCalled();
    expect(dashboard).not.toHaveBeenCalled();
  });

  it('distinguishes operator denial from invalid authentication without masking admin permission errors', () => {
    const denied = { response: { status: 403, data: 'dashboard operator access required\n' } };
    expect(errorMessage(denied, 'session')).toContain('only to system administrators.');
    expect(isUnauthorized(denied)).toBe(false);
    expect(errorMessage({ response: { status: 403, data: 'workspace admin required' } })).toBe(
      'workspace admin required',
    );
    const unauthorized = { response: { status: 401, data: 'unauthorized\n' } };
    expect(errorMessage(unauthorized, 'session')).toBe('Authentication failed. Sign in again.');
    expect(errorMessage(unauthorized, 'token')).toBe('Authentication failed. Check your dashboard token.');
  });
});
