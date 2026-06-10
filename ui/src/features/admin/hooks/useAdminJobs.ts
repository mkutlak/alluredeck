import { useQuery, useMutation, useQueryClient, keepPreviousData } from '@tanstack/react-query'
import { queryKeys } from '@/lib/query-keys'
import { fetchAdminJobs, cancelJob, deleteJob, retryJob } from '@/api/admin'
import { toast } from '@/components/ui/use-toast'
import { extractErrorMessage } from '@/api/client'

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
    onError: (err) => {
      toast({
        title: 'Failed to cancel job',
        description: extractErrorMessage(err),
        variant: 'destructive',
      })
    },
  })

  const { mutate: doDelete } = useMutation({
    mutationFn: deleteJob,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['admin-jobs'] })
    },
    onError: (err) => {
      toast({
        title: 'Failed to delete job',
        description: extractErrorMessage(err),
        variant: 'destructive',
      })
    },
  })

  const { mutate: doRetry } = useMutation({
    mutationFn: retryJob,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['admin-jobs'] })
      toast({ title: 'Job queued for retry' })
    },
    onError: (err) => {
      toast({
        title: 'Failed to retry job',
        description: extractErrorMessage(err),
        variant: 'destructive',
      })
    },
  })

  return { ...query, doCancel, doDelete, doRetry }
}
