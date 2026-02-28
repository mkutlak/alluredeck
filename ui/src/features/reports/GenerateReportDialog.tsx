import { useState, useEffect, useRef } from 'react'
import { useMutation } from '@tanstack/react-query'
import { Loader2 } from 'lucide-react'
import { generateReport } from '@/api/reports'
import { extractErrorMessage } from '@/api/client'
import { useJobPolling } from '@/hooks/useJobPolling'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
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

interface GenerateReportDialogProps {
  projectId: string
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function GenerateReportDialog({
  projectId,
  open,
  onOpenChange,
}: GenerateReportDialogProps) {
  const [execName, setExecName] = useState('')
  const [execFrom, setExecFrom] = useState('')
  const [mutationError, setMutationError] = useState('')
  const [jobId, setJobId] = useState<string | null>(null)

  const { isPolling, isCompleted, isFailed, error: jobError } = useJobPolling(projectId, jobId)

  // Ref guards so the effect only fires once per completed job (no setState inside effect)
  const handledJobRef = useRef<string | null>(null)
  useEffect(() => {
    if (isCompleted && jobId && handledJobRef.current !== jobId) {
      handledJobRef.current = jobId
      toast({ title: 'Report generated', description: `New report is ready for "${projectId}".` })
      onOpenChange(false)
    }
  }, [isCompleted, jobId, projectId, onOpenChange])

  // Derive display error: job failure error takes priority over mutation error
  const displayError = isFailed && jobError ? jobError : mutationError

  const mutation = useMutation({
    mutationFn: () =>
      generateReport({
        project_id: projectId,
        execution_name: execName || undefined,
        execution_from: execFrom || undefined,
      }),
    onSuccess: (data) => {
      setJobId(data.data.job_id)
    },
    onError: (err) => {
      setMutationError(extractErrorMessage(err))
    },
  })

  const isActive = mutation.isPending || isPolling
  const buttonLabel = mutation.isPending ? 'Queuing...' : isPolling ? 'Generating...' : 'Generate'

  return (
    <Dialog
      open={open}
      onOpenChange={(v) => {
        if (!v) {
          setExecName('')
          setExecFrom('')
          setMutationError('')
          setJobId(null)
        }
        onOpenChange(v)
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Generate report</DialogTitle>
          <DialogDescription>
            Generate an Allure report for{' '}
            <span className="font-mono font-medium">{projectId}</span> from the current results.
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="exec-name">Execution name (optional)</Label>
            <Input
              id="exec-name"
              placeholder="e.g. Release 1.2.3"
              value={execName}
              onChange={(e) => setExecName(e.target.value)}
              disabled={isActive}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="exec-from">CI build URL (optional)</Label>
            <Input
              id="exec-from"
              placeholder="https://ci.example.com/job/123"
              value={execFrom}
              onChange={(e) => setExecFrom(e.target.value)}
              disabled={isActive}
            />
          </div>
          {displayError && <p className="text-sm text-destructive">{displayError}</p>}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button
            onClick={() => {
              setMutationError('')
              mutation.mutate()
            }}
            disabled={isActive}
          >
            {isActive && <Loader2 className="animate-spin" />}
            {buttonLabel}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
