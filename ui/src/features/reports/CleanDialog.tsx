import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Loader2, AlertTriangle } from 'lucide-react'
import { cleanHistory, cleanResults, cleanGroupHistory, cleanGroupResults } from '@/api/reports'
import { extractErrorMessage } from '@/api/client'
import { invalidateProjectQueries, queryKeys } from '@/lib/query-keys'
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
  groupMode?: boolean
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

const GROUP_MESSAGES = {
  results: {
    title: 'Clean all results',
    description: 'This will delete all current test result files for ALL projects in this group.',
    confirm: 'Clean all results',
    successTitle: 'Group results cleaned',
    successDesc: (id: string) => `All results for group "${id}" have been removed.`,
  },
  history: {
    title: 'Clean all history',
    description:
      'This will permanently delete all historical report snapshots for ALL projects in this group.',
    confirm: 'Clean all history',
    successTitle: 'Group history cleaned',
    successDesc: (id: string) => `All report history for group "${id}" has been removed.`,
  },
}

export function CleanDialog({ projectId, mode, open, onOpenChange, groupMode }: CleanDialogProps) {
  const [error, setError] = useState('')
  const queryClient = useQueryClient()
  const msg = groupMode ? GROUP_MESSAGES[mode] : MESSAGES[mode]

  const mutation = useMutation({
    mutationFn: async () => {
      if (groupMode) {
        await (mode === 'history' ? cleanGroupHistory(projectId) : cleanGroupResults(projectId))
      } else {
        await (mode === 'history' ? cleanHistory(projectId) : cleanResults(projectId))
      }
    },
    onSuccess: () => {
      void invalidateProjectQueries(queryClient, projectId)
      if (groupMode) {
        void queryClient.invalidateQueries({ queryKey: queryKeys.dashboard() })
      }
      toast({ title: msg.successTitle, description: msg.successDesc(projectId) })
      onOpenChange(false)
    },
    onError: (err) => {
      setError(extractErrorMessage(err))
    },
  })

  return (
    <AlertDialog
      open={open}
      onOpenChange={(v) => {
        if (!v) setError('')
        onOpenChange(v)
      }}
    >
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle className="text-destructive flex items-center gap-2">
            <AlertTriangle size={18} />
            {msg.title}
          </AlertDialogTitle>
          <AlertDialogDescription>{msg.description} This cannot be undone.</AlertDialogDescription>
        </AlertDialogHeader>
        {error && <p className="text-destructive text-sm">{error}</p>}
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
