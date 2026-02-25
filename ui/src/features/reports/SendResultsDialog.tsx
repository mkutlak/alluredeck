import { useRef, useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Loader2, Upload, X, FileText } from 'lucide-react'
import { sendResultsMultipart, generateReport } from '@/api/reports'
import { extractErrorMessage } from '@/api/client'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Label } from '@/components/ui/label'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { toast } from '@/components/ui/use-toast'
import { cn } from '@/lib/utils'

interface SendResultsDialogProps {
  projectId: string
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function SendResultsDialog({ projectId, open, onOpenChange }: SendResultsDialogProps) {
  const [files, setFiles] = useState<File[]>([])
  const [dragging, setDragging] = useState(false)
  const [error, setError] = useState('')
  const [generateAfterUpload, setGenerateAfterUpload] = useState(true)
  const [uploadPhase, setUploadPhase] = useState<'uploading' | 'generating' | null>(null)
  const inputRef = useRef<HTMLInputElement>(null)
  const queryClient = useQueryClient()

  const mutation = useMutation({
    mutationFn: async () => {
      setUploadPhase('uploading')
      await sendResultsMultipart(projectId, files)
      if (generateAfterUpload) {
        setUploadPhase('generating')
        await generateReport({ project_id: projectId })
      }
    },
    onSuccess: () => {
      toast({
        title: generateAfterUpload ? 'Report generated' : 'Results sent',
        description: generateAfterUpload
          ? `${files.length} file${files.length !== 1 ? 's' : ''} uploaded and report generated for "${projectId}".`
          : `${files.length} file${files.length !== 1 ? 's' : ''} uploaded to "${projectId}".`,
      })
      void queryClient.invalidateQueries({ queryKey: ['report-history', projectId] })
      setFiles([])
      setUploadPhase(null)
      onOpenChange(false)
    },
    onError: (err) => {
      setUploadPhase(null)
      setError(extractErrorMessage(err))
    },
  })

  const addFiles = (incoming: FileList | null) => {
    if (!incoming) return
    setFiles((prev) => {
      const existing = new Set(prev.map((f) => f.name))
      return [...prev, ...Array.from(incoming).filter((f) => !existing.has(f.name))]
    })
  }

  const removeFile = (name: string) => setFiles((prev) => prev.filter((f) => f.name !== name))

  return (
    <Dialog
      open={open}
      onOpenChange={(v) => {
        if (!v) {
          setFiles([])
          setError('')
          setUploadPhase(null)
        }
        onOpenChange(v)
      }}
    >
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>Send results</DialogTitle>
          <DialogDescription>
            Upload Allure result files (.json, .xml, attachments) for project{' '}
            <span className="font-mono font-medium">{projectId}</span>.
          </DialogDescription>
        </DialogHeader>

        {/* Drop zone */}
        <div
          role="button"
          tabIndex={0}
          aria-label="Drop zone for result files"
          className={cn(
            'flex cursor-pointer flex-col items-center justify-center gap-2 rounded-lg border-2 border-dashed p-8 text-sm transition-colors',
            dragging ? 'border-primary bg-primary/5' : 'border-input hover:border-primary/50',
          )}
          onClick={() => inputRef.current?.click()}
          onKeyDown={(e) => e.key === 'Enter' && inputRef.current?.click()}
          onDragOver={(e) => {
            e.preventDefault()
            setDragging(true)
          }}
          onDragLeave={() => setDragging(false)}
          onDrop={(e) => {
            e.preventDefault()
            setDragging(false)
            addFiles(e.dataTransfer.files)
          }}
        >
          <Upload size={24} className="text-muted-foreground" />
          <p className="text-muted-foreground">
            Drag & drop files here, or <span className="text-primary">browse</span>
          </p>
          <input
            ref={inputRef}
            type="file"
            multiple
            className="hidden"
            onChange={(e) => addFiles(e.target.files)}
          />
        </div>

        {/* File list */}
        {files.length > 0 && (
          <div className="max-h-40 space-y-1 overflow-y-auto rounded-md border p-2">
            {files.map((f) => (
              <div key={f.name} className="flex items-center gap-2 rounded px-2 py-1 text-xs hover:bg-muted">
                <FileText size={12} className="shrink-0 text-muted-foreground" />
                <span className="flex-1 truncate font-mono">{f.name}</span>
                <button
                  onClick={() => removeFile(f.name)}
                  className="text-muted-foreground hover:text-foreground"
                  aria-label={`Remove ${f.name}`}
                >
                  <X size={12} />
                </button>
              </div>
            ))}
          </div>
        )}

        {/* Auto-generate checkbox */}
        <div className="flex items-center gap-2">
          <Checkbox
            id="generate-after-upload"
            checked={generateAfterUpload}
            onCheckedChange={(v: boolean | 'indeterminate') => setGenerateAfterUpload(v === true)}
            disabled={mutation.isPending}
          />
          <Label htmlFor="generate-after-upload" className="text-sm font-normal cursor-pointer">
            Generate report after upload
          </Label>
        </div>

        {error && <p className="text-sm text-destructive">{error}</p>}

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={mutation.isPending}>
            Cancel
          </Button>
          <Button
            disabled={files.length === 0 || mutation.isPending}
            onClick={() => {
              setError('')
              mutation.mutate()
            }}
          >
            {mutation.isPending && <Loader2 className="animate-spin" />}
            {uploadPhase === 'generating'
              ? 'Generating…'
              : uploadPhase === 'uploading'
                ? 'Uploading…'
                : `Upload${files.length > 0 ? ` (${files.length})` : ''}`}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
