import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Loader2 } from 'lucide-react'
import { createProject } from '@/api/projects'
import { extractErrorMessage } from '@/api/client'
import { queryKeys } from '@/lib/query-keys'
import { projectListOptions, projectParentsOptions } from '@/lib/queries'
import type { PaginatedResponse, ProjectsData } from '@/types/api'
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

  const { data: projectsResp } = useQuery({ ...projectParentsOptions(), enabled: open })

  // Only top-level projects (no parent_id) can be parents
  const availableParents = (projectsResp?.data ?? []).filter((p) => !p.parent_id)

  const mutation = useMutation({
    mutationFn: createProject,
    onMutate: async (newProject) => {
      // Cancel outgoing refetches so they don't overwrite the optimistic update
      await queryClient.cancelQueries({ queryKey: queryKeys.projects })
      // Snapshot previous value for rollback
      const previous = queryClient.getQueryData<PaginatedResponse<ProjectsData>>(
        projectListOptions().queryKey,
      )
      // Optimistically add the new project to the list
      if (previous) {
        queryClient.setQueryData<PaginatedResponse<ProjectsData>>(
          projectListOptions().queryKey,
          {
            ...previous,
            data: [
              ...previous.data,
              {
                project_id: newProject.id,
                ...(newProject.parent_id ? { parent_id: newProject.parent_id } : {}),
              },
            ],
          },
        )
      }
      return { previous }
    },
    onError: (err, _newProject, context) => {
      // Rollback to previous state on failure
      if (context?.previous) {
        queryClient.setQueryData(projectListOptions().queryKey, context.previous)
      }
      setError(extractErrorMessage(err))
    },
    onSettled: () => {
      // Always refetch after error or success for server-authoritative data
      void queryClient.invalidateQueries({ queryKey: queryKeys.projects })
      void queryClient.invalidateQueries({ queryKey: queryKeys.dashboard() })
    },
    onSuccess: () => {
      toast({ title: 'Project created', description: `"${projectId}" is ready.` })
      setProjectId('')
      setParentId('')
      onOpenChange(false)
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
