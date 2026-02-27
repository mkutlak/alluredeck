import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { createKnownIssue } from '@/api/known-issues'
import { extractErrorMessage } from '@/api/client'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { toast } from '@/components/ui/use-toast'

interface Props {
  projectId: string
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function CreateKnownIssueDialog({ projectId, open, onOpenChange }: Props) {
  const queryClient = useQueryClient()
  const [testName, setTestName] = useState('')
  const [ticketUrl, setTicketUrl] = useState('')
  const [description, setDescription] = useState('')
  const [nameError, setNameError] = useState('')

  const mutation = useMutation({
    mutationFn: () =>
      createKnownIssue(projectId, {
        test_name: testName,
        ticket_url: ticketUrl || undefined,
        description: description || undefined,
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['known-issues', projectId] })
      toast({ title: 'Known issue added' })
      setTestName('')
      setTicketUrl('')
      setDescription('')
      setNameError('')
      onOpenChange(false)
    },
    onError: (err) => {
      toast({ title: 'Failed to create', description: extractErrorMessage(err), variant: 'destructive' })
    },
  })

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (!testName.trim()) {
      setNameError('Test name is required')
      return
    }
    setNameError('')
    mutation.mutate()
  }

  function handleOpenChange(nextOpen: boolean) {
    if (!nextOpen) {
      setTestName('')
      setTicketUrl('')
      setDescription('')
      setNameError('')
    }
    onOpenChange(nextOpen)
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Add Known Issue</DialogTitle>
          <DialogDescription>
            Track a test that is known to fail so it can be excluded from new failure alerts.
          </DialogDescription>
        </DialogHeader>
        <form onSubmit={handleSubmit} className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="test_name">Test Name *</Label>
            <Input
              id="test_name"
              placeholder="Exact test name as it appears in the report"
              value={testName}
              onChange={(e) => setTestName(e.target.value)}
            />
            {nameError && <p className="text-xs text-destructive">{nameError}</p>}
          </div>
          <div className="space-y-2">
            <Label htmlFor="ticket_url">Ticket URL</Label>
            <Input
              id="ticket_url"
              placeholder="https://jira.example.com/PROJ-123"
              value={ticketUrl}
              onChange={(e) => setTicketUrl(e.target.value)}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="description">Description</Label>
            <Input
              id="description"
              placeholder="Why is this test failing?"
              value={description}
              onChange={(e) => setDescription(e.target.value)}
            />
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => handleOpenChange(false)}>
              Cancel
            </Button>
            <Button type="submit" disabled={mutation.isPending}>
              {mutation.isPending ? 'Adding…' : 'Add'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
