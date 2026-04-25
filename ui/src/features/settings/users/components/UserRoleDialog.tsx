import { useState } from 'react'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import type { User, UserRole } from '@/types/api'

interface UserRoleDialogProps {
  user: User | null
  onClose: () => void
  onConfirm: (id: number, role: UserRole) => void
  isPending: boolean
}

export function UserRoleDialog({ user, onClose, onConfirm, isPending }: UserRoleDialogProps) {
  const [role, setRole] = useState<UserRole>(user?.role ?? 'viewer')

  // Sync role state when dialog opens for a different user
  const currentRole = user?.role ?? 'viewer'
  const effectiveRole = user ? role : currentRole

  const handleOpenChange = (open: boolean) => {
    if (!open) onClose()
  }

  const handleConfirm = () => {
    if (!user) return
    onConfirm(user.id, effectiveRole)
  }

  return (
    <Dialog open={user !== null} onOpenChange={handleOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Change Role</DialogTitle>
          <DialogDescription>
            Update the role for <span className="font-medium">{user?.email}</span>.
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-1">
          <label htmlFor="role-select" className="text-sm font-medium">
            New role
          </label>
          <Select
            value={effectiveRole}
            onValueChange={(v) => setRole(v as UserRole)}
          >
            <SelectTrigger id="role-select">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="viewer">Viewer</SelectItem>
              <SelectItem value="editor">Editor</SelectItem>
              <SelectItem value="admin">Admin</SelectItem>
            </SelectContent>
          </Select>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            Cancel
          </Button>
          <Button onClick={handleConfirm} disabled={isPending}>
            {isPending ? 'Saving…' : 'Save'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
