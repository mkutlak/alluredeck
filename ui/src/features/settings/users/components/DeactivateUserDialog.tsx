import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import type { User } from '@/types/api'

interface DeactivateUserDialogProps {
  user: User | null
  onClose: () => void
  onConfirm: (id: number) => void
  isPending: boolean
}

export function DeactivateUserDialog({
  user,
  onClose,
  onConfirm,
  isPending,
}: DeactivateUserDialogProps) {
  return (
    <AlertDialog open={user !== null} onOpenChange={(open) => !open && onClose()}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Deactivate User?</AlertDialogTitle>
          <AlertDialogDescription>
            Deactivate <span className="font-medium">{user?.email}</span>? They will no longer be
            able to log in. This can be reversed by an admin.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>Cancel</AlertDialogCancel>
          <AlertDialogAction
            disabled={isPending}
            onClick={() => user && onConfirm(user.id)}
            className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
          >
            Deactivate
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
