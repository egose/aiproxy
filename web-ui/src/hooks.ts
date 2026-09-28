import { useQuery as usePublicQuery } from '@tanstack/react-query';
import { useSensitiveQuery as useQuery } from './sensitive-query';
import { useSnapshot as useTokenSnapshot } from 'valtio';
import { adminWorkspaceStore, adminStore, dashboardStore } from './store';
import {
  fetchAdminAliases,
  fetchAdminInvites,
  fetchAdminKeys,
  fetchAdminMe,
  fetchAdminOIDCConfig,
  fetchAdminWorkspaces,
  fetchAdminProviders,
  fetchAdminStatus,
  fetchAdminUsers,
  fetchWorkspaceMembers,
  fetchWorkspaceTeams,
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
  const me = useAdminMe();
  const operator = !!session.accessToken && !me.error && me.data?.is_admin === true;
  const authed = token ? 'token' : operator ? 'session' : 'anon';
  return {
    authed,
    enabled: !!token || operator,
    pending: !token && !!session.accessToken && me.isPending,
    error: !token ? me.error : null,
  };
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
  return usePublicQuery({
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

export function useAdminWorkspace() {
  return useTokenSnapshot(adminWorkspaceStore);
}

export function useCurrentWorkspace() {
  const workspaces = useAdminWorkspaces(true);
  const { workspaceId } = useAdminWorkspace();
  const list = workspaces.data ?? [];
  const workspace = list.some((o) => o.id === workspaceId) ? list.find((o) => o.id === workspaceId) : list[0];
  return { workspace, workspaces };
}

export function useAdminProviders(enabled = true) {
  const session = useAdminSession();
  const workspaceId = useAdminWorkspace().workspaceId;
  return useQuery({
    queryKey: ['admin', 'providers', session.accessToken ? 'authed' : 'anon', workspaceId || 'all'],
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
  const workspaceId = useAdminWorkspace().workspaceId;
  return useQuery({
    queryKey: ['admin', 'aliases', session.accessToken ? 'authed' : 'anon', workspaceId || 'all'],
    queryFn: fetchAdminAliases,
    enabled: enabled && !!session.accessToken,
    retry: false,
  });
}

export function useAdminKeys(enabled = true) {
  const session = useAdminSession();
  const workspaceId = useAdminWorkspace().workspaceId;
  return useQuery({
    queryKey: ['admin', 'keys', session.accessToken ? 'authed' : 'anon', workspaceId || 'all'],
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

export function useAdminWorkspaces(enabled = true) {
  const session = useAdminSession();
  return useQuery({
    queryKey: ['admin', 'workspaces', session.accessToken ? 'authed' : 'anon'],
    queryFn: fetchAdminWorkspaces,
    enabled: enabled && !!session.accessToken,
    retry: false,
  });
}

export function useWorkspaceTeams(workspaceId: string | null, enabled = true) {
  const session = useAdminSession();
  return useQuery({
    queryKey: ['admin', 'workspaces', workspaceId, 'teams', session.accessToken ? 'authed' : 'anon'],
    queryFn: () => fetchWorkspaceTeams(workspaceId ?? ''),
    enabled: enabled && !!session.accessToken && !!workspaceId,
    retry: false,
  });
}

export function useWorkspaceMembers(workspaceId: string | null, enabled = true) {
  const session = useAdminSession();
  return useQuery({
    queryKey: ['admin', 'workspaces', workspaceId, 'members', session.accessToken ? 'authed' : 'anon'],
    queryFn: () => fetchWorkspaceMembers(workspaceId ?? ''),
    enabled: enabled && !!session.accessToken && !!workspaceId,
    retry: false,
  });
}

export function useScopeQuota(workspaceId: string | null, scope: 'users' | 'teams', id: string | null, enabled = true) {
  const session = useAdminSession();
  return useQuery({
    queryKey: ['admin', 'workspaces', workspaceId, scope, id, 'quota', session.accessToken ? 'authed' : 'anon'],
    queryFn: () => fetchScopeQuota(workspaceId ?? '', scope, id ?? ''),
    enabled: enabled && !!session.accessToken && !!workspaceId && !!id,
    retry: false,
  });
}

export function useTeamMembers(workspaceId: string | null, teamId: string | null, enabled = true) {
  const session = useAdminSession();
  return useQuery({
    queryKey: ['admin', 'workspaces', workspaceId, 'teams', teamId, 'members', session.accessToken ? 'authed' : 'anon'],
    queryFn: () => fetchTeamMembers(workspaceId ?? '', teamId ?? ''),
    enabled: enabled && !!session.accessToken && !!workspaceId && !!teamId,
    retry: false,
  });
}
