import { useState } from 'react'
import { History, Loader2, Pencil, Send, Trash2 } from 'lucide-react'

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
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { formatDate } from '@/lib/utils'
import {
  useDeleteWebhook,
  useTestWebhook,
  useWebhookListQuery,
  type Webhook,
} from '../hooks/useWebhooks'
import { WebhookDeliveryHistory } from './WebhookDeliveryHistory'
import { WebhookFormDialog } from './WebhookFormDialog'

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

interface TestWebhookButtonProps {
  projectId: string
  webhookId: string
}

function TestWebhookButton({ projectId, webhookId }: TestWebhookButtonProps) {
  const { mutate: doTest, isPending } = useTestWebhook(projectId, webhookId)

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

interface DeleteWebhookDialogProps {
  projectId: string
  webhook: Webhook | null
  onClose: () => void
}

function DeleteWebhookDialog({ projectId, webhook, onClose }: DeleteWebhookDialogProps) {
  const { mutate: doDelete, isPending } = useDeleteWebhook(projectId)

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
            onClick={() =>
              webhook &&
              doDelete(String(webhook.id), {
                onSuccess: () => {
                  onClose()
                },
              })
            }
          >
            Delete
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}

export interface WebhookListProps {
  projectId: string
  createOpen: boolean
  onCreateOpenChange: (open: boolean) => void
}

export function WebhookList({ projectId, createOpen, onCreateOpenChange }: WebhookListProps) {
  const [editingWebhook, setEditingWebhook] = useState<Webhook | null>(null)
  const [deletingWebhook, setDeletingWebhook] = useState<Webhook | null>(null)
  const [historyWebhook, setHistoryWebhook] = useState<Webhook | null>(null)

  const { data: webhooks = [], isLoading } = useWebhookListQuery(projectId)

  return (
    <>
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
                  <Badge variant={webhook.is_active ? 'default' : 'secondary'} className="text-xs">
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

      <WebhookFormDialog
        mode="create"
        projectId={projectId}
        open={createOpen}
        onOpenChange={onCreateOpenChange}
      />

      <WebhookFormDialog
        key={editingWebhook?.id ?? 'none'}
        mode="edit"
        projectId={projectId}
        webhook={editingWebhook}
        onClose={() => setEditingWebhook(null)}
      />

      <DeleteWebhookDialog
        projectId={projectId}
        webhook={deletingWebhook}
        onClose={() => setDeletingWebhook(null)}
      />

      <WebhookDeliveryHistory
        projectId={projectId}
        webhook={historyWebhook}
        onClose={() => setHistoryWebhook(null)}
      />
    </>
  )
}
