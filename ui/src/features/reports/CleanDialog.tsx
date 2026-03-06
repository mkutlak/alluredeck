import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Loader2, AlertTriangle } from 'lucide-react'
import { cleanHistory, cleanResults } from '@/api/reports'
import { extractErrorMessage } from '@/api/client'
import { invalidateProjectQueries } from '@/lib/query-keys'
import { Button } from '@/components/ui/button'
import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { toast } from '@/components/ui/use-toast'
import { useState } from 'react'

type CleanMode = 'results' | 'history'

interface CleanDialogProps {
  projectId: string
  mode: CleanMode
  open: boolean
  onOpenChange: (open: boolean) => void
}

const MESSAGES = {
  results: {
    title: 'Clean results',
    description: 'This will delete all current test result files for this project.',
    confirm: 'Clean results',
    successTitle: 'Results cleaned',
    successDesc: (id: string) => `All results for "${id}" have been removed.`,
  },
  history: {
    title: 'Clean history',
    description: 'This will permanently delete all historical report snapshots for this project.',
    confirm: 'Clean history',
    successTitle: 'History cleaned',
    successDesc: (id: string) => `All report history for "${id}" has been removed.`,
  },
}

export function CleanDialog({ projectId, mode, open, onOpenChange }: CleanDialogProps) {
  const [error, setError] = useState('')
  const queryClient = useQueryClient()
  const msg = MESSAGES[mode]

  const mutation = useMutation({
    mutationFn: () => (mode === 'history' ? cleanHistory(projectId) : cleanResults(projectId)),
    onSuccess: () => {
      void invalidateProjectQueries(queryClient, projectId)
      toast({ title: msg.successTitle, description: msg.successDesc(projectId) })
      onOpenChange(false)
    },
    onError: (err) => {
      setError(extractErrorMessage(err))
    },
  })

  return (
    <AlertDialog open={open} onOpenChange={(v) => { if (!v) setError(''); onOpenChange(v) }}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle className="flex items-center gap-2 text-destructive">
            <AlertTriangle size={18} />
            {msg.title}
          </AlertDialogTitle>
          <AlertDialogDescription>{msg.description} This cannot be undone.</AlertDialogDescription>
        </AlertDialogHeader>
        {error && <p className="text-sm text-destructive">{error}</p>}
        <AlertDialogFooter>
          <AlertDialogCancel>Cancel</AlertDialogCancel>
          <Button
            variant="destructive"
            disabled={mutation.isPending}
            onClick={() => {
              setError('')
              mutation.mutate()
            }}
          >
            {mutation.isPending && <Loader2 className="animate-spin" />}
            {msg.confirm}
          </Button>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
