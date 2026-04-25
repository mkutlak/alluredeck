import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  createUser,
  fetchUsers,
  updateUserActive,
  updateUserRole,
  deactivateUser,
  changeMyPassword,
  resetUserPassword,
  type FetchUsersParams,
} from '@/api/users'
import { queryKeys } from '@/lib/query-keys'
import { toast } from '@/components/ui/use-toast'
import type { ChangePasswordRequest, UserRole } from '@/types/api'

export function useUsersQuery(params: FetchUsersParams = {}) {
  return useQuery({
    queryKey: [...queryKeys.users, params],
    queryFn: () => fetchUsers(params),
  })
}

export function useCreateUserMutation() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: createUser,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.users })
    },
    onError: () => {
      toast({ title: 'Failed to create user', variant: 'destructive' })
    },
  })
}

export function useUpdateRoleMutation() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, role }: { id: number; role: UserRole }) => updateUserRole(id, role),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.users })
      toast({ title: 'Role updated' })
    },
    onError: () => {
      toast({ title: 'Failed to update role', variant: 'destructive' })
    },
  })
}

export function useUpdateActiveMutation() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, active }: { id: number; active: boolean }) => updateUserActive(id, active),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.users })
      toast({ title: 'User status updated' })
    },
    onError: () => {
      toast({ title: 'Failed to update user status', variant: 'destructive' })
    },
  })
}

export function useDeactivateUserMutation() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => deactivateUser(id),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.users })
      toast({ title: 'User deactivated' })
    },
    onError: () => {
      toast({ title: 'Failed to deactivate user', variant: 'destructive' })
    },
  })
}

export function useChangeMyPasswordMutation() {
  return useMutation({
    mutationFn: (body: ChangePasswordRequest) => changeMyPassword(body),
  })
}

export function useResetUserPasswordMutation() {
  return useMutation({
    mutationFn: (id: number) => resetUserPassword(id),
  })
}
