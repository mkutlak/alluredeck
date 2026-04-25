import { MoreHorizontal } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { formatDate } from '@/lib/utils'
import type { User } from '@/types/api'

interface UserListProps {
  users: User[]
  currentUserId: number | null
  onChangeRole: (user: User) => void
  onDeactivate: (user: User) => void
  onReactivate: (user: User) => void
  onResetPassword: (user: User) => void
}

function RoleBadge({ role }: { role: User['role'] }) {
  if (role === 'admin') {
    return (
      <Badge className="border-transparent bg-blue-100 text-blue-700 hover:bg-blue-100/80 dark:bg-blue-900/30 dark:text-blue-400">
        admin
      </Badge>
    )
  }
  if (role === 'editor') {
    return (
      <Badge className="border-transparent bg-green-100 text-green-700 hover:bg-green-100/80 dark:bg-green-900/30 dark:text-green-400">
        editor
      </Badge>
    )
  }
  return <Badge variant="secondary">viewer</Badge>
}

function ActiveBadge({ active }: { active: boolean }) {
  if (active) {
    return (
      <Badge className="border-transparent bg-emerald-100 text-emerald-700 hover:bg-emerald-100/80 dark:bg-emerald-900/30 dark:text-emerald-400">
        active
      </Badge>
    )
  }
  return <Badge variant="destructive">inactive</Badge>
}

export function UserList({
  users,
  currentUserId,
  onChangeRole,
  onDeactivate,
  onReactivate,
  onResetPassword,
}: UserListProps) {
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Email</TableHead>
          <TableHead>Name</TableHead>
          <TableHead>Role</TableHead>
          <TableHead>Status</TableHead>
          <TableHead>Last Login</TableHead>
          <TableHead className="w-10" />
        </TableRow>
      </TableHeader>
      <TableBody>
        {users.map((user) => (
          <TableRow key={user.id} className={!user.is_active ? 'opacity-50' : undefined}>
            <TableCell className="font-medium">{user.email}</TableCell>
            <TableCell>{user.name}</TableCell>
            <TableCell>
              <RoleBadge role={user.role} />
            </TableCell>
            <TableCell>
              <ActiveBadge active={user.is_active} />
            </TableCell>
            <TableCell className="text-muted-foreground text-sm">
              {user.last_login ? formatDate(user.last_login) : '—'}
            </TableCell>
            <TableCell>
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button
                    size="icon"
                    variant="ghost"
                    aria-label={`Actions for ${user.email}`}
                  >
                    <MoreHorizontal size={16} />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end">
                  <DropdownMenuItem onClick={() => onChangeRole(user)}>
                    Change role
                  </DropdownMenuItem>
                  {user.provider === 'local' && (
                    <DropdownMenuItem onClick={() => onResetPassword(user)}>
                      Reset password
                    </DropdownMenuItem>
                  )}
                  {user.is_active ? (
                    <DropdownMenuItem
                      onClick={() => onDeactivate(user)}
                      disabled={user.id === currentUserId}
                      className="text-destructive focus:text-destructive"
                    >
                      Deactivate
                    </DropdownMenuItem>
                  ) : (
                    <DropdownMenuItem onClick={() => onReactivate(user)}>
                      Reactivate
                    </DropdownMenuItem>
                  )}
                </DropdownMenuContent>
              </DropdownMenu>
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  )
}
