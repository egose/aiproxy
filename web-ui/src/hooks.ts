import { useQuery } from '@tanstack/react-query';
import { useSnapshot as useTokenSnapshot } from 'valtio';
import { dashboardStore } from './store';
import { fetchBlock, fetchBlocks, fetchPayload, fetchPayloads, fetchSnapshot } from './services/dashboard';

export function useDashboardToken() {
  return useTokenSnapshot(dashboardStore).token;
}

export function useSnapshot(enabled = true) {
  const token = useDashboardToken();
  return useQuery({
    queryKey: ['dashboard', 'snapshot', token ? 'authed' : 'anon'],
    queryFn: fetchSnapshot,
    enabled: enabled && !!token,
    refetchInterval: 10_000,
    retry: false,
  });
}

export function usePayloads(limit = 100, errorsOnly = false) {
  const token = useDashboardToken();
  return useQuery({
    queryKey: ['dashboard', 'payloads', limit, errorsOnly, token ? 'authed' : 'anon'],
    queryFn: () => fetchPayloads(limit, errorsOnly),
    enabled: !!token,
    retry: false,
  });
}

export function usePayload(requestId: string | null) {
  const token = useDashboardToken();
  return useQuery({
    queryKey: ['dashboard', 'payload', requestId, token ? 'authed' : 'anon'],
    queryFn: () => fetchPayload(requestId ?? ''),
    enabled: !!token && !!requestId,
    retry: false,
  });
}

export function useBlocks() {
  const token = useDashboardToken();
  return useQuery({
    queryKey: ['dashboard', 'blocks', token ? 'authed' : 'anon'],
    queryFn: fetchBlocks,
    enabled: !!token,
    refetchInterval: 10_000,
    retry: false,
  });
}

export function useBlock(blockId: string | null) {
  const token = useDashboardToken();
  return useQuery({
    queryKey: ['dashboard', 'block', blockId, token ? 'authed' : 'anon'],
    queryFn: () => fetchBlock(blockId ?? ''),
    enabled: !!token && !!blockId,
    retry: false,
  });
}
