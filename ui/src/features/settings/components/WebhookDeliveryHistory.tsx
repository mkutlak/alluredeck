import { useState } from 'react'
import { History } from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
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
  DELIVERIES_PER_PAGE,
  useDeliveryHistory,
  type Webhook,
  type WebhookDelivery,
} from '../hooks/useWebhooks'

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

interface WebhookDeliveryHistoryProps {
  projectId: string
  webhook: Webhook | null
  onClose: () => void
}

export function WebhookDeliveryHistory({
  projectId,
  webhook,
  onClose,
}: WebhookDeliveryHistoryProps) {
  const [page, setPage] = useState(1)

  const { data, isLoading } = useDeliveryHistory(projectId, webhook, page)

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
                    {formatDate(d.delivered_at)}
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
