import { TempPasswordDialog } from './TempPasswordDialog'
import type { CreateUserResponse } from '@/types/api'

interface CreatedUserDialogProps {
  result: CreateUserResponse | null
  onClose: () => void
}

export function CreatedUserDialog({ result, onClose }: CreatedUserDialogProps) {
  return (
    <TempPasswordDialog
      open={result !== null}
      email={result?.user.email ?? ''}
      tempPassword={result?.temp_password ?? ''}
      onClose={onClose}
    />
  )
}
