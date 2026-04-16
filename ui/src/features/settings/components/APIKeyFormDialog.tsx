import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { createAPIKey } from '@/api/api-keys'
import { queryKeys } from '@/lib/query-keys'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { toast } from '@/components/ui/use-toast'
import type { APIKeyCreated, CreateAPIKeyRequest } from '@/types/api'

const EXPIRY_PRESETS = [
  { label: '30d', days: 30 },
  { label: '90d', days: 90 },
  { label: '180d', days: 180 },
  { label: '1y', days: 365 },
  { label: 'Never', days: null },
] as const

function addDays(days: number): string {
  const d = new Date()
  d.setDate(d.getDate() + days)
  return d.toISOString().slice(0, 10)
}

interface APIKeyFormDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onCreated: (apiKey: APIKeyCreated) => void
}

export function APIKeyFormDialog({ open, onOpenChange, onCreated }: APIKeyFormDialogProps) {
  const [name, setName] = useState('')
  const [expiryPreset, setExpiryPreset] = useState<number | null>(90)
  const [customDate, setCustomDate] = useState('')
  const queryClient = useQueryClient()

  const { mutate: doCreate, isPending } = useMutation({
    mutationFn: (data: CreateAPIKeyRequest) => createAPIKey(data),
    onSuccess: ({ apiKey }) => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.apiKeys })
      onCreated(apiKey)
      setName('')
      setExpiryPreset(90)
      setCustomDate('')
    },
    onError: () => {
      toast({ title: 'Failed to create API key', variant: 'destructive' })
    },
  })

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    const req: CreateAPIKeyRequest = { name: name.trim() }
    if (expiryPreset !== null) {
      req.expires_at = addDays(expiryPreset)
    } else if (customDate) {
      req.expires_at = customDate
    }
    doCreate(req)
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Create API Key</DialogTitle>
          <DialogDescription>
            Give your key a name and choose an expiration period.
          </DialogDescription>
        </DialogHeader>
        <form onSubmit={handleSubmit} className="space-y-4">
          <div className="space-y-1">
            <label htmlFor="api-key-name" className="text-sm font-medium">
              Name
            </label>
            <Input
              id="api-key-name"
              placeholder="e.g. CI Pipeline"
              value={name}
              onChange={(e) => setName(e.target.value)}
              required
            />
          </div>
          <div className="space-y-2">
            <p className="text-sm font-medium">Expiration</p>
            <div className="flex flex-wrap gap-2">
              {EXPIRY_PRESETS.map((p) => (
                <Button
                  key={p.label}
                  type="button"
                  size="sm"
                  variant={expiryPreset === p.days ? 'default' : 'outline'}
                  onClick={() => {
                    setExpiryPreset(p.days)
                    setCustomDate('')
                  }}
                >
                  {p.label}
                </Button>
              ))}
            </div>
            <div className="space-y-1">
              <label htmlFor="api-key-custom-date" className="text-muted-foreground text-xs">
                Or pick a custom date
              </label>
              <Input
                id="api-key-custom-date"
                type="date"
                value={customDate}
                min={new Date().toISOString().slice(0, 10)}
                onChange={(e) => {
                  setCustomDate(e.target.value)
                  setExpiryPreset(null)
                }}
                className="w-40"
              />
            </div>
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button type="submit" disabled={!name.trim() || isPending}>
              {isPending ? 'Creating…' : 'Create'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
