import { useState, useMemo } from 'react'
import { Link, NavLink, useParams } from 'react-router'
import { useQuery, useMutation, useQueryClient, keepPreviousData } from '@tanstack/react-query'
import { RefreshCw, Clock, GitCommitHorizontal, GitBranch } from 'lucide-react'
import { fetchReportHistory, deleteReport } from '@/api/reports'
import { fetchBranches } from '@/api/branches'
import { extractErrorMessage } from '@/api/client'
import { invalidateProjectQueries, queryKeys } from '@/lib/query-keys'
import { projectListOptions } from '@/lib/queries'
import { useAuthStore, selectIsAdmin, selectIsEditor } from '@/store/auth'
import { useUIStore } from '@/store/ui'
import { formatDuration } from '@/lib/utils'
import { Button } from '@/components/ui/button'
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
} from '@/components/ui/alert-dialog'
import { toast } from '@/components/ui/use-toast'
import { EnvironmentCard } from '@/features/projects/EnvironmentCard'
import { CategoriesCard } from '@/features/projects/CategoriesCard'
import { FlakyTestsCard } from '@/features/projects/FlakyTestsCard'
import { BranchSelector } from '@/features/projects/BranchSelector'
import { ActionBar } from '@/components/app/ActionBar'
import { PipelineRunsTab } from '@/features/pipeline'
import { Badge } from '@/components/ui/badge'
import { getPassRateBadgeClass } from '@/lib/status-colors'
import { formatProjectLabel } from '@/lib/projectLabel'
import { resolveProjectFromParam } from '@/lib/resolveProject'
import { ReportHistoryTable } from './ReportHistoryTable'
import { ReportPagination } from './ReportPagination'

export function OverviewTab() {
  const { id: projectId } = useParams<{ id: string }>()
  const isAdmin = useAuthStore(selectIsAdmin)
  const isEditor = useAuthStore(selectIsEditor)
  const queryClient = useQueryClient()
  const [deleteReportId, setDeleteReportId] = useState<string | null>(null)
  const [page, setPage] = useState(1)
  const selectedBranch = useUIStore((s) => s.selectedBranch)
  const setSelectedBranch = useUIStore((s) => s.setSelectedBranch)
  const [selectedBuilds, setSelectedBuilds] = useState<Set<string>>(new Set())
  const reportsPerPage = useUIStore((s) => s.reportsPerPage)
  const setReportsPerPage = useUIStore((s) => s.setReportsPerPage)
  const groupBy = useUIStore((s) => s.reportsGroupBy)
  const setGroupBy = useUIStore((s) => s.setReportsGroupBy)

  const { data: branchesData } = useQuery({
    queryKey: queryKeys.branches.list(projectId ?? ''),
    queryFn: () => fetchBranches(projectId ?? ''),
    enabled: !!projectId,
    staleTime: 60_000,
  })
  const effectiveBranch =
    selectedBranch && branchesData?.some((b) => b.name === selectedBranch)
      ? selectedBranch
      : undefined

  // Hierarchy detection: fetch the project list to find parent/child relationships
  const { data: projectsResp } = useQuery({ ...projectListOptions(), enabled: !!projectId })
  const allProjects = projectsResp?.data ?? []
  const currentProject = resolveProjectFromParam(projectId, allProjects)
  const isParentProject = (currentProject?.children?.length ?? 0) > 0
  const parentProject = currentProject?.parent_id
    ? allProjects.find((p) => p.project_id === currentProject.parent_id)
    : null

  const handleToggleBuild = (id: string) => {
    setSelectedBuilds((prev) => {
      const next = new Set(prev)
      if (next.has(id)) {
        next.delete(id)
      } else if (next.size < 2) {
        next.add(id)
      }
      return next
    })
  }
  const [prevProjectId, setPrevProjectId] = useState(projectId)
  if (prevProjectId !== projectId) {
    setPrevProjectId(projectId)
    setPage(1)
  }

  const { data: historyData, isLoading } = useQuery({
    queryKey: queryKeys.reportHistory(projectId ?? '', page, effectiveBranch, reportsPerPage),
    queryFn: () => fetchReportHistory(projectId ?? '', page, reportsPerPage, effectiveBranch),
    enabled: !!projectId,
    staleTime: 10_000,
    placeholderData: keepPreviousData,
  })

  const deleteMutation = useMutation({
    mutationFn: (reportId: string) => deleteReport(projectId ?? '', reportId),
    onSuccess: (_, reportId) => {
      void invalidateProjectQueries(queryClient, projectId ?? '')
      toast({ title: 'Report deleted', description: `Report #${reportId} has been removed.` })
      setDeleteReportId(null)
    },
    onError: (err) => {
      toast({
        title: 'Delete failed',
        description: extractErrorMessage(err),
        variant: 'destructive',
      })
      setDeleteReportId(null)
    },
  })

  // Memoize derived data. Safe to compute before the projectId guard because
  // historyData and knownFailuresData are undefined until queries are enabled.
  const reports = useMemo(() => historyData?.data.reports ?? [], [historyData])
  const { latest, tableReports, passRate } = useMemo(() => {
    const latest = reports.find((r) => r.is_latest)
    const tableReports = reports.filter((r) => r.report_id !== 'latest')
    const stat = latest?.statistic
    const passRate = stat && stat.total > 0 ? Math.round((stat.passed / stat.total) * 100) : null
    return { latest, tableReports, passRate }
  }, [reports])

  const pagination = historyData?.pagination
  const stat = latest?.statistic

  if (!projectId) return null

  // Parent project view: show pipeline runs grouped by commit SHA
  if (isParentProject) {
    return <PipelineRunsTab projectId={projectId} childIds={(currentProject?.children ?? []).map(String)} />
  }

  const compareBarContent =
    selectedBuilds.size === 2
      ? (() => {
          const [a, b] = Array.from(selectedBuilds)
          const compareUrl = `/projects/${encodeURIComponent(projectId)}/compare?a=${a}&b=${b}`
          return (
            <div className="bg-muted/40 flex items-center gap-3 rounded-lg border px-4 py-2">
              <span className="text-muted-foreground text-sm">2 builds selected</span>
              <Button asChild size="sm">
                <Link to={compareUrl}>Compare Selected</Link>
              </Button>
              <Button size="sm" variant="ghost" onClick={() => setSelectedBuilds(new Set())}>
                Clear
              </Button>
            </div>
          )
        })()
      : null

  return (
    <div className="space-y-6" data-testid="project-overview">
      {/* Page title + action buttons */}
      <div>
        <div className="flex items-center justify-between gap-4">
          <h1 className="font-mono text-2xl font-semibold">{formatProjectLabel(currentProject, allProjects)}</h1>
          <ActionBar />
        </div>
        {parentProject ? (
          <p className="text-muted-foreground flex items-center gap-1 text-sm">
            Part of:{' '}
            <NavLink
              to={`/projects/${parentProject.project_id}`}
              className="text-primary hover:underline"
            >
              {parentProject.slug}
            </NavLink>
          </p>
        ) : (
          <p className="text-muted-foreground text-sm">Overview</p>
        )}
        {stat && passRate != null ? (
          <div className="mt-2 flex flex-wrap items-center gap-1.5">
            <Badge
              variant={passRate >= 90 ? 'default' : passRate >= 70 ? 'secondary' : 'destructive'}
              className={getPassRateBadgeClass(passRate)}
            >
              Pass rate: {passRate.toFixed(0)}%
            </Badge>
            <Badge variant="outline">Tests: {stat.total}</Badge>
            {stat.failed + stat.broken > 0 && (
              <Badge variant="destructive">Failed: {stat.failed + stat.broken}</Badge>
            )}
            {latest?.duration_ms != null && (
              <Badge variant="outline">Last duration: {formatDuration(latest.duration_ms)}</Badge>
            )}
            {latest?.generated_at && (
              <Badge variant="outline">
                Last run:{' '}
                {new Date(latest.generated_at).toLocaleDateString('en-US', {
                  month: 'short',
                  day: '2-digit',
                })}
              </Badge>
            )}
          </div>
        ) : !isLoading ? (
          <div className="mt-2">
            <Badge variant="secondary">No builds</Badge>
          </div>
        ) : null}
      </div>

      {/* Environment & Categories & Flaky Tests — G1/G2/A1 */}
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-3 [&:empty]:hidden">
        <EnvironmentCard projectId={projectId} />
        <CategoriesCard projectId={projectId} />
        <FlakyTestsCard projectId={projectId} />
      </div>

      {/* Compare Selected bar */}
      {compareBarContent}

      {/* Branch filter + Group by toolbar */}
      <div className="flex items-center gap-3">
        <div className="flex items-center gap-2">
          <span className="text-muted-foreground text-xs">Choose branch:</span>
          <BranchSelector
            projectId={projectId}
            selectedBranch={selectedBranch}
            onBranchChange={setSelectedBranch}
          />
        </div>
        <div className="ml-auto flex items-center gap-2">
          <span className="text-muted-foreground text-xs">Group by:</span>
          <div className="flex gap-1">
            <Button
              size="sm"
              variant={groupBy === 'none' ? 'secondary' : 'outline'}
              onClick={() => setGroupBy('none')}
            >
              None
            </Button>
            <Button
              size="sm"
              variant={groupBy === 'commit' ? 'secondary' : 'outline'}
              onClick={() => setGroupBy('commit')}
            >
              <GitCommitHorizontal size={12} />
              Commit
            </Button>
            <Button
              size="sm"
              variant={groupBy === 'branch' ? 'secondary' : 'outline'}
              onClick={() => setGroupBy('branch')}
            >
              <GitBranch size={12} />
              Branch
            </Button>
          </div>
        </div>
      </div>

      {/* Report history table */}
      {isLoading ? (
        <div className="space-y-2">
          <Skeleton className="h-12 w-full" />
          <Skeleton className="h-12 w-full" />
          <Skeleton className="h-12 w-full" />
          <Skeleton className="h-12 w-full" />
        </div>
      ) : tableReports.length === 0 ? (
        <div className="flex flex-col items-center gap-3 rounded-lg border border-dashed py-16 text-center">
          <RefreshCw size={36} className="text-muted-foreground/40" />
          <div>
            <p className="font-medium">No reports yet</p>
            <p className="text-muted-foreground text-sm">
              {isEditor
                ? 'Send results and generate a report to get started.'
                : 'No reports available for this project.'}
            </p>
          </div>
        </div>
      ) : (
        <ReportHistoryTable
          projectId={projectId}
          reports={tableReports}
          isAdmin={isAdmin}
          onDeleteReport={setDeleteReportId}
          selectedBuilds={selectedBuilds}
          onToggleBuild={handleToggleBuild}
          groupBy={groupBy}
        />
      )}

      {/* Pagination controls */}
      {tableReports.length > 0 && (
        <ReportPagination
          page={page}
          totalPages={Math.max(1, pagination?.total_pages ?? 1)}
          onPageChange={setPage}
          perPage={reportsPerPage}
          onPerPageChange={(n) => {
            setReportsPerPage(n)
            setPage(1)
          }}
        />
      )}

      {/* Duration summary */}
      {latest?.duration_ms && (
        <p className="text-muted-foreground flex items-center gap-1 text-xs">
          <Clock size={12} />
          Latest suite duration:{' '}
          <span className="font-mono">{formatDuration(latest.duration_ms)}</span>
        </p>
      )}

      {/* Delete confirmation */}
      <AlertDialog
        open={deleteReportId !== null}
        onOpenChange={(open) => {
          if (!open) setDeleteReportId(null)
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete report #{deleteReportId}?</AlertDialogTitle>
            <AlertDialogDescription>
              This will permanently delete report{' '}
              <span className="font-mono font-medium">#{deleteReportId}</span> for project{' '}
              <span className="font-mono font-medium">{formatProjectLabel(currentProject, allProjects)}</span>. This cannot be undone.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
              onClick={() => deleteReportId && deleteMutation.mutate(deleteReportId)}
            >
              Delete
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
