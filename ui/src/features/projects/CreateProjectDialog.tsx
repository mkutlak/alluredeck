import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Loader2 } from 'lucide-react'
import { createProject, getProjects } from '@/api/projects'
import { extractErrorMessage } from '@/api/client'
import { queryKeys } from '@/lib/query-keys'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { toast } from '@/components/ui/use-toast'

interface CreateProjectDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function CreateProjectDialog({ open, onOpenChange }: CreateProjectDialogProps) {
  const [projectId, setProjectId] = useState('')
  const [parentId, setParentId] = useState('')
  const [error, setError] = useState('')
  const queryClient = useQueryClient()

  const { data: projectsResp } = useQuery({
    queryKey: queryKeys.projects,
    queryFn: () => getProjects(1, 200),
    enabled: open,
  })

  // Only top-level projects (no parent_id) can be parents
  const availableParents = (projectsResp?.data ?? []).filter((p) => !p.parent_id)

  const mutation = useMutation({
    mutationFn: createProject,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.projects })
      void queryClient.invalidateQueries({ queryKey: queryKeys.dashboard() })
      toast({ title: 'Project created', description: `"${projectId}" is ready.` })
      setProjectId('')
      setParentId('')
      onOpenChange(false)
    },
    onError: (err) => {
      setError(extractErrorMessage(err))
    },
  })

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    setError('')
    const id = projectId.trim()
    if (!id) {
      setError('Project ID is required.')
      return
    }
    mutation.mutate({ id, ...(parentId ? { parent_id: parentId } : {}) })
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(v) => {
        if (!v) {
          setProjectId('')
          setParentId('')
          setError('')
        }
        onOpenChange(v)
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Create project</DialogTitle>
          <DialogDescription>
            Enter a unique identifier for the new project. Only letters, numbers, dashes and
            underscores are recommended.
          </DialogDescription>
        </DialogHeader>
        <form onSubmit={handleSubmit} className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="new-project-id">Project ID</Label>
            <Input
              id="new-project-id"
              placeholder="my-project"
              value={projectId}
              onChange={(e) => setProjectId(e.target.value)}
              disabled={mutation.isPending}
              autoFocus
            />
            {error && <p className="text-destructive text-sm">{error}</p>}
          </div>
          {availableParents.length > 0 && (
            <div className="space-y-2">
              <Label>Parent project (optional)</Label>
              <Select value={parentId} onValueChange={setParentId} disabled={mutation.isPending}>
                <SelectTrigger>
                  <SelectValue placeholder="No parent (standalone project)" />
                </SelectTrigger>
                <SelectContent>
                  {availableParents.map((p) => (
                    <SelectItem key={p.project_id} value={p.project_id}>
                      {p.project_id}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          )}
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button type="submit" disabled={mutation.isPending}>
              {mutation.isPending && <Loader2 className="animate-spin" />}
              Create
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
