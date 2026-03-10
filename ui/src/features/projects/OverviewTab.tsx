import { useState, useMemo, useEffect, Fragment } from 'react'
import { Link, useParams } from 'react-router'
import { useQuery, useMutation, useQueryClient, keepPreviousData } from '@tanstack/react-query'
import {
  ExternalLink,
  Trash2,
  CheckCircle2,
  XCircle,
  Clock,
  BarChart3,
  RefreshCw,
  GitBranch,
  GitCommitHorizontal,
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
import { toast } from '@/components/ui/use-toast'
import { Pagination, PaginationContent, PaginationItem } from '@/components/ui/pagination'
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
          <CardTitle className="text-muted-foreground text-sm font-medium">Pass Rate</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex items-center gap-2">
            <CheckCircle2 size={20} className="text-[#40a02b] dark:text-[#a6e3a1]" />
            <span
              className={`text-2xl font-bold ${passRate !== null && passRate >= 90 ? 'text-[#40a02b] dark:text-[#a6e3a1]' : passRate !== null && passRate >= 70 ? 'text-[#fe640b] dark:text-[#fab387]' : passRate !== null ? 'text-[#d20f39] dark:text-[#f38ba8]' : ''}`}
            >
              {passRate !== null ? `${passRate}%` : '—'}
            </span>
          </div>
          {adjustedPassRate !== null && adjustedPassRate !== passRate && (
            <p className="text-muted-foreground mt-1 text-xs">
              {adjustedPassRate}% adjusted
              <span className="ml-1 text-xs opacity-70">(excl. {knownCount} known)</span>
            </p>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-muted-foreground text-sm font-medium">Total Tests</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex items-center gap-2">
            <BarChart3 size={20} className="text-blue-600" />
            <span className="text-2xl font-bold">{stat?.total ?? '—'}</span>
          </div>
          {stat && (
            <div className="mt-1 flex flex-wrap gap-1">
              <Badge variant="passed" className="text-xs">
                {stat.passed} passed
              </Badge>
              <Badge variant="failed" className="text-xs">
                {stat.failed} failed
              </Badge>
              <Badge variant="broken" className="text-xs">
                {stat.broken} broken
              </Badge>
              <Badge variant="secondary" className="text-xs">
                {stat.skipped} skipped
              </Badge>
            </div>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-muted-foreground text-sm font-medium">Last Duration</CardTitle>
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
          <CardTitle className="text-muted-foreground text-sm font-medium">Last Run</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex items-center gap-2">
            <XCircle size={20} className="text-orange-500" />
            <span className="text-sm font-medium">
              {latest?.generated_at ? formatDate(latest.generated_at) : '—'}
            </span>
          </div>
          {pagination && pagination.total > 0 && (
            <p className="text-muted-foreground mt-1 text-xs">
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

interface ReportGroup {
  key: string
  label: string
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
    <TableRow className="hover:bg-muted/50 cursor-pointer">
      <TableCell onClick={(e) => e.stopPropagation()}>
        <Checkbox
          checked={selectedBuilds.has(r.report_id)}
          onCheckedChange={() => onToggleBuild(r.report_id)}
          disabled={!selectedBuilds.has(r.report_id) && selectedBuilds.size >= 2}
          aria-label={`Select report #${r.report_id}`}
        />
      </TableCell>
      <TableCell>
        <Link to={reportUrl} className="text-primary font-mono text-sm font-medium hover:underline">
          #{r.report_id}
        </Link>
      </TableCell>
      <TableCell className="text-muted-foreground text-sm">
        {r.generated_at ? formatDate(r.generated_at) : '—'}
      </TableCell>
      <TableCell className="text-center font-mono text-sm">{rStat?.total ?? '—'}</TableCell>
      <TableCell className="text-center font-mono text-sm text-[#40a02b] dark:text-[#a6e3a1]">
        {rStat?.passed ?? '—'}
      </TableCell>
      <TableCell className="text-center font-mono text-sm text-[#d20f39] dark:text-[#f38ba8]">
        {rStat?.failed ?? '—'}
      </TableCell>
      <TableCell className="text-center font-mono text-sm text-[#fe640b] dark:text-[#fab387]">
        {rStat?.broken ?? '—'}
      </TableCell>
      <TableCell className="text-muted-foreground text-center font-mono text-sm">
        {rStat?.skipped ?? '—'}
      </TableCell>
      <TableCell className="text-center">
        {rPassRate !== null ? (
          <span
            className={
              rPassRate >= 90
                ? 'font-semibold text-[#40a02b] dark:text-[#a6e3a1]'
                : rPassRate >= 70
                  ? 'font-semibold text-[#fe640b] dark:text-[#fab387]'
                  : 'font-semibold text-[#d20f39] dark:text-[#f38ba8]'
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
            <Badge variant="broken" className="text-xs">
              Flaky: {r.flaky_count}
            </Badge>
          )}
          {r.new_failed_count != null && r.new_failed_count > 0 && (
            <Badge variant="destructive" className="text-xs">
              Regressed: {r.new_failed_count}
            </Badge>
          )}
          {r.new_passed_count != null && r.new_passed_count > 0 && (
            <Badge variant="passed" className="text-xs">
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
                  className="text-primary flex items-center gap-1 text-xs hover:underline"
                >
                  <ExternalLink size={10} />
                  {r.ci_provider}
                </a>
              ) : (
                <span className="text-muted-foreground text-xs">{r.ci_provider}</span>
              ))}
            {r.ci_branch && (
              <span className="text-muted-foreground flex items-center gap-1 text-xs">
                <GitBranch size={10} />
                {r.ci_branch}
              </span>
            )}
            {r.ci_commit_sha && (
              <span className="text-muted-foreground font-mono text-xs">
                {r.ci_commit_sha.slice(0, 7)}
              </span>
            )}
          </div>
        ) : (
          <span className="text-muted-foreground text-xs">—</span>
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
  const [groupBy, setGroupBy] = useState<'none' | 'commit' | 'branch'>('none')
  const [expandedKeys, setExpandedKeys] = useState<Set<string>>(new Set())

  const { groups, ungrouped } = useMemo(() => {
    if (groupBy === 'none') {
      return { groups: [] as ReportGroup[], ungrouped: reports }
    }

    const keyMap = new Map<string, ReportHistoryEntry[]>()
    const ungroupedList: ReportHistoryEntry[] = []

    for (const r of reports) {
      const key = groupBy === 'commit' ? r.ci_commit_sha : r.ci_branch
      if (key) {
        const existing = keyMap.get(key)
        if (existing) {
          existing.push(r)
        } else {
          keyMap.set(key, [r])
        }
      } else {
        ungroupedList.push(r)
      }
    }

    const groupList: ReportGroup[] = []
    for (const [key, groupReports] of keyMap.entries()) {
      if (groupReports.length < 2) {
        ungroupedList.push(...groupReports)
      } else {
        const latestDate = groupReports.reduce<string | null>((best, r) => {
          if (!r.generated_at) return best
          if (!best) return r.generated_at
          return r.generated_at > best ? r.generated_at : best
        }, null)
        const label = groupBy === 'commit' ? key.slice(0, 7) : key
        groupList.push({ key, label, reports: groupReports, latestDate })
      }
    }

    return { groups: groupList, ungrouped: ungroupedList }
  }, [reports, groupBy])

  // Expand all groups when grouping is activated or groups change
  useEffect(() => {
    if (groupBy !== 'none' && groups.length > 0) {
      setExpandedKeys(new Set(groups.map((g) => g.key)))
    }
  }, [groups, groupBy])

  const toggleKey = (key: string) => {
    setExpandedKeys((prev) => {
      const next = new Set(prev)
      if (next.has(key)) {
        next.delete(key)
      } else {
        next.add(key)
      }
      return next
    })
  }

  return (
    <div className="space-y-2">
      {/* Grouping toolbar */}
      <div className="flex items-center gap-2">
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
        {groupBy !== 'none' && (
          <span className="text-muted-foreground text-xs">
            Grouped by {groupBy === 'commit' ? 'commit' : 'branch'}
          </span>
        )}
      </div>

      {/* Table */}
      <div className="rounded-lg border">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="w-10" />
              <TableHead>Report</TableHead>
              <TableHead>Generated</TableHead>
              <TableHead className="text-center">Total</TableHead>
              <TableHead className="text-center text-[#40a02b] dark:text-[#a6e3a1]">Passed</TableHead>
              <TableHead className="text-center text-[#d20f39] dark:text-[#f38ba8]">Failed</TableHead>
              <TableHead className="text-center text-[#fe640b] dark:text-[#fab387]">Broken</TableHead>
              <TableHead className="text-center text-[#6c6f85] dark:text-[#a6adc8]">
                Skipped
              </TableHead>
              <TableHead className="text-center">Pass rate</TableHead>
              <TableHead className="text-center">Stability</TableHead>
              <TableHead className="text-center">CI</TableHead>
              <TableHead className="text-right">Actions</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {groups.map(({ key, label, reports: groupReports, latestDate }) => {
              const isExpanded = expandedKeys.has(key)
              return (
                <Fragment key={`group-${key}`}>
                  <TableRow
                    className="bg-muted/20 hover:bg-muted/30 cursor-pointer"
                    onClick={() => toggleKey(key)}
                    data-testid={`${groupBy}-group-${label}`}
                  >
                    <TableCell colSpan={TABLE_COL_COUNT} className="py-0.5">
                      <div className="flex items-center gap-1.5 text-xs">
                        <ChevronRight
                          size={12}
                          className={
                            isExpanded ? 'rotate-90 transition-transform' : 'transition-transform'
                          }
                        />
                        {groupBy === 'commit' ? (
                          <span className="text-muted-foreground font-mono text-xs">{label}</span>
                        ) : (
                          <span className="text-muted-foreground flex items-center gap-1 text-xs">
                            <GitBranch size={10} />
                            {label}
                          </span>
                        )}
                        {latestDate && (
                          <span className="text-muted-foreground text-xs">
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
                </Fragment>
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
          <span className="text-muted-foreground px-4 text-sm">
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
