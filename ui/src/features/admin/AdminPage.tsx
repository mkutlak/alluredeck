import { useState } from 'react'
import { Navigate, Link } from 'react-router'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useAuthStore } from '@/store/auth'
import { queryKeys } from '@/lib/query-keys'
import { formatDate, formatBytes } from '@/lib/utils'
import { fetchAdminJobs, fetchAdminResults, cancelJob, cleanAdminResults, deleteJob } from '@/api/admin'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Skeleton } from '@/components/ui/skeleton'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from '@/components/ui/alert-dialog'
import type { AdminJobStatus } from '@/types/api'

function jobStatusVariant(
  status: AdminJobStatus,
): 'default' | 'secondary' | 'destructive' | 'outline' {
  switch (status) {
    case 'running':
      return 'default'
    case 'completed':
      return 'secondary'
    case 'failed':
      return 'destructive'
    case 'cancelled':
      return 'outline'
    default:
      return 'outline'
  }
}

function isTerminalStatus(status: AdminJobStatus): boolean {
  return status === 'completed' || status === 'failed' || status === 'cancelled'
}

function JobsCard() {
  const queryClient = useQueryClient()
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set())
  const [confirmDeleteOpen, setConfirmDeleteOpen] = useState(false)

  const { data: jobs = [], isLoading } = useQuery({
    queryKey: queryKeys.adminJobs,
    queryFn: fetchAdminJobs,
    refetchInterval: 5_000,
  })

  const { mutate: doCancel } = useMutation({
    mutationFn: cancelJob,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.adminJobs })
    },
  })

  const { mutate: doDeleteSelected } = useMutation({
    mutationFn: async (jobIds: string[]) => {
      await Promise.all(jobIds.map((id) => deleteJob(id)))
    },
    onSuccess: () => {
      setSelectedIds(new Set())
      setConfirmDeleteOpen(false)
      void queryClient.invalidateQueries({ queryKey: queryKeys.adminJobs })
    },
  })

  const terminalJobs = jobs.filter((j) => isTerminalStatus(j.status))
  const allSelected = terminalJobs.length > 0 && terminalJobs.every((j) => selectedIds.has(j.job_id))
  const someSelected = terminalJobs.some((j) => selectedIds.has(j.job_id))

  const toggleSelectAll = () => {
    if (allSelected) {
      setSelectedIds(new Set())
    } else {
      setSelectedIds(new Set(terminalJobs.map((j) => j.job_id)))
    }
  }

  const toggleJob = (jobId: string) => {
    setSelectedIds((prev) => {
      const next = new Set(prev)
      if (next.has(jobId)) {
        next.delete(jobId)
      } else {
        next.add(jobId)
      }
      return next
    })
  }

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center justify-between">
          <div>
            <CardTitle>Jobs</CardTitle>
            <CardDescription>Active and recent report generation jobs</CardDescription>
          </div>
          {someSelected && (
            <AlertDialog open={confirmDeleteOpen} onOpenChange={setConfirmDeleteOpen}>
              <AlertDialogTrigger asChild>
                <Button size="sm" variant="destructive">
                  Delete selected ({selectedIds.size})
                </Button>
              </AlertDialogTrigger>
              <AlertDialogContent>
                <AlertDialogHeader>
                  <AlertDialogTitle>Delete {selectedIds.size} job{selectedIds.size !== 1 ? 's' : ''}?</AlertDialogTitle>
                  <AlertDialogDescription>
                    This will permanently remove the selected job records. This action cannot be undone.
                  </AlertDialogDescription>
                </AlertDialogHeader>
                <AlertDialogFooter>
                  <AlertDialogCancel>Cancel</AlertDialogCancel>
                  <AlertDialogAction onClick={() => doDeleteSelected(Array.from(selectedIds))}>
                    Confirm
                  </AlertDialogAction>
                </AlertDialogFooter>
              </AlertDialogContent>
            </AlertDialog>
          )}
        </div>
      </CardHeader>
      <CardContent>
        {isLoading ? (
          <div className="space-y-2">
            <Skeleton className="h-8 w-full" />
            <Skeleton className="h-8 w-full" />
          </div>
        ) : jobs.length === 0 ? (
          <p className="py-4 text-center text-sm text-muted-foreground">No jobs in queue</p>
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="w-10">
                  <Checkbox
                    checked={allSelected ? true : someSelected ? 'indeterminate' : false}
                    onCheckedChange={toggleSelectAll}
                    aria-label="Select all terminal jobs"
                    disabled={terminalJobs.length === 0}
                  />
                </TableHead>
                <TableHead>Project</TableHead>
                <TableHead>Status</TableHead>
                <TableHead>Created</TableHead>
                <TableHead>Started</TableHead>
                <TableHead className="text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {jobs.map((job) => (
                <TableRow key={job.job_id}>
                  <TableCell>
                    {isTerminalStatus(job.status) ? (
                      <Checkbox
                        checked={selectedIds.has(job.job_id)}
                        onCheckedChange={() => toggleJob(job.job_id)}
                        aria-label={`Select job ${job.job_id}`}
                      />
                    ) : null}
                  </TableCell>
                  <TableCell>
                    <Link
                      to={`/projects/${job.project_id}`}
                      className="font-medium hover:underline"
                    >
                      {job.project_id}
                    </Link>
                  </TableCell>
                  <TableCell>
                    <Badge variant={jobStatusVariant(job.status)}>{job.status}</Badge>
                  </TableCell>
                  <TableCell className="text-sm text-muted-foreground">
                    {formatDate(job.created_at)}
                  </TableCell>
                  <TableCell className="text-sm text-muted-foreground">
                    {job.started_at ? formatDate(job.started_at) : '—'}
                  </TableCell>
                  <TableCell className="text-right">
                    {(job.status === 'pending' || job.status === 'running') && (
                      <Button
                        size="sm"
                        variant="outline"
                        onClick={() => doCancel(job.job_id)}
                      >
                        Cancel
                      </Button>
                    )}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </CardContent>
    </Card>
  )
}

function ResultsCard() {
  const queryClient = useQueryClient()
  const [deletingId, setDeletingId] = useState<string | null>(null)

  const { data: results = [], isLoading } = useQuery({
    queryKey: queryKeys.adminResults,
    queryFn: fetchAdminResults,
    refetchInterval: 30_000,
  })

  const { mutate: doClean } = useMutation({
    mutationFn: cleanAdminResults,
    onSuccess: () => {
      setDeletingId(null)
      void queryClient.invalidateQueries({ queryKey: queryKeys.adminResults })
    },
  })

  return (
    <Card>
      <CardHeader>
        <CardTitle>Pending Results</CardTitle>
        <CardDescription>Projects with unprocessed result files</CardDescription>
      </CardHeader>
      <CardContent>
        {isLoading ? (
          <div className="space-y-2">
            <Skeleton className="h-8 w-full" />
            <Skeleton className="h-8 w-full" />
          </div>
        ) : results.length === 0 ? (
          <p className="py-4 text-center text-sm text-muted-foreground">
            No unprocessed results
          </p>
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Project</TableHead>
                <TableHead>Files</TableHead>
                <TableHead>Total Size</TableHead>
                <TableHead>Last Modified</TableHead>
                <TableHead className="text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {results.map((entry) => (
                <TableRow key={entry.project_id}>
                  <TableCell>
                    <Link
                      to={`/projects/${entry.project_id}`}
                      className="font-medium hover:underline"
                    >
                      {entry.project_id}
                    </Link>
                  </TableCell>
                  <TableCell>{entry.file_count}</TableCell>
                  <TableCell className="text-sm text-muted-foreground">
                    {formatBytes(entry.total_size)}
                  </TableCell>
                  <TableCell className="text-sm text-muted-foreground">
                    {formatDate(entry.last_modified)}
                  </TableCell>
                  <TableCell className="text-right">
                    <AlertDialog
                      open={deletingId === entry.project_id}
                      onOpenChange={(open) => setDeletingId(open ? entry.project_id : null)}
                    >
                      <AlertDialogTrigger asChild>
                        <Button size="sm" variant="destructive">
                          Delete
                        </Button>
                      </AlertDialogTrigger>
                      <AlertDialogContent>
                        <AlertDialogHeader>
                          <AlertDialogTitle>Delete pending results?</AlertDialogTitle>
                          <AlertDialogDescription>
                            This will permanently delete all unprocessed result files for{' '}
                            <strong>{entry.project_id}</strong>. This action cannot be undone.
                          </AlertDialogDescription>
                        </AlertDialogHeader>
                        <AlertDialogFooter>
                          <AlertDialogCancel>Cancel</AlertDialogCancel>
                          <AlertDialogAction onClick={() => doClean(entry.project_id)}>
                            Confirm
                          </AlertDialogAction>
                        </AlertDialogFooter>
                      </AlertDialogContent>
                    </AlertDialog>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </CardContent>
    </Card>
  )
}

export function AdminPage() {
  const isAdmin = useAuthStore((s) => s.isAdmin())

  if (!isAdmin) {
    return <Navigate to="/" replace />
  }

  return (
    <div className="space-y-6 p-6">
      <h1 className="text-2xl font-bold">System Monitor</h1>
      <JobsCard />
      <ResultsCard />
    </div>
  )
}
