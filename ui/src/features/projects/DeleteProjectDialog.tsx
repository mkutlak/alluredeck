import { useState } from 'react'
import { useNavigate } from 'react-router'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Loader2, AlertTriangle } from 'lucide-react'
import { extractErrorMessage, apiClient } from '@/api/client'
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

interface DeleteProjectDialogProps {
  projectId: string
  open: boolean
  onOpenChange: (open: boolean) => void
}

async function deleteProject(projectId: string): Promise<void> {
  await apiClient.delete(`/projects/${encodeURIComponent(projectId)}`)
}

export function DeleteProjectDialog({ projectId, open, onOpenChange }: DeleteProjectDialogProps) {
  const [confirmText, setConfirmText] = useState('')
  const [error, setError] = useState('')
  const queryClient = useQueryClient()
  const navigate = useNavigate()

  const mutation = useMutation({
    mutationFn: () => deleteProject(projectId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['projects'] })
      queryClient.invalidateQueries({ queryKey: ['dashboard'] })
      toast({ title: 'Project deleted', description: `"${projectId}" has been removed.` })
      onOpenChange(false)
      navigate('/', { replace: true })
    },
    onError: (err) => {
      setError(extractErrorMessage(err))
    },
  })

  const canDelete = confirmText === projectId

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
          <DialogTitle className="flex items-center gap-2 text-destructive">
            <AlertTriangle size={18} />
            Delete project
          </DialogTitle>
          <DialogDescription>
            This will permanently delete the project <strong>{projectId}</strong> and all its
            reports and results. This action cannot be undone.
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-2">
          <Label htmlFor="delete-confirm">
            Type <span className="font-mono font-semibold">{projectId}</span> to confirm
          </Label>
          <Input
            id="delete-confirm"
            placeholder={projectId}
            value={confirmText}
            onChange={(e) => setConfirmText(e.target.value)}
            disabled={mutation.isPending}
          />
          {error && <p className="text-sm text-destructive">{error}</p>}
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
