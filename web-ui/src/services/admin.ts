import axios from 'axios';
import { z } from 'zod';

import { adminOrgStore, adminStore, clearAdminSession } from '../store';
import {
  adminAliasSchema,
  adminAliasesSchema,
  adminInviteSchema,
  adminInvitesSchema,
  adminKeySchema,
  adminKeysSchema,
  adminMeSchema,
  adminOIDCConfigSchema,
  adminOrgMemberSchema,
  adminOrgSchema,
  adminOrgsSchema,
  adminProviderSchema,
  adminProvidersSchema,
  adminQuotaSchema,
  adminStatusSchema,
  adminTeamMemberSchema,
  adminTeamSchema,
  adminTeamsSchema,
  adminUserSchema,
  adminUsersSchema,
  providerTypesSchema,
  type AdminAlias,
  type AdminInvite,
  type AdminKey,
  type AdminMe,
  type AdminOIDCConfig,
  type AdminOrg,
  type AdminOrgMember,
  type AdminProvider,
  type AdminQuota,
  type AdminStatus,
  type AdminTeam,
  type AdminTeamMember,
  type AdminUser,
  type ProviderTypeInfo,
} from '../types';

export const adminStatusPath = '/_internal/admin/status';

export const adminOrgHeader = 'X-Org-ID';

export const adminClient = axios.create({
  timeout: 10_000,
  headers: { 'Content-Type': 'application/json' },
});

adminClient.interceptors.request.use((config) => {
  const token = adminStore.accessToken;
  if (token) {
    config.headers = config.headers ?? {};
    config.headers.Authorization = `Bearer ${token}`;
  }
  const orgId = adminOrgStore.orgId;
  if (orgId !== '' && typeof config.url === 'string' && config.url.startsWith('/_internal/admin/')) {
    config.headers = config.headers ?? {};
    if (config.headers[adminOrgHeader] == null) {
      config.headers[adminOrgHeader] = orgId;
    }
  }
  return config;
});

adminClient.interceptors.response.use(
  (res) => res,
  async (err) => {
    const original = err?.config as (typeof err.config & { _retried?: boolean }) | undefined;
    if (err?.response?.status === 401 && original && !original._retried && adminStore.refreshToken) {
      original._retried = true;
      try {
        const res = await axios.post<{ access_token: string; refresh_token: string }>(
          '/_internal/admin/refresh',
          { refresh_token: adminStore.refreshToken },
          { timeout: 10_000 },
        );
        const { setAdminSession } = await import('../store');
        setAdminSession(res.data.access_token, res.data.refresh_token);
        original.headers = original.headers ?? {};
        original.headers.Authorization = `Bearer ${res.data.access_token}`;
        return adminClient.request(original);
      } catch {
        clearAdminSession();
      }
    }
    return Promise.reject(err);
  },
);

export async function fetchAdminStatus(): Promise<AdminStatus> {
  const res = await axios.get<unknown>(adminStatusPath, { timeout: 10_000 });
  return adminStatusSchema.parse(res.data);
}

export async function adminLogin(email: string, password: string) {
  const res = await axios.post<{ access_token: string; refresh_token: string }>(
    '/_internal/admin/login',
    { email, password },
    { timeout: 10_000 },
  );
  return res.data;
}

export async function adminLogout() {
  try {
    await adminClient.post('/_internal/admin/logout', { refresh_token: adminStore.refreshToken || undefined });
  } finally {
    clearAdminSession();
  }
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

export async function fetchAdminOrgs(): Promise<AdminOrg[]> {
  const res = await adminClient.get<unknown>('/_internal/admin/orgs');
  return adminOrgsSchema.parse(res.data).organizations;
}

export async function createAdminOrg(body: Record<string, unknown>): Promise<AdminOrg> {
  const res = await adminClient.post<unknown>('/_internal/admin/orgs', body);
  return adminOrgSchema.parse({
    ...((res.data as Record<string, unknown>) ?? {}),
    role: (res.data as Record<string, unknown>)?.role ?? 'admin',
  });
}

export async function fetchAdminOrg(id: string): Promise<Record<string, unknown>> {
  const res = await adminClient.get<unknown>(`/_internal/admin/orgs/${encodeURIComponent(id)}`);
  return res.data as Record<string, unknown>;
}

export async function updateAdminOrg(id: string, body: Record<string, unknown>) {
  await adminClient.put(`/_internal/admin/orgs/${encodeURIComponent(id)}`, body);
}

export async function deleteAdminOrg(id: string) {
  await adminClient.delete(`/_internal/admin/orgs/${encodeURIComponent(id)}`);
}

export async function fetchOrgTeams(orgId: string): Promise<AdminTeam[]> {
  const res = await adminClient.get<unknown>(`/_internal/admin/orgs/${encodeURIComponent(orgId)}/teams`);
  return adminTeamsSchema.parse(res.data).teams;
}

export async function createOrgTeam(orgId: string, body: Record<string, unknown>): Promise<AdminTeam> {
  const res = await adminClient.post<unknown>(`/_internal/admin/orgs/${encodeURIComponent(orgId)}/teams`, body);
  return adminTeamSchema.parse(res.data);
}

export async function updateOrgTeam(orgId: string, teamId: string, body: Record<string, unknown>): Promise<AdminTeam> {
  const res = await adminClient.put<unknown>(
    `/_internal/admin/orgs/${encodeURIComponent(orgId)}/teams/${encodeURIComponent(teamId)}`,
    body,
  );
  return adminTeamSchema.parse(res.data);
}

export async function deleteOrgTeam(orgId: string, teamId: string) {
  await adminClient.delete(`/_internal/admin/orgs/${encodeURIComponent(orgId)}/teams/${encodeURIComponent(teamId)}`);
}

export async function fetchOrgMembers(orgId: string): Promise<AdminOrgMember[]> {
  const res = await adminClient.get<unknown>(`/_internal/admin/orgs/${encodeURIComponent(orgId)}/members`);
  return z.object({ members: z.array(adminOrgMemberSchema) }).parse(res.data).members;
}

export async function addOrgMember(orgId: string, body: Record<string, unknown>) {
  await adminClient.post(`/_internal/admin/orgs/${encodeURIComponent(orgId)}/members`, body);
}

export async function setOrgMemberRole(orgId: string, userId: string, role: string) {
  await adminClient.put(`/_internal/admin/orgs/${encodeURIComponent(orgId)}/members/${encodeURIComponent(userId)}`, {
    role,
  });
}

export async function removeOrgMember(orgId: string, userId: string) {
  await adminClient.delete(`/_internal/admin/orgs/${encodeURIComponent(orgId)}/members/${encodeURIComponent(userId)}`);
}

export async function fetchTeamMembers(orgId: string, teamId: string): Promise<AdminTeamMember[]> {
  const res = await adminClient.get<unknown>(
    `/_internal/admin/orgs/${encodeURIComponent(orgId)}/teams/${encodeURIComponent(teamId)}/members`,
  );
  return z.object({ members: z.array(adminTeamMemberSchema) }).parse(res.data).members;
}

export async function addTeamMember(orgId: string, teamId: string, body: Record<string, unknown>) {
  await adminClient.post(
    `/_internal/admin/orgs/${encodeURIComponent(orgId)}/teams/${encodeURIComponent(teamId)}/members`,
    body,
  );
}

export async function removeTeamMember(orgId: string, teamId: string, userId: string) {
  await adminClient.delete(
    `/_internal/admin/orgs/${encodeURIComponent(orgId)}/teams/${encodeURIComponent(teamId)}/members/${encodeURIComponent(userId)}`,
  );
}

export async function setTeamMemberRole(orgId: string, teamId: string, userId: string, role: string) {
  await adminClient.put(
    `/_internal/admin/orgs/${encodeURIComponent(orgId)}/teams/${encodeURIComponent(teamId)}/members/${encodeURIComponent(userId)}`,
    { role },
  );
}

export async function fetchScopeQuota(orgId: string, scope: 'users' | 'teams', id: string): Promise<AdminQuota> {
  const res = await adminClient.get<unknown>(
    `/_internal/admin/orgs/${encodeURIComponent(orgId)}/${scope}/${encodeURIComponent(id)}/quota`,
  );
  return adminQuotaSchema.parse(res.data);
}

export async function updateScopeQuota(
  orgId: string,
  scope: 'users' | 'teams',
  id: string,
  body: Record<string, unknown>,
): Promise<AdminQuota> {
  const res = await adminClient.put<unknown>(
    `/_internal/admin/orgs/${encodeURIComponent(orgId)}/${scope}/${encodeURIComponent(id)}/quota`,
    body,
  );
  return adminQuotaSchema.parse(res.data);
}

export async function publicRegister(body: Record<string, unknown>): Promise<unknown> {
  const res = await axios.post<unknown>('/_internal/admin/register', body, { timeout: 10_000 });
  return res.data;
}
