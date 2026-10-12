import { useState } from 'react'
import { useNavigate } from 'react-router'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Loader2, AlertTriangle } from 'lucide-react'
import { extractErrorMessage } from '@/api/client'
import { deleteProject } from '@/api/projects'
import { queryKeys, removeProjectQueries } from '@/lib/query-keys'
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
import { useUIStore } from '@/store/ui'

interface DeleteProjectDialogProps {
  projectId: string
  /** Name shown in the dialog and typed to confirm; the id alone is not meaningful to a reader. */
  label?: string
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function DeleteProjectDialog({
  projectId,
  label = projectId,
  open,
  onOpenChange,
}: DeleteProjectDialogProps) {
  const [confirmText, setConfirmText] = useState('')
  const [error, setError] = useState('')
  const queryClient = useQueryClient()
  const navigate = useNavigate()

  const mutation = useMutation({
    mutationFn: () => deleteProject(projectId),
    onSuccess: () => {
      const { lastProjectId, clearLastProjectId } = useUIStore.getState()
      if (lastProjectId === projectId) {
        clearLastProjectId()
      }
      void queryClient.invalidateQueries({ queryKey: queryKeys.projects })
      void queryClient.invalidateQueries({ queryKey: queryKeys.dashboard() })
      removeProjectQueries(queryClient, projectId)
      toast({ title: 'Project deleted', description: `"${label}" has been removed.` })
      onOpenChange(false)
      void navigate('/', { replace: true })
    },
    onError: (err) => {
      setError(extractErrorMessage(err))
    },
  })

  const canDelete = confirmText === label

  const handleDelete = () => {
    setError('')
    if (!canDelete) return
    mutation.mutate()
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(v) => {
        if (!v) {
          setConfirmText('')
          setError('')
        }
        onOpenChange(v)
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle className="text-destructive flex items-center gap-2">
            <AlertTriangle size={18} />
            Delete project
          </DialogTitle>
          <DialogDescription>
            This will permanently delete the project <strong>{label}</strong> and all its
            reports and results. This action cannot be undone.
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-2">
          <Label htmlFor="delete-confirm">
            Type <span className="font-mono font-semibold">{label}</span> to confirm
          </Label>
          <Input
            id="delete-confirm"
            placeholder={label}
            value={confirmText}
            onChange={(e) => setConfirmText(e.target.value)}
            disabled={mutation.isPending}
          />
          {error && <p className="text-destructive text-sm">{error}</p>}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button
            variant="destructive"
            disabled={!canDelete || mutation.isPending}
            onClick={handleDelete}
          >
            {mutation.isPending && <Loader2 className="animate-spin" />}
            Delete project
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
