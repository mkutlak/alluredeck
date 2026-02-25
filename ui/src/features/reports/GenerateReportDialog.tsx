import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Loader2 } from 'lucide-react'
import { generateReport } from '@/api/reports'
import { extractErrorMessage } from '@/api/client'
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
  const [error, setError] = useState('')
  const queryClient = useQueryClient()

  const mutation = useMutation({
    mutationFn: () =>
      generateReport({
        project_id: projectId,
        execution_name: execName || undefined,
        execution_from: execFrom || undefined,
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['report-history', projectId] })
      toast({ title: 'Report generated', description: `New report is ready for "${projectId}".` })
      setExecName('')
      setExecFrom('')
      onOpenChange(false)
    },
    onError: (err) => {
      setError(extractErrorMessage(err))
    },
  })

  return (
    <Dialog
      open={open}
      onOpenChange={(v) => {
        if (!v) {
          setExecName('')
          setExecFrom('')
          setError('')
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
              disabled={mutation.isPending}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="exec-from">CI build URL (optional)</Label>
            <Input
              id="exec-from"
              placeholder="https://ci.example.com/job/123"
              value={execFrom}
              onChange={(e) => setExecFrom(e.target.value)}
              disabled={mutation.isPending}
            />
          </div>
          {error && <p className="text-sm text-destructive">{error}</p>}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button
            onClick={() => {
              setError('')
              mutation.mutate()
            }}
            disabled={mutation.isPending}
          >
            {mutation.isPending && <Loader2 className="animate-spin" />}
            Generate
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
