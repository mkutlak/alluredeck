import { useState } from 'react'
import { useNavigate } from 'react-router'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { renameProject } from '@/api/projects'
import { extractErrorMessage } from '@/api/client'
import { queryKeys } from '@/lib/query-keys'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

interface Props {
  projectId: string
  open: boolean
  onOpenChange: (open: boolean) => void
}

const ID_PATTERN = /^[a-zA-Z0-9_-]+$/

export function RenameProjectDialog({ projectId, open, onOpenChange }: Props) {
  const [newId, setNewId] = useState('')
  const [error, setError] = useState<string | null>(null)
  const navigate = useNavigate()
  const qc = useQueryClient()

  const { mutate, isPending } = useMutation({
    mutationFn: () => renameProject(projectId, newId.trim()),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.projects })
      void qc.invalidateQueries({ queryKey: queryKeys.dashboard() })
      onOpenChange(false)
      navigate(`/projects/${encodeURIComponent(newId.trim())}`)
    },
    onError: (e) => setError(extractErrorMessage(e)),
  })

  function handleSubmit() {
    const trimmed = newId.trim()
    if (!trimmed) {
      setError('Project ID is required')
      return
    }
    if (trimmed.length > 100) {
      setError('Max 100 characters')
      return
    }
    if (!ID_PATTERN.test(trimmed)) {
      setError('Only letters, numbers, hyphens, and underscores')
      return
    }
    if (trimmed === projectId) {
      setError('New ID must differ from current')
      return
    }
    setError(null)
    mutate()
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Rename project</DialogTitle>
          <DialogDescription>
            Rename <span className="font-mono font-semibold">{projectId}</span> to a new ID.
            All builds and reports will be preserved. This operation may cause data loss if
            interrupted and cannot be undone.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-2">
          <Label htmlFor="new-project-id">New project ID</Label>
          <Input
            id="new-project-id"
            value={newId}
            onChange={(e) => {
              setNewId(e.target.value)
              setError(null)
            }}
            placeholder="my-new-project-name"
            onKeyDown={(e) => {
              if (e.key === 'Enter') handleSubmit()
            }}
          />
          {error && <p className="text-destructive text-sm">{error}</p>}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={isPending}>
            Cancel
          </Button>
          <Button onClick={handleSubmit} disabled={isPending}>
            {isPending ? 'Renaming...' : 'Rename'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
