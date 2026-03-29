import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useSearchParams } from 'react-router'
import { Plus, Trash2, Pencil, Send, History, Bell, Loader2 } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
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
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Badge } from '@/components/ui/badge'
import { Skeleton } from '@/components/ui/skeleton'
import { toast } from '@/components/ui/use-toast'
import { queryKeys } from '@/lib/query-keys'
import {
  fetchWebhooks,
  createWebhook,
  updateWebhook,
  deleteWebhook,
  testWebhook,
  fetchWebhookDeliveries,
} from '@/api/webhooks'
import type {
  Webhook,
  WebhookDelivery,
  CreateWebhookRequest,
  UpdateWebhookRequest,
} from '@/types/api'

type WebhookTargetType = Webhook['target_type']

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function targetTypeClass(type: string): string {
  const variants: Record<string, string> = {
    slack: 'bg-purple-100 text-purple-800',
    discord: 'bg-indigo-100 text-indigo-800',
    teams: 'bg-blue-100 text-blue-800',
    generic: 'bg-gray-100 text-gray-800',
  }
  return variants[type] ?? variants['generic']
}

function maskUrl(url: string): string {
  try {
    const u = new URL(url)
    return `${u.protocol}//${u.host}/…`
  } catch {
    return url.slice(0, 30) + (url.length > 30 ? '…' : '')
  }
}

function formatDate(value: string): string {
  return new Date(value).toLocaleDateString()
}

function StatusBadge({
  code,
  error,
}: {
  code: number | null
  error: string | null
}): React.ReactElement {
  if (error) return <Badge variant="destructive">Error</Badge>
  if (code !== null && code >= 200 && code < 300)
    return <Badge className="bg-green-100 text-green-800">{code}</Badge>
  if (code !== null) return <Badge variant="destructive">{code}</Badge>
  return <Badge variant="secondary">Pending</Badge>
}

// ---------------------------------------------------------------------------
// Webhook form fields (shared between create and edit dialogs)
// ---------------------------------------------------------------------------

interface WebhookFormState {
  name: string
  target_type: WebhookTargetType
  url: string
  secret: string
  custom_template: string
  showTemplate: boolean
  is_active: boolean
}

function defaultFormState(): WebhookFormState {
  return {
    name: '',
    target_type: 'generic',
    url: '',
    secret: '',
    custom_template: '',
    showTemplate: false,
    is_active: true,
  }
}

function webhookToFormState(w: Webhook): WebhookFormState {
  return {
    name: w.name,
    target_type: w.target_type,
    url: w.url,
    secret: '',
    custom_template: w.template ?? '',
    showTemplate: Boolean(w.template),
    is_active: w.is_active,
  }
}

interface WebhookFormProps {
  state: WebhookFormState
  onChange: (next: Partial<WebhookFormState>) => void
}

function WebhookForm({ state, onChange }: WebhookFormProps) {
  return (
    <div className="space-y-4">
      <div className="space-y-1">
        <Label htmlFor="wh-name">Name</Label>
        <Input
          id="wh-name"
          placeholder="e.g. CI Alerts"
          value={state.name}
          onChange={(e) => onChange({ name: e.target.value })}
          required
        />
      </div>

      <div className="space-y-1">
        <Label htmlFor="wh-type">Target Type</Label>
        <Select
          value={state.target_type}
          onValueChange={(v) => onChange({ target_type: v as WebhookTargetType })}
        >
          <SelectTrigger id="wh-type">
            <SelectValue placeholder="Select type" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="slack">Slack</SelectItem>
            <SelectItem value="discord">Discord</SelectItem>
            <SelectItem value="teams">Teams</SelectItem>
            <SelectItem value="generic">Generic</SelectItem>
          </SelectContent>
        </Select>
      </div>

      <div className="space-y-1">
        <Label htmlFor="wh-url">URL</Label>
        <Input
          id="wh-url"
          type="url"
          placeholder="https://hooks.example.com/…"
          value={state.url}
          onChange={(e) => onChange({ url: e.target.value })}
          required
        />
      </div>

      {state.target_type === 'generic' && (
        <div className="space-y-1">
          <Label htmlFor="wh-secret">Secret (optional)</Label>
          <Input
            id="wh-secret"
            type="password"
            placeholder="Signing secret"
            value={state.secret}
            onChange={(e) => onChange({ secret: e.target.value })}
          />
        </div>
      )}

      <div className="space-y-1">
        <button
          type="button"
          className="text-muted-foreground flex items-center gap-1 text-sm hover:underline"
          onClick={() => onChange({ showTemplate: !state.showTemplate })}
        >
          {state.showTemplate ? '▾' : '▸'} Custom Template (optional)
        </button>
        {state.showTemplate && (
          <textarea
            id="wh-template"
            className="border-input bg-background placeholder:text-muted-foreground focus-visible:ring-ring flex min-h-[100px] w-full rounded-md border px-3 py-2 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-1"
            placeholder='{"text": "Build {{.ProjectID}} finished"}'
            value={state.custom_template}
            onChange={(e) => onChange({ custom_template: e.target.value })}
          />
        )}
      </div>

      <div className="flex items-center gap-2">
        <input
          id="wh-active"
          type="checkbox"
          checked={state.is_active}
          onChange={(e) => onChange({ is_active: e.target.checked })}
          className="h-4 w-4 rounded border-gray-300"
        />
        <Label htmlFor="wh-active">Active</Label>
      </div>
    </div>
  )
}

// ---------------------------------------------------------------------------
// Create dialog
// ---------------------------------------------------------------------------

interface CreateWebhookDialogProps {
  projectId: string
  open: boolean
  onOpenChange: (open: boolean) => void
}

function CreateWebhookDialog({ projectId, open, onOpenChange }: CreateWebhookDialogProps) {
  const [form, setForm] = useState<WebhookFormState>(defaultFormState)
  const queryClient = useQueryClient()

  const { mutate: doCreate, isPending } = useMutation({
    mutationFn: (req: CreateWebhookRequest) => createWebhook(projectId, req),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.webhooks(projectId) })
      toast({ title: 'Webhook created' })
      setForm(defaultFormState())
      onOpenChange(false)
    },
    onError: () => {
      toast({ title: 'Failed to create webhook', variant: 'destructive' })
    },
  })

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    const req: CreateWebhookRequest = {
      name: form.name.trim(),
      target_type: form.target_type,
      url: form.url.trim(),
      is_active: form.is_active,
    }
    if (form.secret.trim()) req.secret = form.secret.trim()
    if (form.custom_template.trim()) req.template = form.custom_template.trim()
    doCreate(req)
  }

  const handleOpenChange = (next: boolean) => {
    if (!next) setForm(defaultFormState())
    onOpenChange(next)
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Add Webhook</DialogTitle>
          <DialogDescription>Configure a new webhook for this project.</DialogDescription>
        </DialogHeader>
        <form onSubmit={handleSubmit} className="space-y-4">
          <WebhookForm
            state={form}
            onChange={(next) => setForm((prev) => ({ ...prev, ...next }))}
          />
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => handleOpenChange(false)}>
              Cancel
            </Button>
            <Button type="submit" disabled={!form.name.trim() || !form.url.trim() || isPending}>
              {isPending ? 'Creating…' : 'Create'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

// ---------------------------------------------------------------------------
// Edit dialog
// ---------------------------------------------------------------------------

interface EditWebhookDialogProps {
  projectId: string
  webhook: Webhook | null
  onClose: () => void
}

function EditWebhookDialog({ projectId, webhook, onClose }: EditWebhookDialogProps) {
  const [form, setForm] = useState<WebhookFormState>(() =>
    webhook ? webhookToFormState(webhook) : defaultFormState(),
  )
  const queryClient = useQueryClient()


  const { mutate: doUpdate, isPending } = useMutation({
    mutationFn: (req: UpdateWebhookRequest) =>
      updateWebhook(projectId, String(webhook!.id), req),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.webhooks(projectId) })
      toast({ title: 'Webhook updated' })
      onClose()
    },
    onError: () => {
      toast({ title: 'Failed to update webhook', variant: 'destructive' })
    },
  })

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    const req: UpdateWebhookRequest = {
      name: form.name.trim(),
      target_type: form.target_type,
      url: form.url.trim(),
      is_active: form.is_active,
    }
    if (form.secret.trim()) req.secret = form.secret.trim()
    req.template = form.custom_template.trim() || undefined
    doUpdate(req)
  }

  return (
    <Dialog open={webhook !== null} onOpenChange={(open) => !open && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Edit Webhook</DialogTitle>
          <DialogDescription>Update webhook &quot;{webhook?.name}&quot;.</DialogDescription>
        </DialogHeader>
        <form onSubmit={handleSubmit} className="space-y-4">
          <WebhookForm
            state={form}
            onChange={(next) => setForm((prev) => ({ ...prev, ...next }))}
          />
          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose}>
              Cancel
            </Button>
            <Button type="submit" disabled={!form.name.trim() || !form.url.trim() || isPending}>
              {isPending ? 'Saving…' : 'Save'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

// ---------------------------------------------------------------------------
// Delete dialog
// ---------------------------------------------------------------------------

interface DeleteWebhookDialogProps {
  projectId: string
  webhook: Webhook | null
  onClose: () => void
}

function DeleteWebhookDialog({ projectId, webhook, onClose }: DeleteWebhookDialogProps) {
  const queryClient = useQueryClient()

  const { mutate: doDelete, isPending } = useMutation({
    mutationFn: (id: string) => deleteWebhook(projectId, id),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.webhooks(projectId) })
      toast({ title: 'Webhook deleted' })
      onClose()
    },
    onError: () => {
      toast({ title: 'Failed to delete webhook', variant: 'destructive' })
    },
  })

  return (
    <AlertDialog open={webhook !== null} onOpenChange={(open) => !open && onClose()}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Delete Webhook?</AlertDialogTitle>
          <AlertDialogDescription>
            Delete webhook &quot;{webhook?.name}&quot;? This cannot be undone.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>Cancel</AlertDialogCancel>
          <AlertDialogAction
            disabled={isPending}
            onClick={() => webhook && doDelete(String(webhook.id))}
          >
            Delete
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}

// ---------------------------------------------------------------------------
// Test webhook button
// ---------------------------------------------------------------------------

interface TestWebhookButtonProps {
  projectId: string
  webhookId: string
}

function TestWebhookButton({ projectId, webhookId }: TestWebhookButtonProps) {
  const { mutate: doTest, isPending } = useMutation({
    mutationFn: () => testWebhook(projectId, webhookId),
    onSuccess: () => {
      toast({ title: 'Test delivery sent' })
    },
    onError: () => {
      toast({ title: 'Test delivery failed', variant: 'destructive' })
    },
  })

  return (
    <Button
      size="icon"
      variant="ghost"
      disabled={isPending}
      onClick={() => doTest()}
      aria-label="Send test delivery"
    >
      {isPending ? <Loader2 size={16} className="animate-spin" /> : <Send size={16} />}
    </Button>
  )
}

// ---------------------------------------------------------------------------
// Delivery history dialog
// ---------------------------------------------------------------------------

interface DeliveryHistoryDialogProps {
  projectId: string
  webhook: Webhook | null
  onClose: () => void
}

const DELIVERIES_PER_PAGE = 10

function DeliveryHistoryDialog({ projectId, webhook, onClose }: DeliveryHistoryDialogProps) {
  const [page, setPage] = useState(1)

  const { data, isLoading } = useQuery({
    queryKey: queryKeys.webhookDeliveries(
      projectId,
      String(webhook?.id ?? ''),
      page,
    ),
    queryFn: () =>
      fetchWebhookDeliveries(projectId, String(webhook!.id), page, DELIVERIES_PER_PAGE),
    enabled: webhook !== null,
  })

  const deliveries: WebhookDelivery[] = data?.data ?? []
  const total = data?.total ?? 0
  const totalPages = Math.max(1, Math.ceil(total / DELIVERIES_PER_PAGE))

  const handleOpenChange = (open: boolean) => {
    if (!open) {
      setPage(1)
      onClose()
    }
  }

  return (
    <Dialog open={webhook !== null} onOpenChange={handleOpenChange}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <History size={18} />
            Delivery History — {webhook?.name}
          </DialogTitle>
          <DialogDescription>Recent webhook delivery attempts.</DialogDescription>
        </DialogHeader>

        {isLoading ? (
          <div className="space-y-2">
            <Skeleton className="h-8 w-full" />
            <Skeleton className="h-8 w-full" />
            <Skeleton className="h-8 w-full" />
          </div>
        ) : deliveries.length === 0 ? (
          <p className="text-muted-foreground py-6 text-center text-sm">No deliveries yet.</p>
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Event</TableHead>
                <TableHead>Status</TableHead>
                <TableHead>Attempt</TableHead>
                <TableHead>Duration</TableHead>
                <TableHead>Time</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {deliveries.map((d) => (
                <TableRow key={d.id}>
                  <TableCell className="font-mono text-xs">{d.event}</TableCell>
                  <TableCell>
                    <StatusBadge code={d.status_code} error={d.error} />
                  </TableCell>
                  <TableCell className="text-muted-foreground text-sm">{d.attempt}</TableCell>
                  <TableCell className="text-muted-foreground text-sm">
                    {d.duration_ms !== null ? `${d.duration_ms}ms` : '—'}
                  </TableCell>
                  <TableCell className="text-muted-foreground text-sm">
                    {new Date(d.delivered_at).toLocaleString()}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}

        {totalPages > 1 && (
          <div className="flex items-center justify-end gap-2">
            <Button
              size="sm"
              variant="outline"
              disabled={page <= 1}
              onClick={() => setPage((p) => p - 1)}
            >
              Prev
            </Button>
            <span className="text-muted-foreground text-sm">
              {page} / {totalPages}
            </span>
            <Button
              size="sm"
              variant="outline"
              disabled={page >= totalPages}
              onClick={() => setPage((p) => p + 1)}
            >
              Next
            </Button>
          </div>
        )}
      </DialogContent>
    </Dialog>
  )
}

// ---------------------------------------------------------------------------
// Main page
// ---------------------------------------------------------------------------

export function WebhooksPage() {
  const [searchParams] = useSearchParams()
  const projectId = searchParams.get('project') ?? ''

  const [createOpen, setCreateOpen] = useState(false)
  const [editingWebhook, setEditingWebhook] = useState<Webhook | null>(null)
  const [deletingWebhook, setDeletingWebhook] = useState<Webhook | null>(null)
  const [historyWebhook, setHistoryWebhook] = useState<Webhook | null>(null)

  const { data: webhooks = [], isLoading } = useQuery({
    queryKey: queryKeys.webhooks(projectId),
    queryFn: () => fetchWebhooks(projectId),
    enabled: Boolean(projectId),
  })

  if (!projectId) {
    return (
      <div className="space-y-6 p-6">
        <div>
          <h1 className="text-2xl font-bold">Webhooks</h1>
          <p className="text-muted-foreground mt-1 text-sm">
            Manage webhook notifications for project events.
          </p>
        </div>
        <div className="flex items-center gap-2 rounded-md border border-dashed p-6">
          <Bell className="text-muted-foreground" size={20} />
          <p className="text-muted-foreground text-sm">
            Select a project from the URL to manage its webhooks.
          </p>
        </div>
      </div>
    )
  }

  return (
    <div className="space-y-6 p-6">
      <div className="flex items-start justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold">Webhooks</h1>
          <p className="text-muted-foreground mt-1 text-sm">
            Receive notifications when reports are generated for{' '}
            <span className="font-medium">{projectId}</span>.
          </p>
        </div>
        <Button onClick={() => setCreateOpen(true)} aria-label="Add Webhook">
          <Plus size={16} className="mr-1" />
          Add Webhook
        </Button>
      </div>

      {isLoading ? (
        <div className="space-y-2">
          <Skeleton className="h-10 w-full" />
          <Skeleton className="h-10 w-full" />
        </div>
      ) : webhooks.length === 0 ? (
        <p className="text-muted-foreground py-8 text-center text-sm">
          No webhooks yet. Add one to get started.
        </p>
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Name</TableHead>
              <TableHead>Type</TableHead>
              <TableHead>URL</TableHead>
              <TableHead>Active</TableHead>
              <TableHead>Created</TableHead>
              <TableHead className="w-32" />
            </TableRow>
          </TableHeader>
          <TableBody>
            {webhooks.map((webhook) => (
              <TableRow key={webhook.id}>
                <TableCell className="font-medium">{webhook.name}</TableCell>
                <TableCell>
                  <Badge className={targetTypeClass(webhook.target_type)}>
                    {webhook.target_type}
                  </Badge>
                </TableCell>
                <TableCell className="text-muted-foreground max-w-48 truncate font-mono text-xs">
                  {maskUrl(webhook.url)}
                </TableCell>
                <TableCell>
                  <Badge
                    variant={webhook.is_active ? 'default' : 'secondary'}
                    className="text-xs"
                  >
                    {webhook.is_active ? 'Active' : 'Inactive'}
                  </Badge>
                </TableCell>
                <TableCell className="text-muted-foreground text-sm">
                  {formatDate(webhook.created_at)}
                </TableCell>
                <TableCell>
                  <div className="flex items-center gap-1">
                    <Button
                      size="icon"
                      variant="ghost"
                      onClick={() => setEditingWebhook(webhook)}
                      aria-label={`Edit webhook ${webhook.name}`}
                    >
                      <Pencil size={16} />
                    </Button>
                    <TestWebhookButton projectId={projectId} webhookId={String(webhook.id)} />
                    <Button
                      size="icon"
                      variant="ghost"
                      onClick={() => setHistoryWebhook(webhook)}
                      aria-label={`View delivery history for ${webhook.name}`}
                    >
                      <History size={16} />
                    </Button>
                    <Button
                      size="icon"
                      variant="ghost"
                      onClick={() => setDeletingWebhook(webhook)}
                      aria-label={`Delete webhook ${webhook.name}`}
                    >
                      <Trash2 size={16} />
                    </Button>
                  </div>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}

      <CreateWebhookDialog
        projectId={projectId}
        open={createOpen}
        onOpenChange={setCreateOpen}
      />

      <EditWebhookDialog
        key={editingWebhook?.id ?? ''}
        projectId={projectId}
        webhook={editingWebhook}
        onClose={() => setEditingWebhook(null)}
      />

      <DeleteWebhookDialog
        projectId={projectId}
        webhook={deletingWebhook}
        onClose={() => setDeletingWebhook(null)}
      />

      <DeliveryHistoryDialog
        projectId={projectId}
        webhook={historyWebhook}
        onClose={() => setHistoryWebhook(null)}
      />
    </div>
  )
}
