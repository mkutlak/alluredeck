import { useState } from 'react'
import { useParams } from 'react-router'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Plus, Pencil, Trash2, ExternalLink } from 'lucide-react'
import { listKnownIssues, deleteKnownIssue } from '@/api/known-issues'
import { extractErrorMessage } from '@/api/client'
import { useAuthStore } from '@/store/auth'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Skeleton } from '@/components/ui/skeleton'
import { Checkbox } from '@/components/ui/checkbox'
import { Label } from '@/components/ui/label'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { toast } from '@/components/ui/use-toast'
import { formatDate } from '@/lib/utils'
import type { KnownIssue } from '@/types/api'
import { CreateKnownIssueDialog } from './CreateKnownIssueDialog'
import { EditKnownIssueDialog } from './EditKnownIssueDialog'

export function KnownIssuesTab() {
  const { id: projectId } = useParams<{ id: string }>()
  const isAdmin = useAuthStore((s) => s.isAdmin)
  const queryClient = useQueryClient()

  const [showResolved, setShowResolved] = useState(false)
  const [createOpen, setCreateOpen] = useState(false)
  const [editIssue, setEditIssue] = useState<KnownIssue | null>(null)
  const [deleteIssueId, setDeleteIssueId] = useState<number | null>(null)

  const { data: issues, isLoading } = useQuery({
    queryKey: ['known-issues', projectId],
    queryFn: () => listKnownIssues(projectId!, false),
    enabled: !!projectId,
    staleTime: 15_000,
  })

  const deleteMutation = useMutation({
    mutationFn: (issueId: number) => deleteKnownIssue(projectId!, issueId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['known-issues', projectId] })
      toast({ title: 'Known issue removed' })
      setDeleteIssueId(null)
    },
    onError: (err) => {
      toast({ title: 'Delete failed', description: extractErrorMessage(err), variant: 'destructive' })
      setDeleteIssueId(null)
    },
  })

  if (!projectId) return null

  const filtered = (issues ?? []).filter((i) => showResolved || i.is_active)

  return (
    <div className="space-y-4">
      <div>
        <h1 className="font-mono text-2xl font-semibold">{projectId}</h1>
        <p className="text-sm text-muted-foreground">Known Issues</p>
      </div>

      <div className="flex items-center justify-between gap-4">
        <div className="flex items-center gap-2">
          <Checkbox
            id="show-resolved"
            checked={showResolved}
            onCheckedChange={(v) => setShowResolved(v === true)}
          />
          <Label htmlFor="show-resolved" className="cursor-pointer text-sm">
            Show resolved
          </Label>
        </div>
        {isAdmin() && (
          <Button size="sm" onClick={() => setCreateOpen(true)}>
            <Plus size={14} />
            Add Known Issue
          </Button>
        )}
      </div>

      {isLoading ? (
        <div className="space-y-2">
          {Array.from({ length: 4 }).map((_, i) => (
            <Skeleton key={i} className="h-12 w-full" />
          ))}
        </div>
      ) : filtered.length === 0 ? (
        <div className="flex flex-col items-center gap-3 rounded-lg border border-dashed py-16 text-center">
          <p className="font-medium">No known issues tracked for this project</p>
          {isAdmin() && (
            <p className="text-sm text-muted-foreground">
              Add known issues to separate them from new failures in reports.
            </p>
          )}
        </div>
      ) : (
        <div className="rounded-lg border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Test Name</TableHead>
                <TableHead>Ticket</TableHead>
                <TableHead>Description</TableHead>
                <TableHead className="text-center">Status</TableHead>
                <TableHead>Created</TableHead>
                {isAdmin() && <TableHead className="text-right">Actions</TableHead>}
              </TableRow>
            </TableHeader>
            <TableBody>
              {filtered.map((issue) => (
                <TableRow key={issue.id}>
                  <TableCell className="font-mono text-sm">{issue.test_name}</TableCell>
                  <TableCell>
                    {issue.ticket_url ? (
                      <a
                        href={issue.ticket_url}
                        target="_blank"
                        rel="noopener noreferrer"
                        className="flex items-center gap-1 text-sm text-primary hover:underline"
                      >
                        <ExternalLink size={12} />
                        {issue.ticket_url.replace(/^https?:\/\//, '')}
                      </a>
                    ) : (
                      <span className="text-sm text-muted-foreground">—</span>
                    )}
                  </TableCell>
                  <TableCell className="max-w-xs truncate text-sm text-muted-foreground">
                    {issue.description || '—'}
                  </TableCell>
                  <TableCell className="text-center">
                    <Badge variant={issue.is_active ? 'default' : 'secondary'}>
                      {issue.is_active ? 'active' : 'resolved'}
                    </Badge>
                  </TableCell>
                  <TableCell className="text-sm text-muted-foreground">
                    {formatDate(issue.created_at)}
                  </TableCell>
                  {isAdmin() && (
                    <TableCell className="text-right">
                      <div className="flex justify-end gap-1">
                        <Button
                          size="sm"
                          variant="ghost"
                          onClick={() => setEditIssue(issue)}
                          aria-label={`Edit known issue ${issue.test_name}`}
                        >
                          <Pencil size={12} />
                        </Button>
                        <Button
                          size="sm"
                          variant="ghost"
                          className="text-destructive hover:text-destructive"
                          onClick={() => setDeleteIssueId(issue.id)}
                          aria-label={`Delete known issue ${issue.test_name}`}
                        >
                          <Trash2 size={12} />
                        </Button>
                      </div>
                    </TableCell>
                  )}
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}

      {isAdmin() && (
        <>
          <CreateKnownIssueDialog
            projectId={projectId}
            open={createOpen}
            onOpenChange={setCreateOpen}
          />
          {editIssue && (
            <EditKnownIssueDialog
              projectId={projectId}
              issue={editIssue}
              open={!!editIssue}
              onOpenChange={(open) => {
                if (!open) setEditIssue(null)
              }}
            />
          )}
        </>
      )}

      <AlertDialog
        open={deleteIssueId !== null}
        onOpenChange={(open) => {
          if (!open) setDeleteIssueId(null)
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Remove known issue?</AlertDialogTitle>
            <AlertDialogDescription>
              This will permanently delete this known issue record. It cannot be undone.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
              onClick={() => deleteIssueId !== null && deleteMutation.mutate(deleteIssueId)}
            >
              Delete
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
