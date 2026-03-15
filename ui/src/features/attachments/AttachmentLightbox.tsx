import { Download } from 'lucide-react'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { formatBytes } from '@/lib/utils'
import type { AttachmentEntry } from '@/types/api'

interface AttachmentLightboxProps {
  attachment: AttachmentEntry
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function AttachmentLightbox({ attachment, open, onOpenChange }: AttachmentLightboxProps) {
  const isImage = attachment.mime_type.startsWith('image/')
  const isText =
    attachment.mime_type.startsWith('text/') ||
    attachment.mime_type === 'application/json' ||
    attachment.mime_type === 'application/xml'

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className={isImage ? 'max-w-4xl' : undefined}>
        <DialogHeader>
          <DialogTitle>{attachment.name}</DialogTitle>
          <DialogDescription>{formatBytes(attachment.size_bytes)}</DialogDescription>
        </DialogHeader>

        <div className="flex flex-col gap-4">
          {isImage ? (
            <img
              src={attachment.url}
              alt={attachment.name}
              className="max-h-[80vh] w-full object-contain"
            />
          ) : isText ? (
            <p className="text-sm text-muted-foreground">
              Preview not available for text files
            </p>
          ) : (
            <p className="text-sm text-muted-foreground">
              {attachment.mime_type} · {formatBytes(attachment.size_bytes)}
            </p>
          )}

          <div className="flex justify-end">
            <Button asChild variant="outline" size="sm">
              <a
                href={attachment.url}
                download={attachment.name}
                target="_blank"
                rel="noopener noreferrer"
              >
                <Download className="mr-2 h-4 w-4" />
                Download
              </a>
            </Button>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  )
}
