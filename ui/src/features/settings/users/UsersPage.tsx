import { useState, useCallback } from 'react'
import { Navigate } from 'react-router'
import { UserPlus } from 'lucide-react'
import { useAuthStore, selectIsAdmin } from '@/store/auth'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { useDebounce } from '@/hooks/useDebounce'
import type { CreateUserRequest, CreateUserResponse, User, UserRole } from '@/types/api'
import { UserList } from './components/UserList'
import { UserFormDialog } from './components/UserFormDialog'
import { UserRoleDialog } from './components/UserRoleDialog'
import { DeactivateUserDialog } from './components/DeactivateUserDialog'
import { CreatedUserDialog } from './components/CreatedUserDialog'
import { ResetPasswordDialog } from './components/ResetPasswordDialog'
import {
  useUsersQuery,
  useCreateUserMutation,
  useUpdateRoleMutation,
  useUpdateActiveMutation,
  useDeactivateUserMutation,
} from './hooks/useUsers'

const PAGE_SIZE = 20

export function UsersPage() {
  const isAdmin = useAuthStore(selectIsAdmin)

  if (!isAdmin) {
    return <Navigate to="/settings/profile" replace />
  }

  return <UsersPageContent />
}

function UsersPageContent() {
  const [search, setSearch] = useState('')
  const [roleFilter, setRoleFilter] = useState<UserRole | 'all'>('all')
  const [page, setPage] = useState(0)

  const [inviteOpen, setInviteOpen] = useState(false)
  const [roleTarget, setRoleTarget] = useState<User | null>(null)
  const [deactivateTarget, setDeactivateTarget] = useState<User | null>(null)
  const [createdResult, setCreatedResult] = useState<CreateUserResponse | null>(null)
  const [resetTarget, setResetTarget] = useState<User | null>(null)

  const debouncedSearch = useDebounce(search, 300)

  const { data, isLoading, isError } = useUsersQuery({
    limit: PAGE_SIZE,
    offset: page * PAGE_SIZE,
    search: debouncedSearch || undefined,
    role: roleFilter === 'all' ? undefined : roleFilter,
  })

  const createMutation = useCreateUserMutation()
  const roleMutation = useUpdateRoleMutation()
  const activeMutation = useUpdateActiveMutation()
  const deactivateMutation = useDeactivateUserMutation()

  const handleReactivate = useCallback(
    (user: User) => {
      activeMutation.mutate({ id: user.id, active: true })
    },
    [activeMutation],
  )

  // Read current user id from auth store to guard self-deactivation
  const currentUsername = useAuthStore((s) => s.username)

  const handleCreate = useCallback(
    (req: CreateUserRequest) => {
      createMutation.mutate(req, {
        onSuccess: (result) => {
          setInviteOpen(false)
          setCreatedResult(result)
        },
      })
    },
    [createMutation],
  )

  const handleRoleConfirm = useCallback(
    (id: number, role: UserRole) => {
      roleMutation.mutate(
        { id, role },
        { onSuccess: () => setRoleTarget(null) },
      )
    },
    [roleMutation],
  )

  const handleDeactivateConfirm = useCallback(
    (id: number) => {
      deactivateMutation.mutate(id, { onSuccess: () => setDeactivateTarget(null) })
    },
    [deactivateMutation],
  )

  const users = data?.users ?? []
  const total = data?.total ?? 0
  const totalPages = Math.ceil(total / PAGE_SIZE)

  // Find current user id from user list by matching email/username
  const currentUser = users.find((u) => u.email === currentUsername || u.name === currentUsername)
  const currentUserId = currentUser?.id ?? null

  return (
    <div className="space-y-6 p-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-semibold">Users</h1>
          <p className="text-muted-foreground text-sm">Manage user accounts and roles.</p>
        </div>
        <Button onClick={() => setInviteOpen(true)}>
          <UserPlus size={16} className="mr-2" />
          Invite user
        </Button>
      </div>

      {/* Filters */}
      <div className="flex flex-wrap gap-3">
        <Input
          placeholder="Search by email or name…"
          value={search}
          onChange={(e) => {
            setSearch(e.target.value)
            setPage(0)
          }}
          className="w-64"
          aria-label="Search users"
        />
        <Select
          value={roleFilter}
          onValueChange={(v) => {
            setRoleFilter(v as UserRole | 'all')
            setPage(0)
          }}
        >
          <SelectTrigger className="w-36" aria-label="Filter by role">
            <SelectValue placeholder="All roles" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">All roles</SelectItem>
            <SelectItem value="admin">Admin</SelectItem>
            <SelectItem value="editor">Editor</SelectItem>
            <SelectItem value="viewer">Viewer</SelectItem>
          </SelectContent>
        </Select>
      </div>

      {/* Content */}
      {isLoading && (
        <div className="flex h-32 items-center justify-center">
          <div className="border-primary h-6 w-6 animate-spin rounded-full border-2 border-t-transparent" />
        </div>
      )}
      {isError && (
        <p className="text-destructive text-sm">Failed to load users. Please try again.</p>
      )}
      {!isLoading && !isError && users.length === 0 && (
        <p className="text-muted-foreground py-12 text-center text-sm">No users found.</p>
      )}
      {!isLoading && !isError && users.length > 0 && (
        <UserList
          users={users}
          currentUserId={currentUserId}
          onChangeRole={setRoleTarget}
          onDeactivate={setDeactivateTarget}
          onReactivate={handleReactivate}
          onResetPassword={setResetTarget}
        />
      )}

      {/* Pagination */}
      {totalPages > 1 && (
        <div className="flex items-center justify-between text-sm">
          <span className="text-muted-foreground">
            Showing {page * PAGE_SIZE + 1}–{Math.min((page + 1) * PAGE_SIZE, total)} of {total}
          </span>
          <div className="flex gap-2">
            <Button
              variant="outline"
              size="sm"
              disabled={page === 0}
              onClick={() => setPage((p) => p - 1)}
              aria-label="Previous page"
            >
              Previous
            </Button>
            <Button
              variant="outline"
              size="sm"
              disabled={page >= totalPages - 1}
              onClick={() => setPage((p) => p + 1)}
              aria-label="Next page"
            >
              Next
            </Button>
          </div>
        </div>
      )}

      {/* Dialogs */}
      <UserFormDialog
        open={inviteOpen}
        onOpenChange={setInviteOpen}
        onSubmit={handleCreate}
        isPending={createMutation.isPending}
      />
      <UserRoleDialog
        user={roleTarget}
        onClose={() => setRoleTarget(null)}
        onConfirm={handleRoleConfirm}
        isPending={roleMutation.isPending}
      />
      <DeactivateUserDialog
        user={deactivateTarget}
        onClose={() => setDeactivateTarget(null)}
        onConfirm={handleDeactivateConfirm}
        isPending={deactivateMutation.isPending}
      />
      <CreatedUserDialog result={createdResult} onClose={() => setCreatedResult(null)} />
      <ResetPasswordDialog user={resetTarget} onClose={() => setResetTarget(null)} />
    </div>
  )
}
