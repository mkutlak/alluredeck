import { useState } from 'react'
import { Loader2 } from 'lucide-react'
import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Button } from '@/components/ui/button'
import { extractErrorMessage } from '@/api/client'
import { toast } from '@/components/ui/use-toast'
import { useResetUserPasswordMutation } from '../hooks/useUsers'
import { TempPasswordDialog } from './TempPasswordDialog'
import type { User } from '@/types/api'

interface ResetPasswordDialogProps {
  user: User | null
  onClose: () => void
}

export function ResetPasswordDialog({ user, onClose }: ResetPasswordDialogProps) {
  const [tempPassword, setTempPassword] = useState<string | null>(null)
  const mutation = useResetUserPasswordMutation()

  const handleConfirm = () => {
    if (!user) return
    mutation.mutate(user.id, {
      onSuccess: (data) => {
        toast({ title: 'Password reset' })
        setTempPassword(data.temp_password)
      },
      onError: (err) => {
        toast({ title: extractErrorMessage(err), variant: 'destructive' })
        onClose()
      },
    })
  }

  const handleTempPasswordClose = () => {
    setTempPassword(null)
    onClose()
  }

  // Show TempPasswordDialog once we have the new password
  if (tempPassword !== null && user) {
    return (
      <TempPasswordDialog
        open
        email={user.email}
        tempPassword={tempPassword}
        onClose={handleTempPasswordClose}
      />
    )
  }

  return (
    <AlertDialog open={user !== null} onOpenChange={(open) => !open && onClose()}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Reset password?</AlertDialogTitle>
          <AlertDialogDescription>
            Reset password for <span className="font-medium">{user?.email}</span>? A new temporary
            password will be generated. They will not be able to use their current password after
            this.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel disabled={mutation.isPending}>Cancel</AlertDialogCancel>
          <Button
            disabled={mutation.isPending}
            onClick={handleConfirm}
          >
            {mutation.isPending && <Loader2 className="animate-spin" size={16} />}
            Reset password
          </Button>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
