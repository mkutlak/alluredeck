import { ImageIcon, FileText, File, Layers, Film, Eye, Download } from 'lucide-react'
import { formatBytes } from '@/lib/utils'
import {
  ACCENT_BADGE_CLASSES,
  ACCENT_TEXT_CLASSES,
  INFO_BADGE_CLASSES,
  NEUTRAL_BADGE_CLASSES,
  STATUS_BADGE_CLASSES,
} from '@/lib/status-colors'
import { isPlaywrightTrace } from '@/features/trace/utils'
import { isLogMime } from './utils'
import type { AttachmentEntry } from '@/types/api'

interface AttachmentRowProps {
  attachment: AttachmentEntry
  onView: () => void
}

function getMimeBadge(mimeType: string, name: string): { label: string; className: string } {
  if (mimeType.startsWith('image/')) return { label: 'IMAGE', className: INFO_BADGE_CLASSES }
  if (isLogMime(mimeType))
    return { label: 'LOG', className: 'border-transparent bg-warning/15 text-warning' }
  if (isPlaywrightTrace(name, mimeType)) return { label: 'TRACE', className: ACCENT_BADGE_CLASSES }
  if (mimeType.startsWith('video/'))
    return { label: 'VIDEO', className: STATUS_BADGE_CLASSES.passed }
  return { label: 'OTHER', className: NEUTRAL_BADGE_CLASSES }
}

export function AttachmentRow({ attachment, onView }: AttachmentRowProps) {
  const { mime_type, name, size_bytes, url } = attachment
  const isImage = mime_type.startsWith('image/')
  const isText = isLogMime(mime_type)
  const isTrace = isPlaywrightTrace(name, mime_type)
  const isVideo = mime_type.startsWith('video/')

  const badge = getMimeBadge(mime_type, name)

  return (
    <div className="contents" role="row">
      {/* Cell 1: Icon */}
      <span className="flex items-center">
        {isImage ? (
          <ImageIcon className="text-muted-foreground h-4 w-4" />
        ) : isTrace ? (
          <Layers className={`h-4 w-4 ${ACCENT_TEXT_CLASSES}`} />
        ) : isText ? (
          <FileText className="text-muted-foreground h-4 w-4" />
        ) : isVideo ? (
          <Film className="text-muted-foreground h-4 w-4" />
        ) : (
          <File className="text-muted-foreground h-4 w-4" />
        )}
      </span>

      {/* Cell 2: Filename */}
      <button
        type="button"
        onClick={onView}
        title={name}
        className="truncate text-left text-sm hover:underline"
      >
        {name}
      </button>

      {/* Cell 3: MIME Badge */}
      <span
        className={`inline-flex items-center rounded-full px-2 py-0.5 text-[10px] font-semibold uppercase ${badge.className}`}
      >
        {badge.label}
      </span>

      {/* Cell 4: Size */}
      <span className="text-muted-foreground text-xs whitespace-nowrap">
        {formatBytes(size_bytes)}
      </span>

      {/* Cell 5: Actions */}
      <div className="flex items-center gap-1">
        <button
          type="button"
          aria-label="View"
          onClick={onView}
          className="text-muted-foreground hover:bg-accent hover:text-foreground rounded p-1"
        >
          <Eye className="h-4 w-4" />
        </button>
        <a
          aria-label="Download"
          href={`${url}?dl=1`}
          download
          className="text-muted-foreground hover:bg-accent hover:text-foreground rounded p-1"
        >
          <Download className="h-4 w-4" />
        </a>
      </div>
    </div>
  )
}
