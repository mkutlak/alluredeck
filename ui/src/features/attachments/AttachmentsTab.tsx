import { useState } from 'react'
import { useParams } from 'react-router'
import { useQuery } from '@tanstack/react-query'
import { Paperclip } from 'lucide-react'
import { fetchAttachments } from '@/api/attachments'
import { queryKeys } from '@/lib/query-keys'
import { Skeleton } from '@/components/ui/skeleton'
import { Button } from '@/components/ui/button'
import { AttachmentCard } from './AttachmentCard'
import { AttachmentLightbox } from './AttachmentLightbox'
import type { AttachmentEntry } from '@/types/api'

const REPORT_ID = 'latest'

const MIME_FILTERS = [
  { label: 'All', value: '' },
  { label: 'Images', value: 'image' },
  { label: 'Logs', value: 'text' },
  { label: 'Other', value: 'other' },
] as const

type MimeFilter = (typeof MIME_FILTERS)[number]['value']

export function AttachmentsTab() {
  const { id: projectId } = useParams<{ id: string }>()
  const [selectedAttachment, setSelectedAttachment] = useState<AttachmentEntry | null>(null)
  const [mimeFilter, setMimeFilter] = useState<MimeFilter>('')

  const { data, isLoading } = useQuery({
    queryKey: queryKeys.attachments(projectId!, REPORT_ID),
    queryFn: () => fetchAttachments(projectId!, REPORT_ID),
    enabled: !!projectId,
    staleTime: 10_000,
  })

  if (!projectId) return null

  if (isLoading) {
    return (
      <div className="space-y-4">
        <Skeleton className="h-8 w-48" />
        <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6">
          {Array.from({ length: 6 }).map((_, i) => (
            <Skeleton key={i} className="aspect-square rounded-lg" />
          ))}
        </div>
      </div>
    )
  }

  const allAttachments = data?.attachments ?? []
  const total = data?.total ?? 0

  const filtered = allAttachments.filter((a) => {
    if (mimeFilter === '') return true
    if (mimeFilter === 'image') return a.mime_type.startsWith('image/')
    if (mimeFilter === 'text') return a.mime_type.startsWith('text/')
    if (mimeFilter === 'other')
      return !a.mime_type.startsWith('image/') && !a.mime_type.startsWith('text/')
    return true
  })

  return (
    <div className="space-y-4">
      <div>
        <h1 className="font-mono text-2xl font-semibold">{projectId}</h1>
        <p className="text-muted-foreground text-sm">
          Attachments · {total} total
        </p>
      </div>

      <div className="flex flex-wrap gap-2">
        {MIME_FILTERS.map(({ label, value }) => (
          <Button
            key={value}
            variant={mimeFilter === value ? 'default' : 'outline'}
            size="sm"
            onClick={() => setMimeFilter(value)}
          >
            {label}
          </Button>
        ))}
      </div>

      {filtered.length === 0 ? (
        <div className="flex flex-col items-center gap-3 rounded-lg border border-dashed py-16 text-center">
          <Paperclip className="h-8 w-8 text-muted-foreground/40" />
          <p className="font-medium">No attachments found</p>
          <p className="text-muted-foreground text-sm">
            {mimeFilter !== ''
              ? 'Try selecting a different filter.'
              : 'Generate a report with attachments to see them here.'}
          </p>
        </div>
      ) : (
        <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6">
          {filtered.map((attachment) => (
            <AttachmentCard
              key={attachment.id}
              attachment={attachment}
              onClick={() => setSelectedAttachment(attachment)}
            />
          ))}
        </div>
      )}

      {selectedAttachment && (
        <AttachmentLightbox
          attachment={selectedAttachment}
          open={!!selectedAttachment}
          onOpenChange={(open) => {
            if (!open) setSelectedAttachment(null)
          }}
        />
      )}
    </div>
  )
}
