import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { queryKeys } from '@/lib/query-keys'
import {
  fetchAdminResults,
  cleanAdminResults,
  cleanAdminResultsBulk,
} from '@/api/admin'
import { toast } from '@/components/ui/use-toast'
import { extractErrorMessage } from '@/api/client'

export function useAdminResults() {
  const queryClient = useQueryClient()

  const query = useQuery({
    queryKey: queryKeys.adminResults,
    queryFn: fetchAdminResults,
    refetchInterval: 30_000,
  })

  const { mutate: doClean } = useMutation({
    mutationFn: cleanAdminResults,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.adminResults })
    },
    onError: (err) => {
      toast({
        title: 'Failed to clean results',
        description: extractErrorMessage(err),
        variant: 'destructive',
      })
    },
  })

  const { mutate: doCleanBulk } = useMutation({
    mutationFn: cleanAdminResultsBulk,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.adminResults })
    },
    onError: (err) => {
      toast({
        title: 'Failed to clean results',
        description: extractErrorMessage(err),
        variant: 'destructive',
      })
    },
  })

  return { ...query, doClean, doCleanBulk }
}
