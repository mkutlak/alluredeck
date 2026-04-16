import { useQuery } from '@tanstack/react-query'
import { fetchAPIKeys } from '@/api/api-keys'
import { queryKeys } from '@/lib/query-keys'

export const MAX_KEYS = 5

export function useAPIKeys() {
  const query = useQuery({
    queryKey: queryKeys.apiKeys,
    queryFn: fetchAPIKeys,
  })

  const atLimit = (query.data?.length ?? 0) >= MAX_KEYS

  return { ...query, atLimit }
}
