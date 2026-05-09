import { useMemo, useRef, useState } from 'react'
import { Button } from '@/components/ui/button'
import { Combobox } from '@/components/ui/combobox'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import { Separator } from '@/components/ui/separator'
import { formatDate, useFormatDate } from '@/lib/utils'
import { useUIStore } from '@/store/ui'
import { useMeQuery, useUpdateMeMutation } from './hooks/useMe'
import { ChangePasswordCard } from './components/ChangePasswordCard'
import type { User } from '@/types/api'

interface ProfileFormProps {
  me: User
}

function ProfileForm({ me }: ProfileFormProps) {
  const updateMutation = useUpdateMeMutation()
  const [editing, setEditing] = useState(false)
  const [name, setName] = useState(me.name)
  const initialNameRef = useRef(me.name)

  const handleSave = (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault()
    if (!name.trim()) return
    updateMutation.mutate(
      { name: name.trim() },
      { onSuccess: () => setEditing(false) },
    )
  }

  const handleCancel = () => {
    setName(initialNameRef.current)
    setEditing(false)
  }

  return (
    <div className="max-w-md space-y-4">
      {/* Read-only fields */}
      <div className="space-y-1">
        <p className="text-sm font-medium">Email</p>
        <p className="text-muted-foreground text-sm">{me.email}</p>
      </div>

      <div className="space-y-1">
        <p className="text-sm font-medium">Provider</p>
        <Badge variant="secondary">{me.provider}</Badge>
      </div>

      <div className="space-y-1">
        <p className="text-sm font-medium">Role</p>
        <Badge variant="secondary">{me.role}</Badge>
      </div>

      <div className="space-y-1">
        <p className="text-sm font-medium">Last Login</p>
        <p className="text-muted-foreground text-sm">
          {me.last_login ? formatDate(me.last_login) : '—'}
        </p>
      </div>

      {/* Editable name */}
      <form onSubmit={handleSave} className="space-y-2">
        <div className="space-y-1">
          <label htmlFor="profile-name" className="text-sm font-medium">
            Name
          </label>
          {editing ? (
            <Input
              id="profile-name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              required
            />
          ) : (
            <p className="text-muted-foreground text-sm">{me.name}</p>
          )}
        </div>
        <div className="flex gap-2">
          {editing ? (
            <>
              <Button
                type="submit"
                size="sm"
                disabled={!name.trim() || updateMutation.isPending}
              >
                {updateMutation.isPending ? 'Saving…' : 'Save'}
              </Button>
              <Button type="button" size="sm" variant="outline" onClick={handleCancel}>
                Cancel
              </Button>
            </>
          ) : (
            <Button type="button" size="sm" variant="outline" onClick={() => setEditing(true)}>
              Edit name
            </Button>
          )}
        </div>
      </form>
    </div>
  )
}

function DisplayCard() {
  const timezone = useUIStore((s) => s.timezone)
  const timeFormat = useUIStore((s) => s.timeFormat)
  const setTimezone = useUIStore((s) => s.setTimezone)
  const setTimeFormat = useUIStore((s) => s.setTimeFormat)
  const format = useFormatDate()

  const detectedZone = Intl.DateTimeFormat().resolvedOptions().timeZone

  const timezoneOptions = useMemo(
    () => [
      { value: '__auto__', label: `Auto (browser: ${detectedZone})` },
      ...Intl.supportedValuesOf('timeZone').map((z) => ({ value: z, label: z })),
    ],
    [detectedZone],
  )

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-lg font-semibold">Display</h2>
      </div>

      {/* Timezone row */}
      <div className="space-y-1">
        <p className="text-sm font-medium">Timezone</p>
        <Combobox
          options={timezoneOptions}
          value={timezone ?? '__auto__'}
          onChange={(v) => setTimezone(v === '__auto__' ? null : v)}
          placeholder="Select timezone"
          searchPlaceholder="Search timezone…"
          emptyText="No timezone matches"
          className="w-72"
        />
        <p className="text-muted-foreground text-sm">
          Times throughout the app will be shown in this zone.
        </p>
      </div>

      {/* Time format row */}
      <div className="space-y-1">
        <p className="text-sm font-medium">Time format</p>
        <div className="flex gap-2">
          <Button
            type="button"
            variant={timeFormat === null ? 'default' : 'outline'}
            size="sm"
            onClick={() => setTimeFormat(null)}
          >
            Auto
          </Button>
          <Button
            type="button"
            variant={timeFormat === '24h' ? 'default' : 'outline'}
            size="sm"
            onClick={() => setTimeFormat('24h')}
          >
            24-hour
          </Button>
          <Button
            type="button"
            variant={timeFormat === '12h' ? 'default' : 'outline'}
            size="sm"
            onClick={() => setTimeFormat('12h')}
          >
            12-hour
          </Button>
        </div>
      </div>

      {/* Preview row */}
      <p className="text-muted-foreground text-sm">Preview: {format(new Date())}</p>
    </div>
  )
}

export function ProfilePage() {
  const { data: me, isLoading, isError } = useMeQuery()

  if (isLoading) {
    return (
      <div className="flex h-32 items-center justify-center">
        <div className="border-primary h-6 w-6 animate-spin rounded-full border-2 border-t-transparent" />
      </div>
    )
  }

  if (isError || !me) {
    return (
      <div className="p-6">
        <p className="text-destructive text-sm">Failed to load profile. Please try again.</p>
      </div>
    )
  }

  return (
    <div className="space-y-6 p-6">
      <div>
        <h1 className="text-2xl font-semibold">Profile</h1>
        <p className="text-muted-foreground text-sm">Your account information.</p>
      </div>
      <ProfileForm me={me} />
      {me.provider === 'local' && (
        <>
          <Separator />
          <ChangePasswordCard />
        </>
      )}
      <Separator />
      <DisplayCard />
    </div>
  )
}
