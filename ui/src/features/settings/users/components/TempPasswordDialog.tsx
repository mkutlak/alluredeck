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

interface TempPasswordDialogProps {
  open: boolean
  email: string
  tempPassword: string
  onClose: () => void
}

export function TempPasswordDialog({ open, email, tempPassword, onClose }: TempPasswordDialogProps) {
  const [copied, setCopied] = useState(false)

  const handleCopy = () => {
    void navigator.clipboard.writeText(tempPassword).then(() => {
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    })
  }

  return (
    <Dialog open={open} onOpenChange={(isOpen) => !isOpen && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Temporary Password</DialogTitle>
          <DialogDescription>
            Copy this temporary password now. It won&apos;t be shown again.
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-3">
          <div className="text-sm">
            <span className="text-muted-foreground">Email: </span>
            <span className="font-medium">{email}</span>
          </div>
          <div className="space-y-1">
            <p className="text-sm font-medium">Temporary Password</p>
            <div className="bg-muted flex items-center gap-2 rounded-md p-3">
              <code className="flex-1 break-all font-mono text-sm">{tempPassword}</code>
              <Button
                type="button"
                size="icon"
                variant="ghost"
                onClick={handleCopy}
                aria-label="Copy password"
              >
                {copied ? <Check size={16} /> : <Copy size={16} />}
              </Button>
            </div>
          </div>
        </div>
        <DialogFooter>
          <Button onClick={onClose}>Done</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
