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
  it('uses the explicit dashboard secret even when an ordinary account is signed in', async () => {
    setAdminSession('ordinary-session', '');
    setDashboardToken('operator-secret');
    const dashboard = vi.spyOn(dashboardClient, 'get').mockResolvedValue({ data: { request_id: 'request' } });
    const admin = vi.spyOn(adminClient, 'get');
    await fetchPayload('request');
    expect(dashboard).toHaveBeenCalledWith('/_internal/dashboard/payloads/request');
    expect(admin).not.toHaveBeenCalled();
  });

  it('uses the account session when no dashboard secret is configured', async () => {
    setAdminSession('operator-session', '');
    setDashboardToken('');
    const admin = vi.spyOn(adminClient, 'get').mockResolvedValue({ data: { request_id: 'request' } });
    const dashboard = vi.spyOn(dashboardClient, 'get');
    await fetchPayload('request');
    expect(admin).toHaveBeenCalledWith('/_internal/dashboard/payloads/request');
    expect(dashboard).not.toHaveBeenCalled();
  });

  it('distinguishes operator denial from invalid authentication without masking admin permission errors', () => {
    const denied = { response: { status: 403, data: 'dashboard operator access required\n' } };
    expect(errorMessage(denied)).toContain('only to system administrators or dashboard-token holders');
    expect(isUnauthorized(denied)).toBe(false);
    expect(errorMessage({ response: { status: 403, data: 'workspace admin required' } })).toBe(
      'workspace admin required',
    );
    expect(errorMessage({ response: { status: 401, data: 'unauthorized\n' } })).toContain('Sign in again');
  });
});
