import { useQuery, useQueryClient, type QueryClient, type UseQueryOptions } from '@tanstack/react-query';
import { useSnapshot } from 'valtio';
import { assertAuthGeneration, authLifecycle, onAuthChange } from './auth-lifecycle';

const clients = new WeakSet<QueryClient>();

function bindClient(client: QueryClient) {
  if (clients.has(client)) return;
  clients.add(client);
  const reference = new WeakRef(client);
  const unsubscribe = onAuthChange(() => {
    const current = reference.deref();
    if (!current) return unsubscribe();
    const filters = { predicate: (query: { meta?: Record<string, unknown> }) => query.meta?.sensitive === true };
    void current.cancelQueries(filters);
    current.removeQueries(filters);
    current.getMutationCache().clear();
  });
}

export function useSensitiveQuery<T>(options: UseQueryOptions<T> & { queryFn: () => Promise<T> }) {
  bindClient(useQueryClient());
  const { generation } = useSnapshot(authLifecycle);
  return useQuery({
    ...options,
    queryKey: [...options.queryKey, { generation }],
    meta: { ...options.meta, sensitive: true },
    queryFn: async ({ signal }) => {
      assertAuthGeneration(generation);
      const data = await options.queryFn();
      assertAuthGeneration(generation);
      signal.throwIfAborted();
      return data;
    },
  });
}
