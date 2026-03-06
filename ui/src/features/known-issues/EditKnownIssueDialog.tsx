import { useState, useEffect } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { updateKnownIssue } from '@/api/known-issues'
import { extractErrorMessage } from '@/api/client'
import { queryKeys } from '@/lib/query-keys'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { toast } from '@/components/ui/use-toast'
import type { KnownIssue } from '@/types/api'

interface Props {
  projectId: string
  issue: KnownIssue
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function EditKnownIssueDialog({ projectId, issue, open, onOpenChange }: Props) {
  const queryClient = useQueryClient()
  const [ticketUrl, setTicketUrl] = useState(issue.ticket_url)
  const [description, setDescription] = useState(issue.description)
  const [isActive, setIsActive] = useState(issue.is_active)

  useEffect(() => {
    setTicketUrl(issue.ticket_url)
    setDescription(issue.description)
    setIsActive(issue.is_active)
  }, [issue])

  const mutation = useMutation({
    mutationFn: () =>
      updateKnownIssue(projectId, issue.id, {
        ticket_url: ticketUrl,
        description: description,
        is_active: isActive,
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['known-issues', projectId] })
      void queryClient.invalidateQueries({ queryKey: queryKeys.reportKnownFailures(projectId) })
      toast({ title: 'Known issue updated' })
      onOpenChange(false)
    },
    onError: (err) => {
      toast({ title: 'Update failed', description: extractErrorMessage(err), variant: 'destructive' })
    },
  })

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    mutation.mutate()
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Edit Known Issue</DialogTitle>
        </DialogHeader>
        <form onSubmit={handleSubmit} className="space-y-4">
          <div className="space-y-1">
            <Label className="text-xs text-muted-foreground">Test Name (read-only)</Label>
            <p className="font-mono text-sm">{issue.test_name}</p>
          </div>
          <div className="space-y-2">
            <Label htmlFor="edit_ticket_url">Ticket URL</Label>
            <Input
              id="edit_ticket_url"
              placeholder="https://jira.example.com/PROJ-123"
              value={ticketUrl}
              onChange={(e) => setTicketUrl(e.target.value)}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="edit_description">Description</Label>
            <Input
              id="edit_description"
              value={description}
              onChange={(e) => setDescription(e.target.value)}
            />
          </div>
          <div className="flex items-center gap-2">
            <Checkbox
              id="edit_is_active"
              checked={isActive}
              onCheckedChange={(v) => setIsActive(v === true)}
            />
            <Label htmlFor="edit_is_active" className="cursor-pointer">
              Active (uncheck to mark as resolved)
            </Label>
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button type="submit" disabled={mutation.isPending}>
              {mutation.isPending ? 'Saving…' : 'Save'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
