import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { queryKeys } from '@/lib/query-keys'
import { fetchAdminJobs, cancelJob } from '@/api/admin'

export function useAdminJobs() {
  const queryClient = useQueryClient()

  const query = useQuery({
    queryKey: queryKeys.adminJobs,
    queryFn: fetchAdminJobs,
    refetchInterval: 5_000,
  })

  const { mutate: doCancel } = useMutation({
    mutationFn: cancelJob,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.adminJobs })
    },
  })

  return { ...query, doCancel }
}
