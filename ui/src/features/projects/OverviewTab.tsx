import { useState, useMemo } from 'react'
import { Link, useParams } from 'react-router'
import { useQuery, useMutation, useQueryClient, keepPreviousData } from '@tanstack/react-query'
import { RefreshCw, Clock } from 'lucide-react'
import { fetchReportHistory, deleteReport, fetchReportKnownFailures } from '@/api/reports'
import { extractErrorMessage } from '@/api/client'
import { invalidateProjectQueries, queryKeys } from '@/lib/query-keys'
import { useAuthStore, selectIsAdmin } from '@/store/auth'
import { formatDuration, calcPassRate } from '@/lib/utils'
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
import { ProjectStatCards } from './ProjectStatCards'
import { ReportHistoryTable } from './ReportHistoryTable'
import { ReportPagination } from './ReportPagination'

const PER_PAGE = 20

export function OverviewTab() {
  const { id: projectId } = useParams<{ id: string }>()
  const isAdmin = useAuthStore(selectIsAdmin)
  const queryClient = useQueryClient()
  const [deleteReportId, setDeleteReportId] = useState<string | null>(null)
  const [page, setPage] = useState(1)
  const [selectedBranch, setSelectedBranch] = useState<string | undefined>(undefined)
  const [selectedBuilds, setSelectedBuilds] = useState<Set<string>>(new Set())

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
    queryKey: queryKeys.reportHistory(projectId ?? '', page, selectedBranch),
    queryFn: () => fetchReportHistory(projectId ?? '', page, PER_PAGE, selectedBranch),
    enabled: !!projectId,
    staleTime: 10_000,
    placeholderData: keepPreviousData,
  })

  const { data: knownFailuresData } = useQuery({
    queryKey: queryKeys.reportKnownFailures(projectId ?? ''),
    queryFn: () => fetchReportKnownFailures(projectId ?? ''),
    enabled: !!projectId,
    staleTime: 30_000,
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
  const { latest, tableReports, passRate, knownCount, adjustedPassRate } = useMemo(() => {
    const latest = reports.find((r) => r.is_latest)
    const tableReports = reports.filter((r) => r.report_id !== 'latest')
    const stat = latest?.statistic
    const passRate = stat ? calcPassRate(stat.passed, stat.total) : null
    const knownCount = knownFailuresData?.known_failures?.length ?? 0
    const adjustedPassRate =
      stat && knownCount > 0 ? calcPassRate(stat.passed + knownCount, stat.total) : null
    return { latest, tableReports, passRate, knownCount, adjustedPassRate }
  }, [reports, knownFailuresData])

  const pagination = historyData?.pagination
  const stat = latest?.statistic

  if (!projectId) return null

  return (
    <div className="space-y-6">
      {/* Page title */}
      <div>
        <h1 className="font-mono text-2xl font-semibold">{projectId}</h1>
        <p className="text-muted-foreground text-sm">Overview</p>
      </div>

      {/* Stat cards */}
      <ProjectStatCards
        isLoading={isLoading}
        stat={stat}
        passRate={passRate}
        adjustedPassRate={adjustedPassRate}
        knownCount={knownCount}
        latest={latest}
        pagination={pagination}
      />

      {/* Environment & Categories & Flaky Tests — G1/G2/A1 */}
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-3 [&:empty]:hidden">
        <EnvironmentCard projectId={projectId} />
        <CategoriesCard projectId={projectId} />
        <FlakyTestsCard projectId={projectId} />
      </div>

      {/* Compare Selected bar */}
      {selectedBuilds.size === 2 &&
        (() => {
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
        })()}

      {/* Branch filter */}
      <BranchSelector
        projectId={projectId}
        selectedBranch={selectedBranch}
        onBranchChange={setSelectedBranch}
      />

      {/* Report history table */}
      {isLoading ? (
        <div className="space-y-2">
          {Array.from({ length: 4 }).map((_, i) => (
            <Skeleton key={i} className="h-12 w-full" />
          ))}
        </div>
      ) : tableReports.length === 0 ? (
        <div className="flex flex-col items-center gap-3 rounded-lg border border-dashed py-16 text-center">
          <RefreshCw size={36} className="text-muted-foreground/40" />
          <div>
            <p className="font-medium">No reports yet</p>
            <p className="text-muted-foreground text-sm">
              {isAdmin
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
      {pagination && pagination.total_pages > 1 && (
        <ReportPagination page={page} totalPages={pagination.total_pages} onPageChange={setPage} />
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
              <span className="font-mono font-medium">{projectId}</span>. This cannot be undone.
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
