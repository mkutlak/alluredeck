import { useState } from 'react'
import { useParams } from 'react-router'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Plus, Pencil, Trash2, ExternalLink } from 'lucide-react'
import { listKnownIssues, deleteKnownIssue, updateKnownIssue } from '@/api/known-issues'
import { extractErrorMessage } from '@/api/client'
import { queryKeys } from '@/lib/query-keys'
import { isSafeUrl } from '@/lib/url'
import { useAuthStore, selectIsEditor } from '@/store/auth'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { CardState } from '@/components/ui/CardState'
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
import { useProjectDisplay } from '@/features/projects/useProjectDisplay'
import { PageHeader } from '@/components/app/PageHeader'
import { FilterBar } from '@/components/app/FilterBar'
import { CreateKnownIssueDialog } from './CreateKnownIssueDialog'
import { EditKnownIssueDialog } from './EditKnownIssueDialog'

export function KnownIssuesTab() {
  const { id: projectId } = useParams<{ id: string }>()
  const displayName = useProjectDisplay(projectId)
  const isEditor = useAuthStore(selectIsEditor)
  const queryClient = useQueryClient()

  const [showResolved, setShowResolved] = useState(false)
  const [createOpen, setCreateOpen] = useState(false)
  const [editIssue, setEditIssue] = useState<KnownIssue | null>(null)
  const [deleteIssueId, setDeleteIssueId] = useState<number | null>(null)

  const {
    data: issues,
    isLoading,
    isError,
    error,
    refetch,
  } = useQuery({
    queryKey: queryKeys.knownIssues(projectId!, showResolved),
    queryFn: () => listKnownIssues(projectId!, !showResolved),
    enabled: !!projectId,
    staleTime: 15_000,
  })

  const toggleMutation = useMutation({
    mutationFn: (issue: KnownIssue) =>
      updateKnownIssue(projectId!, issue.id, {
        ticket_url: issue.ticket_url,
        description: issue.description,
        is_active: !issue.is_active,
      }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.knownIssues(projectId!) })
      void queryClient.invalidateQueries({ queryKey: queryKeys.reportKnownFailures(projectId!) })
      toast({ title: 'Status updated' })
    },
    onError: (err) => {
      toast({
        title: 'Update failed',
        description: extractErrorMessage(err),
        variant: 'destructive',
      })
    },
  })

  const deleteMutation = useMutation({
    mutationFn: (issueId: number) => deleteKnownIssue(projectId!, issueId),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.knownIssues(projectId!) })
      void queryClient.invalidateQueries({ queryKey: queryKeys.reportKnownFailures(projectId!) })
      toast({ title: 'Known issue removed' })
      setDeleteIssueId(null)
    },
    onError: (err) => {
      toast({
        title: 'Delete failed',
        description: extractErrorMessage(err),
        variant: 'destructive',
      })
      setDeleteIssueId(null)
    },
  })

  if (!projectId) return null

  const filtered = issues ?? []

  return (
    <div className="space-y-4">
      <PageHeader
        title={displayName}
        subtitle="Known Issues"
        actions={
          isEditor && (
            <Button size="sm" onClick={() => setCreateOpen(true)}>
              <Plus size={14} />
              Add Known Issue
            </Button>
          )
        }
        toolbar={
          <FilterBar
            end={
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
            }
          />
        }
      />

      <CardState
        isLoading={isLoading}
        isError={isError}
        error={error}
        isEmpty={filtered.length === 0}
        refetch={refetch}
        skeletonRows={4}
        emptyMessage={
          isEditor
            ? 'No known issues tracked for this project — add known issues to separate them from new failures in reports.'
            : 'No known issues tracked for this project'
        }
      >
        <div className="rounded-lg border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Test Name</TableHead>
                <TableHead>Ticket</TableHead>
                <TableHead>Description</TableHead>
                <TableHead className="text-center">Status</TableHead>
                <TableHead>Created</TableHead>
                {isEditor && <TableHead className="text-right">Actions</TableHead>}
              </TableRow>
            </TableHeader>
            <TableBody>
              {filtered.map((issue) => (
                <TableRow key={issue.id}>
                  <TableCell className="font-mono text-sm">{issue.test_name}</TableCell>
                  <TableCell>
                    {issue.ticket_url && isSafeUrl(issue.ticket_url) ? (
                      <a
                        href={issue.ticket_url}
                        target="_blank"
                        rel="noopener noreferrer"
                        className="text-primary flex items-center gap-1 text-sm hover:underline"
                      >
                        <ExternalLink size={12} />
                        {issue.ticket_url.replace(/^https?:\/\//, '')}
                      </a>
                    ) : (
                      <span className="text-muted-foreground text-sm">—</span>
                    )}
                  </TableCell>
                  <TableCell className="text-muted-foreground max-w-xs truncate text-sm">
                    {issue.description || '—'}
                  </TableCell>
                  <TableCell className="text-center">
                    {isEditor ? (
                      <Button
                        size="sm"
                        variant="ghost"
                        aria-label="Toggle issue status"
                        onClick={() => toggleMutation.mutate(issue)}
                        disabled={toggleMutation.isPending}
                      >
                        <Badge variant={issue.is_active ? 'default' : 'secondary'}>
                          {issue.is_active ? 'active' : 'resolved'}
                        </Badge>
                      </Button>
                    ) : (
                      <Badge variant={issue.is_active ? 'default' : 'secondary'}>
                        {issue.is_active ? 'active' : 'resolved'}
                      </Badge>
                    )}
                  </TableCell>
                  <TableCell className="text-muted-foreground text-sm">
                    {formatDate(issue.created_at)}
                  </TableCell>
                  {isEditor && (
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
      </CardState>

      {isEditor && (
        <>
          <CreateKnownIssueDialog
            projectId={projectId}
            open={createOpen}
            onOpenChange={setCreateOpen}
          />
          {editIssue && (
            <EditKnownIssueDialog
              key={editIssue.id}
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
