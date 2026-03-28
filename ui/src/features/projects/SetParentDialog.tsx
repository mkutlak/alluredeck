import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { setProjectParent } from '@/api/projects'
import { extractErrorMessage } from '@/api/client'
import { queryKeys } from '@/lib/query-keys'
import { projectParentsOptions } from '@/lib/queries'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

interface Props {
  projectId: string
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function SetParentDialog({ projectId, open, onOpenChange }: Props) {
  const [selectedParent, setSelectedParent] = useState<string>('')
  const [error, setError] = useState<string | null>(null)
  const qc = useQueryClient()

  const { data: projectsResp } = useQuery({ ...projectParentsOptions(), enabled: open })

  // Filter: exclude self, exclude projects that already have a parent (they can't be parents)
  const availableParents = (projectsResp?.data ?? []).filter(
    (p) => p.project_id !== projectId && !p.parent_id,
  )

  const { mutate, isPending } = useMutation({
    mutationFn: () => setProjectParent(projectId, selectedParent),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.projects })
      void qc.invalidateQueries({ queryKey: queryKeys.dashboard() })
      onOpenChange(false)
      setSelectedParent('')
    },
    onError: (e) => setError(extractErrorMessage(e)),
  })

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Move to group</DialogTitle>
          <DialogDescription>
            Move <span className="font-mono font-semibold">{projectId}</span> into a parent project
            group.
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-2">
          <Select
            value={selectedParent}
            onValueChange={(v) => {
              setSelectedParent(v)
              setError(null)
            }}
          >
            <SelectTrigger>
              <SelectValue placeholder="Select a parent project..." />
            </SelectTrigger>
            <SelectContent>
              {availableParents.map((p) => (
                <SelectItem key={p.project_id} value={p.project_id}>
                  {p.project_id}
                </SelectItem>
              ))}
              {availableParents.length === 0 && (
                <div className="text-muted-foreground px-2 py-1.5 text-sm">
                  No available parent projects
                </div>
              )}
            </SelectContent>
          </Select>
          {error && <p className="text-destructive text-sm">{error}</p>}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={isPending}>
            Cancel
          </Button>
          <Button onClick={() => mutate()} disabled={isPending || !selectedParent}>
            {isPending ? 'Moving...' : 'Move'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
