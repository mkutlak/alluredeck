import { useState, useMemo } from 'react'
import { Link, useParams } from 'react-router'
import { useQuery, useMutation, useQueryClient, keepPreviousData } from '@tanstack/react-query'
import { RefreshCw, Clock } from 'lucide-react'
import { fetchReportHistory, deleteReport } from '@/api/reports'
import { fetchBranches } from '@/api/branches'
import { extractErrorMessage } from '@/api/client'
import { invalidateProjectQueries, queryKeys } from '@/lib/query-keys'
import { projectIndexOptions } from '@/lib/queries'
import { useAuthStore, selectIsAdmin, selectIsEditor } from '@/store/auth'
import { useUIStore } from '@/store/ui'
import { formatDuration, calcPassRate, formatPassRate } from '@/lib/utils'
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
import { FilterBar } from '@/components/app/FilterBar'
import { BranchSelect } from '@/components/app/BranchSelect'
import { EnvironmentCard } from '@/features/projects/EnvironmentCard'
import { CategoriesCard } from '@/features/projects/CategoriesCard'
import { FlakyTestsCard } from '@/features/projects/FlakyTestsCard'
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
  const [selectedBuilds, setSelectedBuilds] = useState<Set<string>>(new Set())
  const reportsPerPage = useUIStore((s) => s.reportsPerPage)
  const setReportsPerPage = useUIStore((s) => s.setReportsPerPage)

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
  const { data: projectsResp } = useQuery({ ...projectIndexOptions(), enabled: !!projectId })
  const allProjects = projectsResp?.data ?? []
  const currentProject = resolveProjectFromParam(projectId, allProjects)
  const isParentProject = (currentProject?.children?.length ?? 0) > 0

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

  // Reset to page 1 when branch filter changes
  const [prevBranch, setPrevBranch] = useState(selectedBranch)
  if (prevBranch !== selectedBranch) {
    setPrevBranch(selectedBranch)
    setPage(1)
  }

  const { data: historyData, isLoading } = useQuery({
    queryKey: queryKeys.reportHistory(projectId ?? '', page, effectiveBranch, reportsPerPage),
    queryFn: () => fetchReportHistory(projectId ?? '', page, reportsPerPage, effectiveBranch),
    enabled: !!projectId,
    staleTime: 10_000,
    placeholderData: keepPreviousData,
  })

  // Header stat chips must reflect the branch filter, so derive them from the
  // newest branch-filtered NUMBERED build (1-entry query) instead of the
  // "latest" alias, which the API prepends unfiltered by branch.
  const { data: chipHistoryData } = useQuery({
    queryKey: queryKeys.reportHistory(projectId ?? '', 1, effectiveBranch, 1),
    queryFn: () => fetchReportHistory(projectId ?? '', 1, 1, effectiveBranch),
    enabled: !!projectId,
    staleTime: 10_000,
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
  const { latest, tableReports } = useMemo(() => {
    const latest = reports.find((r) => r.is_latest)
    const tableReports = reports.filter((r) => r.report_id !== 'latest')
    return { latest, tableReports }
  }, [reports])

  // Header chips: newest branch-filtered numbered build (ignore the synthetic
  // "latest" alias, which is not branch-filtered by the API).
  const { chipLatest, passRate } = useMemo(() => {
    const chipReports = chipHistoryData?.data.reports ?? []
    const chipLatest = chipReports.find((r) => r.report_id !== 'latest')
    const stat = chipLatest?.statistic
    const passRate = stat ? calcPassRate(stat.passed, stat.total, stat.skipped) : null
    return { chipLatest, passRate }
  }, [chipHistoryData])

  const pagination = historyData?.pagination
  const stat = chipLatest?.statistic

  if (!projectId) return null

  // Parent project view: show pipeline runs grouped by commit SHA
  if (isParentProject) {
    return (
      <PipelineRunsTab
        projectId={projectId}
        childIds={(currentProject?.children ?? []).map(String)}
      />
    )
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
      {/* Stat chips */}
      {stat && passRate != null ? (
        <div className="flex flex-wrap items-center gap-1.5">
          <Badge
            variant={passRate >= 90 ? 'default' : passRate >= 70 ? 'secondary' : 'destructive'}
            className={getPassRateBadgeClass(passRate)}
          >
            Pass rate: {formatPassRate(stat.passed, stat.total, stat.skipped)}
          </Badge>
          <Badge variant="outline">Tests: {stat.total}</Badge>
          {stat.failed + stat.broken > 0 && (
            <Badge variant="destructive">Failed: {stat.failed + stat.broken}</Badge>
          )}
          {chipLatest?.duration_ms != null && (
            <Badge variant="outline">Last duration: {formatDuration(chipLatest.duration_ms)}</Badge>
          )}
          {chipLatest?.generated_at && (
            <Badge variant="outline">
              Last run:{' '}
              {new Date(chipLatest.generated_at).toLocaleDateString('en-US', {
                month: 'short',
                day: '2-digit',
              })}
            </Badge>
          )}
          {effectiveBranch && <Badge variant="outline">Branch: {effectiveBranch}</Badge>}
        </div>
      ) : !isLoading ? (
        <div>
          <Badge variant="secondary">No builds</Badge>
        </div>
      ) : null}

      {/* Environment & Categories & Flaky Tests — G1/G2/A1 */}
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-3 [&:empty]:hidden">
        <EnvironmentCard projectId={projectId} />
        <CategoriesCard projectId={projectId} />
        <FlakyTestsCard projectId={projectId} numericProjectId={currentProject?.project_id} />
      </div>

      {/* Compare Selected bar */}
      {compareBarContent}

      {/* Filter row */}
      <FilterBar filters={<BranchSelect />} />

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
              <span className="font-mono font-medium">
                {formatProjectLabel(currentProject, allProjects)}
              </span>
              . This cannot be undone.
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
