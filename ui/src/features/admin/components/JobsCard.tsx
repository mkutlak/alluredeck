import { useState } from 'react'
import { Link } from 'react-router'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { projectIndexOptions } from '@/lib/queries/projects'
import { formatProjectLabel } from '@/lib/projectLabel'
import { queryKeys } from '@/lib/query-keys'
import { formatDate } from '@/lib/utils'
import { deleteJob } from '@/api/admin'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
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
import { ReportPagination } from '@/features/projects/ReportPagination'
import { useAdminJobs } from '../hooks/useAdminJobs'
import { isTerminalStatus, jobStatusVariant } from './jobStatus'
import { DeleteJobsDialog } from './DeleteJobsDialog'

export function JobsCard() {
  const queryClient = useQueryClient()
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set())
  const [confirmDeleteOpen, setConfirmDeleteOpen] = useState(false)
  const [page, setPage] = useState(1)
  const [perPage, setPerPage] = useState(20)

  const { data, isLoading, doCancel } = useAdminJobs(page, perPage)
  const jobs = data?.data ?? []
  const pagination = data?.pagination
  const totalPages = Math.max(1, pagination?.total_pages ?? 1)

  const { data: projectsData } = useQuery(projectIndexOptions())
  const projects = projectsData?.data

  const { mutate: doDeleteSelected } = useMutation({
    mutationFn: async (jobIds: string[]) => {
      const BATCH_SIZE = 5
      for (let i = 0; i < jobIds.length; i += BATCH_SIZE) {
        await Promise.all(jobIds.slice(i, i + BATCH_SIZE).map((id) => deleteJob(id)))
      }
    },
    onSuccess: () => {
      setSelectedIds(new Set())
      setConfirmDeleteOpen(false)
      void queryClient.invalidateQueries({ queryKey: queryKeys.adminJobs() })
    },
  })

  const terminalJobs = jobs.filter((j) => isTerminalStatus(j.status))
  const allSelected =
    terminalJobs.length > 0 && terminalJobs.every((j) => selectedIds.has(j.job_id))
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

  const renderJobRow = (job: (typeof jobs)[number]) => (
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
          {(() => {
            const matched = projects?.find((p) => p.project_id === job.project_id)
            if (!matched) return job.slug || '(unknown)'
            return formatProjectLabel(matched, projects)
          })()}
        </Link>
      </TableCell>
      <TableCell>
        <Badge variant={jobStatusVariant(job.status)}>{job.status}</Badge>
        {job.error && (job.status === 'retrying' || job.status === 'failed') && (
          <p className="text-destructive mt-1 max-w-xs truncate text-xs" title={job.error}>
            {job.error}
          </p>
        )}
      </TableCell>
      <TableCell className="text-muted-foreground text-sm">
        {formatDate(job.created_at)}
      </TableCell>
      <TableCell className="text-muted-foreground text-sm">
        {job.started_at ? formatDate(job.started_at) : '—'}
      </TableCell>
      <TableCell className="text-right">
        {(job.status === 'pending' || job.status === 'running' || job.status === 'retrying') && (
          <Button size="sm" variant="outline" onClick={() => doCancel(job.job_id)}>
            Cancel
          </Button>
        )}
      </TableCell>
    </TableRow>
  )

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center justify-between">
          <div>
            <CardTitle>Jobs</CardTitle>
            <CardDescription>Active and recent report generation jobs</CardDescription>
          </div>
          {someSelected && (
            <DeleteJobsDialog
              open={confirmDeleteOpen}
              onOpenChange={setConfirmDeleteOpen}
              count={selectedIds.size}
              onConfirm={() => doDeleteSelected(Array.from(selectedIds))}
            />
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
          <p className="text-muted-foreground py-4 text-center text-sm">No jobs in queue</p>
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
              {jobs.map((job) => renderJobRow(job))}
            </TableBody>
          </Table>
        )}
        {pagination && pagination.total_pages > 1 && (
          <div className="mt-4">
            <ReportPagination
              page={page}
              totalPages={totalPages}
              onPageChange={(updater) => setPage(updater)}
              perPage={perPage}
              onPerPageChange={(v) => {
                setPerPage(v)
                setPage(1)
              }}
            />
          </div>
        )}
      </CardContent>
    </Card>
  )
}
