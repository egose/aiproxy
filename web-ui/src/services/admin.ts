import axios from 'axios';
import { z } from 'zod';

import {
  adminWorkspaceStore,
  adminStore,
  clearAdminSession,
  refreshAdminSession,
  setAdminSession,
  setDashboardToken,
} from '../store';
import {
  assertAdminSession,
  assertAuthGeneration,
  authLifecycle,
  guardAuthClient,
  type AuthRequestConfig,
} from '../auth-lifecycle';
import {
  adminAliasSchema,
  adminAliasesSchema,
  adminInviteSchema,
  adminInvitesSchema,
  adminKeySchema,
  adminKeysSchema,
  adminMeSchema,
  adminOIDCConfigSchema,
  adminWorkspaceMemberSchema,
  adminWorkspaceSchema,
  adminWorkspacesSchema,
  adminProviderSchema,
  adminProvidersSchema,
  adminQuotaSchema,
  adminStatusSchema,
  adminTeamMemberSchema,
  adminTeamSchema,
  adminTeamsSchema,
  adminUserSchema,
  adminUsersSchema,
  copilotDeviceFlowStatusSchema,
  providerTypesSchema,
  type AdminAlias,
  type AdminInvite,
  type AdminKey,
  type AdminMe,
  type AdminOIDCConfig,
  type AdminWorkspace,
  type AdminWorkspaceMember,
  type AdminProvider,
  type AdminQuota,
  type AdminStatus,
  type AdminTeam,
  type AdminTeamMember,
  type AdminUser,
  type CopilotDeviceFlowStatus,
  type ProviderTypeInfo,
} from '../types';

export const adminStatusPath = '/_internal/admin/status';

export const adminWorkspaceHeader = 'X-Workspace-ID';

export const adminClient = axios.create({
  timeout: 10_000,
  headers: { 'Content-Type': 'application/json' },
});

guardAuthClient(adminClient);

adminClient.interceptors.request.use(
  (config: AuthRequestConfig) => {
    const token = adminStore.accessToken;
    config.authAccessToken = token;
    if (token) {
      config.headers = config.headers ?? {};
      config.headers.Authorization = `Bearer ${token}`;
    }
    const workspaceId = adminWorkspaceStore.workspaceId;
    if (workspaceId !== '' && typeof config.url === 'string' && config.url.startsWith('/_internal/admin/')) {
      config.headers = config.headers ?? {};
      if (config.headers[adminWorkspaceHeader] == null) {
        config.headers[adminWorkspaceHeader] = workspaceId;
      }
    }
    return config;
  },
  undefined,
  { synchronous: true },
);

let refresh: { session: number; promise: Promise<void> } | undefined;

function refreshSession(session: number): Promise<void> {
  assertAdminSession(session);
  if (refresh?.session === session) return refresh.promise;
  const refreshToken = adminStore.refreshToken;
  const promise = (async () => {
    try {
      const res = await axios.post<{ access_token: string; refresh_token: string }>(
        '/_internal/admin/refresh',
        { refresh_token: refreshToken },
        { timeout: 10_000 },
      );
      refreshAdminSession(session, res.data.access_token, res.data.refresh_token);
    } catch (error) {
      assertAdminSession(session);
      clearAdminSession();
      throw error;
    } finally {
      if (refresh?.session === session) refresh = undefined;
    }
  })();
  refresh = { session, promise };
  return promise;
}

adminClient.interceptors.response.use(
  (res) => res,
  async (err) => {
    const original = err?.config as AuthRequestConfig | undefined;
    if (err?.response?.status === 401 && original && !original._retried && adminStore.refreshToken) {
      assertAuthGeneration(original.authGeneration!);
      original._retried = true;
      if (original.authAccessToken === adminStore.accessToken) {
        await refreshSession(original.authSession!);
      }
      assertAuthGeneration(original.authGeneration!);
      return adminClient.request(original);
    }
    return Promise.reject(err);
  },
);

export async function fetchAdminStatus(): Promise<AdminStatus> {
  const res = await axios.get<unknown>(adminStatusPath, { timeout: 10_000 });
  return adminStatusSchema.parse(res.data);
}

export async function adminLogin(email: string, password: string) {
  const session = authLifecycle.session;
  const res = await axios.post<{ access_token: string; refresh_token: string }>(
    '/_internal/admin/login',
    { email, password },
    { timeout: 10_000 },
  );
  assertAdminSession(session);
  setAdminSession(res.data.access_token, res.data.refresh_token);
  return res.data;
}

export async function adminLogout() {
  const accessToken = adminStore.accessToken;
  const refreshToken = adminStore.refreshToken;
  clearAdminSession();
  setDashboardToken('');
  await axios.post(
    '/_internal/admin/logout',
    { refresh_token: refreshToken || undefined },
    {
      timeout: 10_000,
      headers: { Authorization: `Bearer ${accessToken}` },
    },
  );
}

export async function fetchAdminMe(): Promise<AdminMe> {
  const res = await adminClient.get<unknown>('/_internal/admin/me');
  return adminMeSchema.parse(res.data);
}

export async function fetchAdminProviders(): Promise<AdminProvider[]> {
  const res = await adminClient.get<unknown>('/_internal/admin/providers');
  return adminProvidersSchema.parse(res.data).providers;
}

export async function fetchProviderTypes(): Promise<ProviderTypeInfo[]> {
  const res = await adminClient.get<unknown>('/_internal/admin/provider-types');
  return providerTypesSchema.parse(res.data).provider_types;
}

export async function createAdminProvider(body: Record<string, unknown>): Promise<AdminProvider> {
  const res = await adminClient.post<unknown>('/_internal/admin/providers', body);
  return adminProviderSchema.parse(res.data);
}

export async function updateAdminProvider(name: string, body: Record<string, unknown>): Promise<AdminProvider> {
  const res = await adminClient.put<unknown>(`/_internal/admin/providers/${encodeURIComponent(name)}`, body);
  return adminProviderSchema.parse(res.data);
}

export async function deleteAdminProvider(name: string) {
  await adminClient.delete(`/_internal/admin/providers/${encodeURIComponent(name)}`);
}

export async function setProviderCredential(name: string, body: Record<string, unknown>) {
  await adminClient.put(`/_internal/admin/providers/${encodeURIComponent(name)}/credential`, body);
}

export const copilotDeviceFlowTimeout = 25_000;

export async function startCopilotDeviceFlow(
  body: Record<string, unknown>,
  options?: { signal?: AbortSignal },
): Promise<CopilotDeviceFlowStatus> {
  const res = await adminClient.post<unknown>('/_internal/admin/copilot-device-flows', body, {
    timeout: copilotDeviceFlowTimeout,
    signal: options?.signal,
  });
  return copilotDeviceFlowStatusSchema.parse(res.data);
}

export async function fetchCopilotDeviceFlow(
  id: string,
  options?: { signal?: AbortSignal },
): Promise<CopilotDeviceFlowStatus> {
  const res = await adminClient.get<unknown>(`/_internal/admin/copilot-device-flows/${encodeURIComponent(id)}`, {
    signal: options?.signal,
  });
  return copilotDeviceFlowStatusSchema.parse(res.data);
}

export async function pollCopilotDeviceFlow(
  id: string,
  options?: { signal?: AbortSignal },
): Promise<CopilotDeviceFlowStatus> {
  const res = await adminClient.post<unknown>(
    `/_internal/admin/copilot-device-flows/${encodeURIComponent(id)}/poll`,
    {},
    { timeout: copilotDeviceFlowTimeout, signal: options?.signal },
  );
  return copilotDeviceFlowStatusSchema.parse(res.data);
}

export async function cancelCopilotDeviceFlow(
  id: string,
  options?: { signal?: AbortSignal },
): Promise<CopilotDeviceFlowStatus> {
  const res = await adminClient.delete<unknown>(`/_internal/admin/copilot-device-flows/${encodeURIComponent(id)}`, {
    signal: options?.signal,
  });
  return copilotDeviceFlowStatusSchema.parse(res.data);
}

export async function fetchAdminAliases(): Promise<AdminAlias[]> {
  const res = await adminClient.get<unknown>('/_internal/admin/aliases');
  return adminAliasesSchema.parse(res.data).aliases;
}

export async function createAdminAlias(body: Record<string, unknown>): Promise<AdminAlias> {
  const res = await adminClient.post<unknown>('/_internal/admin/aliases', body);
  return adminAliasSchema.parse(res.data);
}

export async function updateAdminAlias(name: string, body: Record<string, unknown>): Promise<AdminAlias> {
  const res = await adminClient.put<unknown>(`/_internal/admin/aliases/${encodeURIComponent(name)}`, body);
  return adminAliasSchema.parse(res.data);
}

export async function deleteAdminAlias(name: string) {
  await adminClient.delete(`/_internal/admin/aliases/${encodeURIComponent(name)}`);
}

export async function fetchAdminKeys(): Promise<AdminKey[]> {
  const res = await adminClient.get<unknown>('/_internal/admin/keys');
  return adminKeysSchema.parse(res.data).keys;
}

export async function createAdminKey(body: Record<string, unknown>): Promise<{ key: AdminKey; token: string }> {
  const res = await adminClient.post<{ key: unknown; token: string }>('/_internal/admin/keys', body);
  const key = adminKeySchema.parse(res.data.key);
  return { key, token: res.data.token };
}

export async function rotateAdminKey(id: string): Promise<string> {
  const res = await adminClient.post<{ token: string }>(`/_internal/admin/keys/${encodeURIComponent(id)}/rotate`);
  return res.data.token;
}

export async function revokeAdminKey(id: string) {
  await adminClient.post(`/_internal/admin/keys/${encodeURIComponent(id)}/revoke`);
}

export async function deleteAdminKey(id: string) {
  await adminClient.delete(`/_internal/admin/keys/${encodeURIComponent(id)}`);
}

export async function fetchAdminKey(id: string): Promise<AdminKey> {
  const res = await adminClient.get<{ key: unknown }>(`/_internal/admin/keys/${encodeURIComponent(id)}`);
  return adminKeySchema.parse(res.data.key);
}

export async function updateAdminKey(id: string, body: Record<string, unknown>): Promise<AdminKey> {
  const res = await adminClient.put<{ key: unknown }>(`/_internal/admin/keys/${encodeURIComponent(id)}`, body);
  return adminKeySchema.parse(res.data.key);
}

export async function fetchAdminUsers(): Promise<AdminUser[]> {
  const res = await adminClient.get<unknown>('/_internal/admin/users');
  return adminUsersSchema.parse(res.data).users;
}

export async function createAdminUser(body: Record<string, unknown>): Promise<AdminUser> {
  const res = await adminClient.post<unknown>('/_internal/admin/users', body);
  return adminUserSchema.parse(res.data);
}

export async function setUserRole(id: string, role: string): Promise<AdminUser> {
  const res = await adminClient.post<unknown>(`/_internal/admin/users/${encodeURIComponent(id)}/role`, { role });
  return adminUserSchema.parse(res.data);
}

export async function resetUserPassword(id: string, password: string) {
  await adminClient.post(`/_internal/admin/users/${encodeURIComponent(id)}/reset-password`, { password });
}

export async function deleteAdminUser(id: string) {
  await adminClient.delete(`/_internal/admin/users/${encodeURIComponent(id)}`);
}

export async function setUserDisabled(id: string, disabled: boolean) {
  await adminClient.post(`/_internal/admin/users/${encodeURIComponent(id)}/${disabled ? 'disable' : 'enable'}`);
}

export async function fetchAdminInvites(): Promise<AdminInvite[]> {
  const res = await adminClient.get<unknown>('/_internal/admin/invites');
  return adminInvitesSchema.parse(res.data).invites;
}

export async function createAdminInvite(
  body: Record<string, unknown>,
): Promise<{ invite: AdminInvite; token: string }> {
  const res = await adminClient.post<{ invite: unknown; token: string }>('/_internal/admin/invites', body);
  return { invite: adminInviteSchema.parse(res.data.invite), token: res.data.token };
}

export async function deleteAdminInvite(id: string) {
  await adminClient.delete(`/_internal/admin/invites/${encodeURIComponent(id)}`);
}

export async function fetchAdminOIDCConfig(): Promise<AdminOIDCConfig> {
  const res = await adminClient.get<unknown>('/_internal/admin/oidc-config');
  return adminOIDCConfigSchema.parse(res.data);
}

export async function updateAdminOIDCConfig(body: Record<string, unknown>): Promise<AdminOIDCConfig> {
  const res = await adminClient.put<unknown>('/_internal/admin/oidc-config', body);
  return adminOIDCConfigSchema.parse(res.data);
}

export async function fetchAdminWorkspaces(): Promise<AdminWorkspace[]> {
  const res = await adminClient.get<unknown>('/_internal/admin/workspaces');
  return adminWorkspacesSchema.parse(res.data).workspaces;
}

export async function createAdminWorkspace(body: Record<string, unknown>): Promise<AdminWorkspace> {
  const res = await adminClient.post<unknown>('/_internal/admin/workspaces', body);
  return adminWorkspaceSchema.parse({
    ...((res.data as Record<string, unknown>) ?? {}),
    role: (res.data as Record<string, unknown>)?.role ?? 'admin',
  });
}

export async function fetchAdminWorkspace(id: string): Promise<Record<string, unknown>> {
  const res = await adminClient.get<unknown>(`/_internal/admin/workspaces/${encodeURIComponent(id)}`);
  return res.data as Record<string, unknown>;
}

export async function updateAdminWorkspace(id: string, body: Record<string, unknown>) {
  await adminClient.put(`/_internal/admin/workspaces/${encodeURIComponent(id)}`, body);
}

export async function deleteAdminWorkspace(id: string) {
  await adminClient.delete(`/_internal/admin/workspaces/${encodeURIComponent(id)}`);
}

export async function fetchWorkspaceTeams(workspaceId: string): Promise<AdminTeam[]> {
  const res = await adminClient.get<unknown>(`/_internal/admin/workspaces/${encodeURIComponent(workspaceId)}/teams`);
  return adminTeamsSchema.parse(res.data).teams;
}

export async function createWorkspaceTeam(workspaceId: string, body: Record<string, unknown>): Promise<AdminTeam> {
  const res = await adminClient.post<unknown>(
    `/_internal/admin/workspaces/${encodeURIComponent(workspaceId)}/teams`,
    body,
  );
  return adminTeamSchema.parse(res.data);
}

export async function updateWorkspaceTeam(
  workspaceId: string,
  teamId: string,
  body: Record<string, unknown>,
): Promise<AdminTeam> {
  const res = await adminClient.put<unknown>(
    `/_internal/admin/workspaces/${encodeURIComponent(workspaceId)}/teams/${encodeURIComponent(teamId)}`,
    body,
  );
  return adminTeamSchema.parse(res.data);
}

export async function deleteWorkspaceTeam(workspaceId: string, teamId: string) {
  await adminClient.delete(
    `/_internal/admin/workspaces/${encodeURIComponent(workspaceId)}/teams/${encodeURIComponent(teamId)}`,
  );
}

export async function fetchWorkspaceMembers(workspaceId: string): Promise<AdminWorkspaceMember[]> {
  const res = await adminClient.get<unknown>(`/_internal/admin/workspaces/${encodeURIComponent(workspaceId)}/members`);
  return z.object({ members: z.array(adminWorkspaceMemberSchema) }).parse(res.data).members;
}

export async function addWorkspaceMember(workspaceId: string, body: Record<string, unknown>) {
  await adminClient.post(`/_internal/admin/workspaces/${encodeURIComponent(workspaceId)}/members`, body);
}

export async function setWorkspaceMemberRole(workspaceId: string, userId: string, role: string) {
  await adminClient.put(
    `/_internal/admin/workspaces/${encodeURIComponent(workspaceId)}/members/${encodeURIComponent(userId)}`,
    {
      role,
    },
  );
}

export async function removeWorkspaceMember(workspaceId: string, userId: string) {
  await adminClient.delete(
    `/_internal/admin/workspaces/${encodeURIComponent(workspaceId)}/members/${encodeURIComponent(userId)}`,
  );
}

export async function fetchTeamMembers(workspaceId: string, teamId: string): Promise<AdminTeamMember[]> {
  const res = await adminClient.get<unknown>(
    `/_internal/admin/workspaces/${encodeURIComponent(workspaceId)}/teams/${encodeURIComponent(teamId)}/members`,
  );
  return z.object({ members: z.array(adminTeamMemberSchema) }).parse(res.data).members;
}

export async function addTeamMember(workspaceId: string, teamId: string, body: Record<string, unknown>) {
  await adminClient.post(
    `/_internal/admin/workspaces/${encodeURIComponent(workspaceId)}/teams/${encodeURIComponent(teamId)}/members`,
    body,
  );
}

export async function removeTeamMember(workspaceId: string, teamId: string, userId: string) {
  await adminClient.delete(
    `/_internal/admin/workspaces/${encodeURIComponent(workspaceId)}/teams/${encodeURIComponent(teamId)}/members/${encodeURIComponent(userId)}`,
  );
}

export async function setTeamMemberRole(workspaceId: string, teamId: string, userId: string, role: string) {
  await adminClient.put(
    `/_internal/admin/workspaces/${encodeURIComponent(workspaceId)}/teams/${encodeURIComponent(teamId)}/members/${encodeURIComponent(userId)}`,
    { role },
  );
}

export async function fetchScopeQuota(workspaceId: string, scope: 'users' | 'teams', id: string): Promise<AdminQuota> {
  const res = await adminClient.get<unknown>(
    `/_internal/admin/workspaces/${encodeURIComponent(workspaceId)}/${scope}/${encodeURIComponent(id)}/quota`,
  );
  return adminQuotaSchema.parse(res.data);
}

export async function updateScopeQuota(
  workspaceId: string,
  scope: 'users' | 'teams',
  id: string,
  body: Record<string, unknown>,
): Promise<AdminQuota> {
  const res = await adminClient.put<unknown>(
    `/_internal/admin/workspaces/${encodeURIComponent(workspaceId)}/${scope}/${encodeURIComponent(id)}/quota`,
    body,
  );
  return adminQuotaSchema.parse(res.data);
}

export async function publicRegister(body: Record<string, unknown>): Promise<unknown> {
  const res = await axios.post<unknown>('/_internal/admin/register', body, { timeout: 10_000 });
  return res.data;
}
