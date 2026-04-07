import { useState } from 'react'
import { Download, Loader2 } from 'lucide-react'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { formatBytes } from '@/lib/utils'
import { downloadAttachment } from '@/api/attachments'
import { isPlaywrightTrace } from '@/features/trace/utils'
import { AttachmentTextPreview } from './AttachmentTextPreview'
import type { AttachmentEntry } from '@/types/api'

interface AttachmentLightboxProps {
  attachment: AttachmentEntry
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function AttachmentLightbox({ attachment, open, onOpenChange }: AttachmentLightboxProps) {
  const [downloading, setDownloading] = useState(false)
  const isImage = attachment.mime_type.startsWith('image/')
  const isVideo = attachment.mime_type.startsWith('video/')
  const isText =
    attachment.mime_type.startsWith('text/') ||
    attachment.mime_type === 'application/json' ||
    attachment.mime_type === 'application/xml'

  // Playwright traces are handled by the card navigation — never open in lightbox.
  if (isPlaywrightTrace(attachment.name, attachment.mime_type)) return null

  async function handleDownload() {
    setDownloading(true)
    try {
      await downloadAttachment(attachment.url, attachment.name)
    } finally {
      setDownloading(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className={
          isImage || isVideo || isText
            ? 'max-h-[85vh] w-full max-w-[90vw] grid-rows-[auto_1fr_auto]'
            : undefined
        }
      >
        <DialogHeader>
          <DialogTitle>{attachment.name}</DialogTitle>
          <DialogDescription>{formatBytes(attachment.size_bytes)}</DialogDescription>
        </DialogHeader>

        <div className="flex min-h-0 flex-col gap-4 overflow-hidden">
          {isImage ? (
            <img
              src={attachment.url}
              alt={attachment.name}
              crossOrigin="use-credentials"
              className="min-h-0 w-full flex-1 object-contain"
            />
          ) : isVideo ? (
            <video
              src={attachment.url}
              controls
              crossOrigin="use-credentials"
              className="min-h-0 w-full flex-1 object-contain"
            >
              <track kind="captions" />
            </video>
          ) : isText ? (
            <AttachmentTextPreview
              url={attachment.url}
              mimeType={attachment.mime_type}
              fileName={attachment.name}
            />
          ) : (
            <p className="text-muted-foreground text-sm">
              {attachment.mime_type} · {formatBytes(attachment.size_bytes)}
            </p>
          )}

          <div className="flex justify-end">
            <Button variant="outline" size="sm" disabled={downloading} onClick={handleDownload}>
              {downloading ? (
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
              ) : (
                <Download className="mr-2 h-4 w-4" />
              )}
              Download
            </Button>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  )
}
