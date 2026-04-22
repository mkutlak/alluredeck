import { ImageIcon, FileText, File, Layers, Film, Eye, Download } from 'lucide-react'
import { formatBytes } from '@/lib/utils'
import { isPlaywrightTrace } from '@/features/trace/utils'
import { isLogMime } from './utils'
import type { AttachmentEntry } from '@/types/api'

interface AttachmentRowProps {
  attachment: AttachmentEntry
  onView: () => void
}

function getMimeBadge(mimeType: string, name: string): { label: string; className: string } {
  if (mimeType.startsWith('image/'))
    return {
      label: 'IMAGE',
      className: 'bg-sky-100 text-sky-700 dark:bg-sky-900 dark:text-sky-300',
    }
  if (isLogMime(mimeType))
    return {
      label: 'LOG',
      className: 'bg-amber-100 text-amber-700 dark:bg-amber-900 dark:text-amber-300',
    }
  if (isPlaywrightTrace(name, mimeType))
    return {
      label: 'TRACE',
      className: 'bg-violet-100 text-violet-700 dark:bg-violet-900 dark:text-violet-300',
    }
  if (mimeType.startsWith('video/'))
    return {
      label: 'VIDEO',
      className: 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900 dark:text-emerald-300',
    }
  return {
    label: 'OTHER',
    className: 'bg-gray-100 text-gray-700 dark:bg-gray-800 dark:text-gray-300',
  }
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
          <Layers className="h-4 w-4 text-violet-500" />
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
