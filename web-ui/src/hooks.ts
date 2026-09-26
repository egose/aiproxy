import { useQuery } from '@tanstack/react-query';
import { useSnapshot as useTokenSnapshot } from 'valtio';
import { adminOrgStore, adminStore, dashboardStore } from './store';
import {
  fetchAdminAliases,
  fetchAdminInvites,
  fetchAdminKeys,
  fetchAdminMe,
  fetchAdminOIDCConfig,
  fetchAdminOrgs,
  fetchAdminProviders,
  fetchAdminStatus,
  fetchAdminUsers,
  fetchOrgMembers,
  fetchOrgTeams,
  fetchProviderTypes,
  fetchScopeQuota,
  fetchTeamMembers,
} from './services/admin';
import { fetchBlock, fetchBlocks, fetchPayload, fetchPayloads, fetchSnapshot } from './services/dashboard';

export function useDashboardToken() {
  return useTokenSnapshot(dashboardStore).token;
}

export function useDataAuth() {
  const token = useDashboardToken();
  const session = useAdminSession();
  const authed = token ? 'token' : session.accessToken ? 'session' : 'anon';
  return { authed, enabled: !!token || !!session.accessToken };
}

export function useSnapshot(enabled = true) {
  const { authed, enabled: hasAuth } = useDataAuth();
  return useQuery({
    queryKey: ['dashboard', 'snapshot', authed],
    queryFn: fetchSnapshot,
    enabled: enabled && hasAuth,
    refetchInterval: 10_000,
    retry: false,
  });
}

export function usePayloads(limit = 100, errorsOnly = false) {
  const { authed, enabled: hasAuth } = useDataAuth();
  return useQuery({
    queryKey: ['dashboard', 'payloads', limit, errorsOnly, authed],
    queryFn: () => fetchPayloads(limit, errorsOnly),
    enabled: hasAuth,
    retry: false,
  });
}

export function usePayload(requestId: string | null) {
  const { authed, enabled: hasAuth } = useDataAuth();
  return useQuery({
    queryKey: ['dashboard', 'payload', requestId, authed],
    queryFn: () => fetchPayload(requestId ?? ''),
    enabled: hasAuth && !!requestId,
    retry: false,
  });
}

export function useBlocks() {
  const { authed, enabled: hasAuth } = useDataAuth();
  return useQuery({
    queryKey: ['dashboard', 'blocks', authed],
    queryFn: fetchBlocks,
    enabled: hasAuth,
    refetchInterval: 10_000,
    retry: false,
  });
}

export function useBlock(blockId: string | null) {
  const { authed, enabled: hasAuth } = useDataAuth();
  return useQuery({
    queryKey: ['dashboard', 'block', blockId, authed],
    queryFn: () => fetchBlock(blockId ?? ''),
    enabled: hasAuth && !!blockId,
    retry: false,
  });
}

export function useAdminStatus() {
  return useQuery({
    queryKey: ['admin', 'status'],
    queryFn: fetchAdminStatus,
    retry: false,
  });
}

export function useAdminSession() {
  return useTokenSnapshot(adminStore);
}

export function useAdminMe(enabled = true) {
  const session = useAdminSession();
  return useQuery({
    queryKey: ['admin', 'me', session.accessToken ? 'authed' : 'anon'],
    queryFn: fetchAdminMe,
    enabled: enabled && !!session.accessToken,
    retry: false,
    staleTime: 60_000,
  });
}

export function useAdminOrg() {
  return useTokenSnapshot(adminOrgStore);
}

export function useCurrentOrg() {
  const orgs = useAdminOrgs(true);
  const { orgId } = useAdminOrg();
  const list = orgs.data ?? [];
  const org = list.some((o) => o.id === orgId) ? list.find((o) => o.id === orgId) : list[0];
  return { org, orgs };
}

export function useAdminProviders(enabled = true) {
  const session = useAdminSession();
  const orgId = useAdminOrg().orgId;
  return useQuery({
    queryKey: ['admin', 'providers', session.accessToken ? 'authed' : 'anon', orgId || 'all'],
    queryFn: fetchAdminProviders,
    enabled: enabled && !!session.accessToken,
    retry: false,
  });
}

export function useProviderTypes(enabled = true) {
  const session = useAdminSession();
  return useQuery({
    queryKey: ['admin', 'provider-types', session.accessToken ? 'authed' : 'anon'],
    queryFn: fetchProviderTypes,
    enabled: enabled && !!session.accessToken,
    retry: false,
    staleTime: 300_000,
  });
}

export function useAdminAliases(enabled = true) {
  const session = useAdminSession();
  const orgId = useAdminOrg().orgId;
  return useQuery({
    queryKey: ['admin', 'aliases', session.accessToken ? 'authed' : 'anon', orgId || 'all'],
    queryFn: fetchAdminAliases,
    enabled: enabled && !!session.accessToken,
    retry: false,
  });
}

export function useAdminKeys(enabled = true) {
  const session = useAdminSession();
  const orgId = useAdminOrg().orgId;
  return useQuery({
    queryKey: ['admin', 'keys', session.accessToken ? 'authed' : 'anon', orgId || 'all'],
    queryFn: fetchAdminKeys,
    enabled: enabled && !!session.accessToken,
    retry: false,
  });
}

export function useAdminUsers(enabled = true) {
  const session = useAdminSession();
  return useQuery({
    queryKey: ['admin', 'users', session.accessToken ? 'authed' : 'anon'],
    queryFn: fetchAdminUsers,
    enabled: enabled && !!session.accessToken,
    retry: false,
  });
}

export function useAdminInvites(enabled = true) {
  const session = useAdminSession();
  return useQuery({
    queryKey: ['admin', 'invites', session.accessToken ? 'authed' : 'anon'],
    queryFn: fetchAdminInvites,
    enabled: enabled && !!session.accessToken,
    retry: false,
  });
}

export function useAdminOIDCConfig(enabled = true) {
  const session = useAdminSession();
  return useQuery({
    queryKey: ['admin', 'oidc-config', session.accessToken ? 'authed' : 'anon'],
    queryFn: fetchAdminOIDCConfig,
    enabled: enabled && !!session.accessToken,
    retry: false,
  });
}

export function useAdminOrgs(enabled = true) {
  const session = useAdminSession();
  return useQuery({
    queryKey: ['admin', 'orgs', session.accessToken ? 'authed' : 'anon'],
    queryFn: fetchAdminOrgs,
    enabled: enabled && !!session.accessToken,
    retry: false,
  });
}

export function useOrgTeams(orgId: string | null, enabled = true) {
  const session = useAdminSession();
  return useQuery({
    queryKey: ['admin', 'orgs', orgId, 'teams', session.accessToken ? 'authed' : 'anon'],
    queryFn: () => fetchOrgTeams(orgId ?? ''),
    enabled: enabled && !!session.accessToken && !!orgId,
    retry: false,
  });
}

export function useOrgMembers(orgId: string | null, enabled = true) {
  const session = useAdminSession();
  return useQuery({
    queryKey: ['admin', 'orgs', orgId, 'members', session.accessToken ? 'authed' : 'anon'],
    queryFn: () => fetchOrgMembers(orgId ?? ''),
    enabled: enabled && !!session.accessToken && !!orgId,
    retry: false,
  });
}

export function useScopeQuota(orgId: string | null, scope: 'users' | 'teams', id: string | null, enabled = true) {
  const session = useAdminSession();
  return useQuery({
    queryKey: ['admin', 'orgs', orgId, scope, id, 'quota', session.accessToken ? 'authed' : 'anon'],
    queryFn: () => fetchScopeQuota(orgId ?? '', scope, id ?? ''),
    enabled: enabled && !!session.accessToken && !!orgId && !!id,
    retry: false,
  });
}

export function useTeamMembers(orgId: string | null, teamId: string | null, enabled = true) {
  const session = useAdminSession();
  return useQuery({
    queryKey: ['admin', 'orgs', orgId, 'teams', teamId, 'members', session.accessToken ? 'authed' : 'anon'],
    queryFn: () => fetchTeamMembers(orgId ?? '', teamId ?? ''),
    enabled: enabled && !!session.accessToken && !!orgId && !!teamId,
    retry: false,
  });
}
