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
  const isText =
    attachment.mime_type.startsWith('text/') ||
    attachment.mime_type === 'application/json' ||
    attachment.mime_type === 'application/xml'

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
      <DialogContent className={isImage || isText ? 'max-w-[90vw] w-full' : undefined}>
        <DialogHeader>
          <DialogTitle>{attachment.name}</DialogTitle>
          <DialogDescription>{formatBytes(attachment.size_bytes)}</DialogDescription>
        </DialogHeader>

        <div className="flex flex-col gap-4">
          {isImage ? (
            <img
              src={attachment.url}
              alt={attachment.name}
              crossOrigin="use-credentials"
              className="max-h-[80vh] w-full object-contain"
            />
          ) : isText ? (
            <AttachmentTextPreview
              url={attachment.url}
              mimeType={attachment.mime_type}
              fileName={attachment.name}
            />
          ) : (
            <p className="text-sm text-muted-foreground">
              {attachment.mime_type} · {formatBytes(attachment.size_bytes)}
            </p>
          )}

          <div className="flex justify-end">
            <Button
              variant="outline"
              size="sm"
              disabled={downloading}
              onClick={handleDownload}
            >
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
