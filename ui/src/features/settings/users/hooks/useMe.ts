import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { fetchMe, updateMe } from '@/api/users'
import { queryKeys } from '@/lib/query-keys'
import { toast } from '@/components/ui/use-toast'

export function useMeQuery() {
  return useQuery({
    queryKey: queryKeys.me,
    queryFn: fetchMe,
  })
}

export function useUpdateMeMutation() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (body: { name: string }) => updateMe(body),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.me })
      toast({ title: 'Profile updated' })
    },
    onError: () => {
      toast({ title: 'Failed to update profile', variant: 'destructive' })
    },
  })
}
