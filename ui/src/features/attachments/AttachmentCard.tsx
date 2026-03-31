import { FileText, File, Layers } from 'lucide-react'
import { formatBytes } from '@/lib/utils'
import { isPlaywrightTrace } from '@/features/trace/utils'
import type { AttachmentEntry } from '@/types/api'

interface AttachmentCardProps {
  attachment: AttachmentEntry
  onClick: () => void
}

export function AttachmentCard({ attachment, onClick }: AttachmentCardProps) {
  const isImage = attachment.mime_type.startsWith('image/')
  const isText = attachment.mime_type.startsWith('text/')
  const isTrace = isPlaywrightTrace(attachment.name, attachment.mime_type)

  return (
    <button
      type="button"
      onClick={onClick}
      className="relative cursor-pointer rounded-lg border bg-card p-2 text-left transition-colors hover:bg-accent w-full"
    >
      {isTrace && (
        <span className="absolute right-1.5 top-1.5 z-10 rounded bg-blue-600 px-1 py-0.5 text-[10px] font-bold uppercase leading-none text-white">
          TRACE
        </span>
      )}
      <div className="aspect-square mb-2 flex items-center justify-center overflow-hidden rounded-md bg-muted">
        {isImage ? (
          <img
            src={attachment.url}
            alt={attachment.name}
            loading="lazy"
            crossOrigin="use-credentials"
            className="h-full w-full object-cover"
          />
        ) : isTrace ? (
          <Layers className="h-8 w-8 text-blue-500" />
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
