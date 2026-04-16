import { useState } from 'react'
import { Check, Copy } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import type { APIKeyCreated } from '@/types/api'

interface CreatedKeyDialogProps {
  apiKey: APIKeyCreated | null
  onClose: () => void
}

export function CreatedKeyDialog({ apiKey, onClose }: CreatedKeyDialogProps) {
  const [copied, setCopied] = useState(false)

  const handleCopy = () => {
    if (!apiKey) return
    void navigator.clipboard.writeText(apiKey.key).then(() => {
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    })
  }

  return (
    <Dialog open={apiKey !== null} onOpenChange={(open) => !open && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>API Key Created</DialogTitle>
          <DialogDescription>
            Copy this key now. You won&apos;t be able to see it again.
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-3">
          <div className="bg-muted flex items-center gap-2 rounded-md p-3">
            <code className="flex-1 font-mono text-sm break-all">{apiKey?.key ?? ''}</code>
            <Button
              type="button"
              size="icon"
              variant="ghost"
              onClick={handleCopy}
              aria-label="Copy key"
            >
              {copied ? <Check size={16} /> : <Copy size={16} />}
            </Button>
          </div>
        </div>
        <DialogFooter>
          <Button onClick={onClose}>Done</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
