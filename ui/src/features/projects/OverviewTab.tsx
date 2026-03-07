import { useState, useMemo } from 'react'
import { Link, useParams } from 'react-router'
import { useQuery, useMutation, useQueryClient, keepPreviousData } from '@tanstack/react-query'
import {
  ExternalLink,
  Upload,
  Play,
  Trash2,
  CheckCircle2,
  XCircle,
  Clock,
  BarChart3,
  RefreshCw,
  GitBranch,
  ChevronLeft,
  ChevronRight,
} from 'lucide-react'
import { fetchReportHistory, deleteReport, fetchReportKnownFailures } from '@/api/reports'
import { extractErrorMessage } from '@/api/client'
import { invalidateProjectQueries, queryKeys } from '@/lib/query-keys'
import { useAuthStore } from '@/store/auth'
import { env } from '@/lib/env'
import { isSafeUrl } from '@/lib/url'
import { formatDate, formatDuration, calcPassRate } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Checkbox } from '@/components/ui/checkbox'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
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
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip'
import { toast } from '@/components/ui/use-toast'
import { Pagination, PaginationContent, PaginationItem } from '@/components/ui/pagination'
import { SendResultsDialog } from '@/features/reports/SendResultsDialog'
import { GenerateReportDialog } from '@/features/reports/GenerateReportDialog'
import { CleanDialog } from '@/features/reports/CleanDialog'
import { EnvironmentCard } from '@/features/projects/EnvironmentCard'
import { CategoriesCard } from '@/features/projects/CategoriesCard'
import { FlakyTestsCard } from '@/features/projects/FlakyTestsCard'
import { BranchSelector } from '@/features/projects/BranchSelector'
import type { ReportHistoryEntry, PaginationMeta, AllureStatistic } from '@/types/api'

const PER_PAGE = 20

// ---------------------------------------------------------------------------
// Sub-component: ProjectStatCards
// ---------------------------------------------------------------------------

interface ProjectStatCardsProps {
  isLoading: boolean
  stat: AllureStatistic | null | undefined
  passRate: number | null
  adjustedPassRate: number | null
  knownCount: number
  latest: ReportHistoryEntry | undefined
  pagination: PaginationMeta | undefined
}

function ProjectStatCards({
  isLoading,
  stat,
  passRate,
  adjustedPassRate,
  knownCount,
  latest,
  pagination,
}: ProjectStatCardsProps) {
  if (isLoading) {
    return (
      <div className="grid grid-cols-2 gap-4 sm:grid-cols-2 lg:grid-cols-4">
        {Array.from({ length: 4 }).map((_, i) => (
          <Skeleton key={i} className="h-28 w-full rounded-lg" />
        ))}
      </div>
    )
  }

  return (
    <div className="grid grid-cols-2 gap-4 sm:grid-cols-2 lg:grid-cols-4">
      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-sm font-medium text-muted-foreground">Pass Rate</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex items-center gap-2">
            <CheckCircle2 size={20} className="text-green-600" />
            <span
              className={`text-2xl font-bold ${passRate !== null && passRate >= 90 ? 'text-green-600' : passRate !== null && passRate >= 70 ? 'text-amber-600' : passRate !== null ? 'text-red-600' : ''}`}
            >
              {passRate !== null ? `${passRate}%` : '—'}
            </span>
          </div>
          {adjustedPassRate !== null && adjustedPassRate !== passRate && (
            <p className="mt-1 text-xs text-muted-foreground">
              {adjustedPassRate}% adjusted
              <span className="ml-1 text-xs opacity-70">(excl. {knownCount} known)</span>
            </p>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-sm font-medium text-muted-foreground">Total Tests</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex items-center gap-2">
            <BarChart3 size={20} className="text-blue-600" />
            <span className="text-2xl font-bold">{stat?.total ?? '—'}</span>
          </div>
          {stat && (
            <div className="mt-1 flex flex-wrap gap-1">
              <Badge className="bg-green-500 text-xs text-white">{stat.passed} passed</Badge>
              <Badge variant="destructive" className="text-xs">
                {stat.failed} failed
              </Badge>
              <Badge className="bg-amber-500 text-xs text-white">{stat.broken} broken</Badge>
              <Badge variant="secondary" className="text-xs">
                {stat.skipped} skipped
              </Badge>
            </div>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-sm font-medium text-muted-foreground">
            Last Duration
          </CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex items-center gap-2">
            <Clock size={20} className="text-purple-600" />
            <span className="text-2xl font-bold">
              {latest?.duration_ms ? formatDuration(latest.duration_ms) : '—'}
            </span>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-sm font-medium text-muted-foreground">Last Run</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex items-center gap-2">
            <XCircle size={20} className="text-orange-500" />
            <span className="text-sm font-medium">
              {latest?.generated_at ? formatDate(latest.generated_at) : '—'}
            </span>
          </div>
          {pagination && pagination.total > 0 && (
            <p className="mt-1 text-xs text-muted-foreground">
              {pagination.total} report{pagination.total !== 1 ? 's' : ''} total
            </p>
          )}
        </CardContent>
      </Card>
    </div>
  )
}

// ---------------------------------------------------------------------------
// Sub-component: ReportHistoryTable
// ---------------------------------------------------------------------------

interface ReportHistoryTableProps {
  projectId: string
  reports: ReportHistoryEntry[]
  isAdmin: () => boolean
  onDeleteReport: (reportId: string) => void
  selectedBuilds: Set<string>
  onToggleBuild: (id: string) => void
}

interface CommitGroup {
  sha: string
  reports: ReportHistoryEntry[]
  latestDate: string | null
}

function ReportRow({
  projectId,
  r,
  isAdmin,
  onDeleteReport,
  selectedBuilds,
  onToggleBuild,
}: {
  projectId: string
  r: ReportHistoryEntry
  isAdmin: () => boolean
  onDeleteReport: (reportId: string) => void
  selectedBuilds: Set<string>
  onToggleBuild: (id: string) => void
}) {
  const rStat = r.statistic
  const rPassRate = rStat ? calcPassRate(rStat.passed, rStat.total) : null
  const reportUrl = `/projects/${encodeURIComponent(projectId)}/reports/${encodeURIComponent(r.report_id)}`

  return (
    <TableRow className="cursor-pointer hover:bg-muted/50">
      <TableCell onClick={(e) => e.stopPropagation()}>
        <Checkbox
          checked={selectedBuilds.has(r.report_id)}
          onCheckedChange={() => onToggleBuild(r.report_id)}
          disabled={!selectedBuilds.has(r.report_id) && selectedBuilds.size >= 2}
          aria-label={`Select report #${r.report_id}`}
        />
      </TableCell>
      <TableCell>
        <Link
          to={reportUrl}
          className="font-mono text-sm font-medium text-primary hover:underline"
        >
          #{r.report_id}
        </Link>
      </TableCell>
      <TableCell className="text-sm text-muted-foreground">
        {r.generated_at ? formatDate(r.generated_at) : '—'}
      </TableCell>
      <TableCell className="text-center font-mono text-sm">{rStat?.total ?? '—'}</TableCell>
      <TableCell className="text-center font-mono text-sm text-green-600">
        {rStat?.passed ?? '—'}
      </TableCell>
      <TableCell className="text-center font-mono text-sm text-red-600">
        {rStat?.failed ?? '—'}
      </TableCell>
      <TableCell className="text-center font-mono text-sm text-amber-600">
        {rStat?.broken ?? '—'}
      </TableCell>
      <TableCell className="text-center font-mono text-sm text-muted-foreground">
        {rStat?.skipped ?? '—'}
      </TableCell>
      <TableCell className="text-center">
        {rPassRate !== null ? (
          <span
            className={
              rPassRate >= 90
                ? 'font-semibold text-green-600'
                : rPassRate >= 70
                  ? 'font-semibold text-amber-600'
                  : 'font-semibold text-red-600'
            }
          >
            {rPassRate}%
          </span>
        ) : (
          '—'
        )}
      </TableCell>
      <TableCell className="text-center">
        <div className="flex justify-center gap-1">
          {r.flaky_count != null && r.flaky_count > 0 && (
            <Badge className="bg-amber-500 text-xs text-white hover:bg-amber-600">
              Flaky: {r.flaky_count}
            </Badge>
          )}
          {r.new_failed_count != null && r.new_failed_count > 0 && (
            <Badge variant="destructive" className="text-xs">
              Regressed: {r.new_failed_count}
            </Badge>
          )}
          {r.new_passed_count != null && r.new_passed_count > 0 && (
            <Badge className="bg-green-500 text-xs text-white hover:bg-green-600">
              Fixed: {r.new_passed_count}
            </Badge>
          )}
        </div>
      </TableCell>
      <TableCell className="text-center">
        {r.ci_provider || r.ci_branch || r.ci_commit_sha ? (
          <div className="flex flex-col items-center gap-1">
            {r.ci_provider &&
              (r.ci_build_url && isSafeUrl(r.ci_build_url) ? (
                <a
                  href={r.ci_build_url}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="flex items-center gap-1 text-xs text-primary hover:underline"
                >
                  <ExternalLink size={10} />
                  {r.ci_provider}
                </a>
              ) : (
                <span className="text-xs text-muted-foreground">{r.ci_provider}</span>
              ))}
            {r.ci_branch && (
              <span className="flex items-center gap-1 text-xs text-muted-foreground">
                <GitBranch size={10} />
                {r.ci_branch}
              </span>
            )}
            {r.ci_commit_sha && (
              <span className="font-mono text-xs text-muted-foreground">
                {r.ci_commit_sha.slice(0, 7)}
              </span>
            )}
          </div>
        ) : (
          <span className="text-xs text-muted-foreground">—</span>
        )}
      </TableCell>
      <TableCell className="text-right">
        <div className="flex justify-end gap-1">
          <Button asChild size="sm" variant="ghost">
            <Link to={reportUrl}>View</Link>
          </Button>
          <Button asChild size="sm" variant="ghost">
            <a
              href={`${env.apiUrl}/projects/${encodeURIComponent(projectId)}/reports/${encodeURIComponent(r.report_id)}/index.html`}
              target="_blank"
              rel="noopener noreferrer"
              aria-label="Open in new tab"
            >
              <ExternalLink size={12} />
            </a>
          </Button>
          {isAdmin() && !r.is_latest && (
            <Button
              size="sm"
              variant="ghost"
              className="text-destructive hover:text-destructive"
              aria-label={`Delete report #${r.report_id}`}
              onClick={() => onDeleteReport(r.report_id)}
            >
              <Trash2 size={12} />
            </Button>
          )}
        </div>
      </TableCell>
    </TableRow>
  )
}

const TABLE_COL_COUNT = 12

function ReportHistoryTable({
  projectId,
  reports,
  isAdmin,
  onDeleteReport,
  selectedBuilds,
  onToggleBuild,
}: ReportHistoryTableProps) {
  const [expandedShas, setExpandedShas] = useState<Set<string>>(new Set())

  const { groups, ungrouped } = useMemo(() => {
    const shaMap = new Map<string, ReportHistoryEntry[]>()
    const ungroupedList: ReportHistoryEntry[] = []

    for (const r of reports) {
      if (r.ci_commit_sha) {
        const existing = shaMap.get(r.ci_commit_sha)
        if (existing) {
          existing.push(r)
        } else {
          shaMap.set(r.ci_commit_sha, [r])
        }
      } else {
        ungroupedList.push(r)
      }
    }

    const groupList: CommitGroup[] = []
    for (const [sha, groupReports] of shaMap.entries()) {
      if (groupReports.length < 2) {
        ungroupedList.push(...groupReports)
      } else {
        const latestDate = groupReports.reduce<string | null>((best, r) => {
          if (!r.generated_at) return best
          if (!best) return r.generated_at
          return r.generated_at > best ? r.generated_at : best
        }, null)
        groupList.push({ sha, reports: groupReports, latestDate })
      }
    }

    return { groups: groupList, ungrouped: ungroupedList }
  }, [reports])

  const toggleSha = (sha: string) => {
    setExpandedShas((prev) => {
      const next = new Set(prev)
      if (next.has(sha)) {
        next.delete(sha)
      } else {
        next.add(sha)
      }
      return next
    })
  }

  return (
    <div className="rounded-lg border">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead className="w-10" />
            <TableHead>Report</TableHead>
            <TableHead>Generated</TableHead>
            <TableHead className="text-center">Total</TableHead>
            <TableHead className="text-center text-green-600">Passed</TableHead>
            <TableHead className="text-center text-red-600">Failed</TableHead>
            <TableHead className="text-center text-amber-600">Broken</TableHead>
            <TableHead className="text-center text-gray-500">Skipped</TableHead>
            <TableHead className="text-center">Pass rate</TableHead>
            <TableHead className="text-center">Stability</TableHead>
            <TableHead className="text-center">CI</TableHead>
            <TableHead className="text-right">Actions</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {groups.map(({ sha, reports: groupReports, latestDate }) => {
            const isExpanded = expandedShas.has(sha)
            return (
              <>
                <TableRow
                  key={`group-${sha}`}
                  className="cursor-pointer bg-muted/30 hover:bg-muted/50"
                  onClick={() => toggleSha(sha)}
                  data-testid={`commit-group-${sha.slice(0, 7)}`}
                >
                  <TableCell colSpan={TABLE_COL_COUNT}>
                    <div className="flex items-center gap-2 text-sm">
                      <ChevronRight
                        size={14}
                        className={isExpanded ? 'rotate-90 transition-transform' : 'transition-transform'}
                      />
                      <span className="font-mono text-xs text-muted-foreground">
                        {sha.slice(0, 7)}
                      </span>
                      {latestDate && (
                        <span className="text-xs text-muted-foreground">
                          {formatDate(latestDate)}
                        </span>
                      )}
                      <Badge variant="secondary" className="text-xs">
                        {groupReports.length} builds
                      </Badge>
                    </div>
                  </TableCell>
                </TableRow>
                {isExpanded &&
                  groupReports.map((r) => (
                    <ReportRow
                      key={r.report_id}
                      projectId={projectId}
                      r={r}
                      isAdmin={isAdmin}
                      onDeleteReport={onDeleteReport}
                      selectedBuilds={selectedBuilds}
                      onToggleBuild={onToggleBuild}
                    />
                  ))}
              </>
            )
          })}
          {ungrouped.map((r) => (
            <ReportRow
              key={r.report_id}
              projectId={projectId}
              r={r}
              isAdmin={isAdmin}
              onDeleteReport={onDeleteReport}
              selectedBuilds={selectedBuilds}
              onToggleBuild={onToggleBuild}
            />
          ))}
        </TableBody>
      </Table>
    </div>
  )
}

// ---------------------------------------------------------------------------
// Sub-component: ReportPagination
// ---------------------------------------------------------------------------

interface ReportPaginationProps {
  page: number
  totalPages: number
  onPageChange: (updater: (p: number) => number) => void
}

function ReportPagination({ page, totalPages, onPageChange }: ReportPaginationProps) {
  return (
    <Pagination>
      <PaginationContent>
        <PaginationItem>
          <Button
            variant="outline"
            size="sm"
            onClick={() => onPageChange((p) => Math.max(1, p - 1))}
            disabled={page <= 1}
          >
            <ChevronLeft size={14} />
            Previous
          </Button>
        </PaginationItem>
        <PaginationItem>
          <span className="px-4 text-sm text-muted-foreground">
            Page {page} of {totalPages}
          </span>
        </PaginationItem>
        <PaginationItem>
          <Button
            variant="outline"
            size="sm"
            onClick={() => onPageChange((p) => Math.min(totalPages, p + 1))}
            disabled={page >= totalPages}
          >
            Next
            <ChevronRight size={14} />
          </Button>
        </PaginationItem>
      </PaginationContent>
    </Pagination>
  )
}

// ---------------------------------------------------------------------------
// Main component: OverviewTab
// ---------------------------------------------------------------------------

export function OverviewTab() {
  const { id: projectId } = useParams<{ id: string }>()
  const isAdmin = useAuthStore((s) => s.isAdmin)
  const queryClient = useQueryClient()
  const [sendOpen, setSendOpen] = useState(false)
  const [generateOpen, setGenerateOpen] = useState(false)
  const [cleanResultsOpen, setCleanResultsOpen] = useState(false)
  const [cleanHistoryOpen, setCleanHistoryOpen] = useState(false)
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
    queryFn: () => fetchReportHistory(projectId!, page, PER_PAGE, selectedBranch),
    enabled: !!projectId,
    staleTime: 10_000,
    placeholderData: keepPreviousData,
  })

  const { data: knownFailuresData } = useQuery({
    queryKey: queryKeys.reportKnownFailures(projectId ?? ''),
    queryFn: () => fetchReportKnownFailures(projectId!),
    enabled: !!projectId,
    staleTime: 30_000,
  })

  const deleteMutation = useMutation({
    mutationFn: (reportId: string) => deleteReport(projectId!, reportId),
    onSuccess: (_, reportId) => {
      void invalidateProjectQueries(queryClient, projectId!)
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
    const tableReports = reports.filter((r) => !r.is_latest)
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
        <p className="text-sm text-muted-foreground">Overview</p>
      </div>

      {/* Admin actions */}
      {isAdmin() && (
        <div className="rounded-lg border p-4">
          <p className="mb-3 text-sm font-medium">Admin actions</p>
          <TooltipProvider>
            <div className="flex flex-wrap gap-2">
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button size="sm" variant="outline" onClick={() => setSendOpen(true)}>
                    <Upload size={14} />
                    Send results
                  </Button>
                </TooltipTrigger>
                <TooltipContent>Upload Allure result files</TooltipContent>
              </Tooltip>
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button size="sm" variant="outline" onClick={() => setGenerateOpen(true)}>
                    <Play size={14} />
                    Generate report
                  </Button>
                </TooltipTrigger>
                <TooltipContent>Generate report from current results</TooltipContent>
              </Tooltip>
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button
                    size="sm"
                    variant="outline"
                    className="text-amber-600 hover:text-amber-700"
                    onClick={() => setCleanResultsOpen(true)}
                  >
                    <Trash2 size={14} />
                    Clean results
                  </Button>
                </TooltipTrigger>
                <TooltipContent>Delete pending result files</TooltipContent>
              </Tooltip>
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button
                    size="sm"
                    variant="outline"
                    className="text-destructive hover:text-destructive"
                    onClick={() => setCleanHistoryOpen(true)}
                  >
                    <Trash2 size={14} />
                    Clean history
                  </Button>
                </TooltipTrigger>
                <TooltipContent>Delete all report history</TooltipContent>
              </Tooltip>
            </div>
          </TooltipProvider>
        </div>
      )}

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
      <div className="grid grid-cols-1 gap-4 [&:empty]:hidden lg:grid-cols-3">
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
            <div className="flex items-center gap-3 rounded-lg border bg-muted/40 px-4 py-2">
              <span className="text-sm text-muted-foreground">2 builds selected</span>
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
            <p className="text-sm text-muted-foreground">
              {isAdmin()
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
        <ReportPagination
          page={page}
          totalPages={pagination.total_pages}
          onPageChange={setPage}
        />
      )}

      {/* Duration summary */}
      {latest?.duration_ms && (
        <p className="flex items-center gap-1 text-xs text-muted-foreground">
          <Clock size={12} />
          Latest suite duration:{' '}
          <span className="font-mono">{formatDuration(latest.duration_ms)}</span>
        </p>
      )}

      {/* Dialogs */}
      {isAdmin() && (
        <>
          <SendResultsDialog projectId={projectId} open={sendOpen} onOpenChange={setSendOpen} />
          <GenerateReportDialog
            projectId={projectId}
            open={generateOpen}
            onOpenChange={setGenerateOpen}
          />
          <CleanDialog
            projectId={projectId}
            mode="results"
            open={cleanResultsOpen}
            onOpenChange={setCleanResultsOpen}
          />
          <CleanDialog
            projectId={projectId}
            mode="history"
            open={cleanHistoryOpen}
            onOpenChange={setCleanHistoryOpen}
          />
        </>
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
