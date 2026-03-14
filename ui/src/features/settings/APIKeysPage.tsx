import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Trash2, Copy, Check } from 'lucide-react'
import { fetchAPIKeys, createAPIKey, deleteAPIKey } from '@/api/api-keys'
import { queryKeys } from '@/lib/query-keys'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
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
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip'
import { Skeleton } from '@/components/ui/skeleton'
import { toast } from '@/components/ui/use-toast'
import type { APIKey, APIKeyCreated, CreateAPIKeyRequest } from '@/types/api'

const MAX_KEYS = 5

const EXPIRY_PRESETS = [
  { label: '30d', days: 30 },
  { label: '90d', days: 90 },
  { label: '180d', days: 180 },
  { label: '1y', days: 365 },
  { label: 'Never', days: null },
] as const

function isExpired(expiresAt: string | null): boolean {
  if (!expiresAt) return false
  return new Date(expiresAt) < new Date()
}

function formatDate(value: string | null): string {
  if (!value) return '—'
  return new Date(value).toLocaleDateString()
}

function addDays(days: number): string {
  const d = new Date()
  d.setDate(d.getDate() + days)
  return d.toISOString().slice(0, 10)
}

// ---------------------------------------------------------------------------
// Role badge
// ---------------------------------------------------------------------------
function RoleBadge({ role }: { role: APIKey['role'] }) {
  if (role === 'admin') {
    return (
      <Badge className="border-transparent bg-blue-100 text-blue-700 hover:bg-blue-100/80 dark:bg-blue-900/30 dark:text-blue-400">
        admin
      </Badge>
    )
  }
  return <Badge variant="secondary">viewer</Badge>
}

// ---------------------------------------------------------------------------
// Create dialog
// ---------------------------------------------------------------------------
interface CreateDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onCreated: (apiKey: APIKeyCreated) => void
}

function CreateDialog({ open, onOpenChange, onCreated }: CreateDialogProps) {
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

// ---------------------------------------------------------------------------
// Created key dialog (shows full key once)
// ---------------------------------------------------------------------------
interface CreatedKeyDialogProps {
  apiKey: APIKeyCreated | null
  onClose: () => void
}

function CreatedKeyDialog({ apiKey, onClose }: CreatedKeyDialogProps) {
  const [copied, setCopied] = useState(false)

  const handleCopy = () => {
    if (!apiKey) return
    void navigator.clipboard.writeText(apiKey.key).then(() => {
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    })
  }

  return (
    <Dialog open={apiKey !== null} onOpenChange={(open) => !open && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>API Key Created</DialogTitle>
          <DialogDescription>
            Copy this key now. You won&apos;t be able to see it again.
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-3">
          <div className="bg-muted flex items-center gap-2 rounded-md p-3">
            <code className="flex-1 break-all font-mono text-sm">{apiKey?.key ?? ''}</code>
            <Button type="button" size="icon" variant="ghost" onClick={handleCopy} aria-label="Copy key">
              {copied ? <Check size={16} /> : <Copy size={16} />}
            </Button>
          </div>
        </div>
        <DialogFooter>
          <Button onClick={onClose}>Done</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

// ---------------------------------------------------------------------------
// Delete confirmation
// ---------------------------------------------------------------------------
interface DeleteDialogProps {
  apiKey: APIKey | null
  onClose: () => void
}

function DeleteDialog({ apiKey, onClose }: DeleteDialogProps) {
  const queryClient = useQueryClient()

  const { mutate: doDelete, isPending } = useMutation({
    mutationFn: (id: number) => deleteAPIKey(id),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.apiKeys })
      toast({ title: 'API key deleted' })
      onClose()
    },
    onError: () => {
      toast({ title: 'Failed to delete API key', variant: 'destructive' })
    },
  })

  return (
    <AlertDialog open={apiKey !== null} onOpenChange={(open) => !open && onClose()}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Delete API Key?</AlertDialogTitle>
          <AlertDialogDescription>
            Delete API key &quot;{apiKey?.name}&quot; ({apiKey?.prefix}…)? This cannot be undone.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>Cancel</AlertDialogCancel>
          <AlertDialogAction
            disabled={isPending}
            onClick={() => apiKey && doDelete(apiKey.id)}
          >
            Delete
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}

// ---------------------------------------------------------------------------
// Main page
// ---------------------------------------------------------------------------
export function APIKeysPage() {
  const [createOpen, setCreateOpen] = useState(false)
  const [createdKey, setCreatedKey] = useState<APIKeyCreated | null>(null)
  const [deletingKey, setDeletingKey] = useState<APIKey | null>(null)

  const { data: keys = [], isLoading } = useQuery({
    queryKey: queryKeys.apiKeys,
    queryFn: fetchAPIKeys,
  })

  const atLimit = keys.length >= MAX_KEYS

  return (
    <div className="space-y-6 p-6">
      <div className="flex items-start justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold">API Keys</h1>
          <p className="text-muted-foreground mt-1 text-sm">
            Create API keys for programmatic access from CI/CD pipelines.
          </p>
        </div>
        <TooltipProvider>
          <Tooltip>
            <TooltipTrigger asChild>
              <span>
                <Button
                  onClick={() => setCreateOpen(true)}
                  disabled={atLimit}
                  aria-label="Create API Key"
                >
                  Create API Key
                </Button>
              </span>
            </TooltipTrigger>
            {atLimit && (
              <TooltipContent>Maximum {MAX_KEYS} API keys allowed</TooltipContent>
            )}
          </Tooltip>
        </TooltipProvider>
      </div>

      {isLoading ? (
        <div className="space-y-2">
          <Skeleton className="h-10 w-full" />
          <Skeleton className="h-10 w-full" />
        </div>
      ) : keys.length === 0 ? (
        <p className="text-muted-foreground py-8 text-center text-sm">
          No API keys yet. Create one to get started.
        </p>
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Name</TableHead>
              <TableHead>Prefix</TableHead>
              <TableHead>Role</TableHead>
              <TableHead>Expires</TableHead>
              <TableHead>Last Used</TableHead>
              <TableHead>Created</TableHead>
              <TableHead className="w-10" />
            </TableRow>
          </TableHeader>
          <TableBody>
            {keys.map((key) => {
              const expired = isExpired(key.expires_at)
              return (
                <TableRow key={key.id} className={expired ? 'opacity-50' : undefined}>
                  <TableCell className="font-medium">
                    <span className="flex items-center gap-2">
                      {key.name}
                      {expired && (
                        <Badge variant="destructive" className="text-xs">
                          Expired
                        </Badge>
                      )}
                    </span>
                  </TableCell>
                  <TableCell>
                    <code className="font-mono text-sm">{key.prefix}</code>
                  </TableCell>
                  <TableCell>
                    <RoleBadge role={key.role} />
                  </TableCell>
                  <TableCell className="text-muted-foreground text-sm">
                    {formatDate(key.expires_at)}
                  </TableCell>
                  <TableCell className="text-muted-foreground text-sm">
                    {formatDate(key.last_used)}
                  </TableCell>
                  <TableCell className="text-muted-foreground text-sm">
                    {formatDate(key.created_at)}
                  </TableCell>
                  <TableCell>
                    <Button
                      size="icon"
                      variant="ghost"
                      onClick={() => setDeletingKey(key)}
                      aria-label={`Delete API key ${key.name}`}
                    >
                      <Trash2 size={16} />
                    </Button>
                  </TableCell>
                </TableRow>
              )
            })}
          </TableBody>
        </Table>
      )}

      <CreateDialog
        open={createOpen}
        onOpenChange={setCreateOpen}
        onCreated={(apiKey) => {
          setCreateOpen(false)
          setCreatedKey(apiKey)
        }}
      />

      <CreatedKeyDialog
        apiKey={createdKey}
        onClose={() => setCreatedKey(null)}
      />

      <DeleteDialog
        apiKey={deletingKey}
        onClose={() => setDeletingKey(null)}
      />
    </div>
  )
}
