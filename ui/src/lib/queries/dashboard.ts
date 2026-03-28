import { queryOptions } from '@tanstack/react-query'
import { fetchDashboard } from '@/api/dashboard'

export function dashboardOptions() {
  return queryOptions({
    queryKey: ['dashboard'] as const,
    queryFn: () => fetchDashboard(),
    staleTime: 5_000,
    refetchOnWindowFocus: 'always' as const,
  })
}
