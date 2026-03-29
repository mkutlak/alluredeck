import { FileText, File } from 'lucide-react'
import { formatBytes } from '@/lib/utils'
import type { AttachmentEntry } from '@/types/api'

interface AttachmentCardProps {
  attachment: AttachmentEntry
  onClick: () => void
}

export function AttachmentCard({ attachment, onClick }: AttachmentCardProps) {
  const isImage = attachment.mime_type.startsWith('image/')
  const isText = attachment.mime_type.startsWith('text/')

  return (
    <button
      type="button"
      onClick={onClick}
      className="cursor-pointer rounded-lg border bg-card p-2 transition-colors hover:bg-accent w-full text-left"
    >
      <div className="aspect-square mb-2 flex items-center justify-center overflow-hidden rounded-md bg-muted">
        {isImage ? (
          <img
            src={attachment.url}
            alt={attachment.name}
            loading="lazy"
            crossOrigin="use-credentials"
            className="h-full w-full object-cover"
          />
        ) : isText ? (
          <FileText className="h-8 w-8 text-muted-foreground" />
        ) : (
          <File className="h-8 w-8 text-muted-foreground" />
        )}
      </div>
      <p className="truncate text-xs font-medium" title={attachment.name}>
        {attachment.name}
      </p>
      <p className="text-xs text-muted-foreground">{formatBytes(attachment.size_bytes)}</p>
    </button>
  )
}
