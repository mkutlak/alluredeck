import { useQuery, useMutation, useQueryClient, keepPreviousData } from '@tanstack/react-query'
import { queryKeys } from '@/lib/query-keys'
import { fetchAdminJobs, cancelJob } from '@/api/admin'

export function useAdminJobs(page: number, perPage: number) {
  const queryClient = useQueryClient()

  const query = useQuery({
    queryKey: queryKeys.adminJobs(page, perPage),
    queryFn: () => fetchAdminJobs(page, perPage),
    refetchInterval: 5_000,
    placeholderData: keepPreviousData,
  })

  const { mutate: doCancel } = useMutation({
    mutationFn: cancelJob,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['admin-jobs'] })
    },
  })

  return { ...query, doCancel }
}
