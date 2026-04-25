import { useState } from 'react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { extractErrorMessage } from '@/api/client'
import { useChangeMyPasswordMutation } from '../hooks/useUsers'
import { toast } from '@/components/ui/use-toast'

export function ChangePasswordCard() {
  const [currentPassword, setCurrentPassword] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [bannerError, setBannerError] = useState('')

  const mutation = useChangeMyPasswordMutation()

  // Inline validation messages
  const newPasswordError =
    newPassword.length > 0 && newPassword.length < 12
      ? 'Password must be at least 12 characters'
      : newPassword.length >= 12 && currentPassword.length > 0 && newPassword === currentPassword
        ? 'New password must be different from current'
        : ''

  const confirmPasswordError =
    confirmPassword.length > 0 && confirmPassword !== newPassword ? 'Passwords do not match' : ''

  const isValid =
    currentPassword.length > 0 &&
    newPassword.length >= 12 &&
    newPassword !== currentPassword &&
    confirmPassword === newPassword

  const handleSubmit = (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault()
    setBannerError('')
    mutation.mutate(
      { current_password: currentPassword, new_password: newPassword },
      {
        onSuccess: () => {
          toast({ title: 'Password changed' })
          setCurrentPassword('')
          setNewPassword('')
          setConfirmPassword('')
        },
        onError: (err) => {
          setBannerError(extractErrorMessage(err))
        },
      },
    )
  }

  return (
    <div className="max-w-md space-y-4">
      <div>
        <h2 className="text-lg font-semibold">Change Password</h2>
      </div>
      {bannerError && (
        <div
          role="alert"
          className="bg-destructive/10 text-destructive rounded-md px-4 py-3 text-sm"
        >
          {bannerError}
        </div>
      )}
      <form onSubmit={handleSubmit} className="space-y-4">
        <div className="space-y-1">
          <Label htmlFor="current-password">Current password</Label>
          <Input
            id="current-password"
            type="password"
            value={currentPassword}
            onChange={(e) => setCurrentPassword(e.target.value)}
            autoComplete="current-password"
          />
        </div>
        <div className="space-y-1">
          <Label htmlFor="new-password">New password</Label>
          <Input
            id="new-password"
            type="password"
            value={newPassword}
            onChange={(e) => setNewPassword(e.target.value)}
            autoComplete="new-password"
          />
          {newPasswordError && (
            <p className="text-destructive text-xs">{newPasswordError}</p>
          )}
        </div>
        <div className="space-y-1">
          <Label htmlFor="confirm-password">Confirm new password</Label>
          <Input
            id="confirm-password"
            type="password"
            value={confirmPassword}
            onChange={(e) => setConfirmPassword(e.target.value)}
            autoComplete="new-password"
          />
          {confirmPasswordError && (
            <p className="text-destructive text-xs">{confirmPasswordError}</p>
          )}
        </div>
        <Button type="submit" size="sm" disabled={!isValid || mutation.isPending}>
          {mutation.isPending ? 'Changing…' : 'Change password'}
        </Button>
      </form>
    </div>
  )
}
