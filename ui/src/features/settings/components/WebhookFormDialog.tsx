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
  useCreateWebhook,
  useUpdateWebhook,
  type CreateWebhookRequest,
  type UpdateWebhookRequest,
  type Webhook,
} from '../hooks/useWebhooks'

type WebhookTargetType = Webhook['target_type']

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
            className="border-input bg-background placeholder:text-muted-foreground focus-visible:ring-ring flex min-h-[100px] w-full rounded-md border px-3 py-2 text-sm shadow-sm focus-visible:ring-1 focus-visible:outline-none"
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

type CreateMode = { mode: 'create'; projectId: string; open: boolean; onOpenChange: (open: boolean) => void }
type EditMode = { mode: 'edit'; projectId: string; webhook: Webhook | null; onClose: () => void }

export type WebhookFormDialogProps = CreateMode | EditMode

export function WebhookFormDialog(props: WebhookFormDialogProps) {
  if (props.mode === 'create') {
    return <CreateDialog {...props} />
  }
  return <EditDialog {...props} />
}

function CreateDialog({
  projectId,
  open,
  onOpenChange,
}: Omit<CreateMode, 'mode'>) {
  const [form, setForm] = useState<WebhookFormState>(defaultFormState)

  const { mutate: doCreate, isPending } = useCreateWebhook(projectId)

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
    doCreate(req, {
      onSuccess: () => {
        setForm(defaultFormState())
        onOpenChange(false)
      },
    })
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

function EditDialog({ projectId, webhook, onClose }: Omit<EditMode, 'mode'>) {
  const [form, setForm] = useState<WebhookFormState>(() =>
    webhook ? webhookToFormState(webhook) : defaultFormState(),
  )

  const { mutate: doUpdate, isPending } = useUpdateWebhook(projectId, String(webhook?.id ?? ''))

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
    doUpdate(req, {
      onSuccess: () => {
        onClose()
      },
    })
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
