import { useState } from 'react'
import { Link, useParams } from 'react-router'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  ExternalLink,
  FileText,
  Upload,
  Play,
  Trash2,
  CheckCircle2,
  XCircle,
  Clock,
  BarChart3,
  RefreshCw,
} from 'lucide-react'
import { fetchReportHistory, deleteReport, getEmailableReportUrl, fetchReportKnownFailures } from '@/api/reports'
import { extractErrorMessage } from '@/api/client'
import { useAuthStore } from '@/store/auth'
import { env } from '@/lib/env'
import { formatDate, formatDuration, calcPassRate } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
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
import { SendResultsDialog } from '@/features/reports/SendResultsDialog'
import { GenerateReportDialog } from '@/features/reports/GenerateReportDialog'
import { CleanDialog } from '@/features/reports/CleanDialog'
import { EnvironmentCard } from '@/features/projects/EnvironmentCard'
import { CategoriesCard } from '@/features/projects/CategoriesCard'
import { FlakyTestsCard } from '@/features/projects/FlakyTestsCard'

export function OverviewTab() {
  const { id: projectId } = useParams<{ id: string }>()
  const isAdmin = useAuthStore((s) => s.isAdmin)
  const queryClient = useQueryClient()
  const [sendOpen, setSendOpen] = useState(false)
  const [generateOpen, setGenerateOpen] = useState(false)
  const [cleanResultsOpen, setCleanResultsOpen] = useState(false)
  const [cleanHistoryOpen, setCleanHistoryOpen] = useState(false)
  const [deleteReportId, setDeleteReportId] = useState<string | null>(null)

  const { data: historyData, isLoading } = useQuery({
    queryKey: ['report-history', projectId],
    queryFn: () => fetchReportHistory(projectId!),
    enabled: !!projectId,
    staleTime: 10_000,
  })

  const { data: knownFailuresData } = useQuery({
    queryKey: ['report-known-failures', projectId],
    queryFn: () => fetchReportKnownFailures(projectId!),
    enabled: !!projectId,
    staleTime: 30_000,
  })

  const deleteMutation = useMutation({
    mutationFn: (reportId: string) => deleteReport(projectId!, reportId),
    onSuccess: (_, reportId) => {
      queryClient.invalidateQueries({ queryKey: ['report-history', projectId] })
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

  if (!projectId) return null

  const reports = historyData?.reports ?? []
  const latest = reports[0]
  const stat = latest?.statistic
  const passRate = stat ? calcPassRate(stat.passed, stat.total) : null
  const knownCount = knownFailuresData?.known_failures?.length ?? 0
  const adjustedPassRate =
    stat && knownCount > 0
      ? calcPassRate(stat.passed + knownCount, stat.total)
      : null
  const latestReportUrl = `${env.apiUrl}/projects/${encodeURIComponent(projectId)}/reports/latest/index.html`

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
          <div className="flex flex-wrap gap-2">
            <Button size="sm" variant="outline" onClick={() => setSendOpen(true)}>
              <Upload size={14} />
              Send results
            </Button>
            <Button size="sm" variant="outline" onClick={() => setGenerateOpen(true)}>
              <Play size={14} />
              Generate report
            </Button>
            <Button
              size="sm"
              variant="outline"
              className="text-amber-600 hover:text-amber-700"
              onClick={() => setCleanResultsOpen(true)}
            >
              <Trash2 size={14} />
              Clean results
            </Button>
            <Button
              size="sm"
              variant="outline"
              className="text-destructive hover:text-destructive"
              onClick={() => setCleanHistoryOpen(true)}
            >
              <Trash2 size={14} />
              Clean history
            </Button>
          </div>
        </div>
      )}

      {/* Quick actions */}
      <div className="flex flex-wrap gap-2">
        <Button asChild variant="outline" size="sm">
          <a href={latestReportUrl} target="_blank" rel="noopener noreferrer">
            <ExternalLink size={14} />
            Open latest report
          </a>
        </Button>
        <Button asChild variant="ghost" size="sm">
          <a href={getEmailableReportUrl(projectId)} target="_blank" rel="noopener noreferrer">
            <FileText size={14} />
            Emailable report
          </a>
        </Button>
      </div>

      {/* Stat cards */}
      {isLoading ? (
        <div className="grid grid-cols-2 gap-4 sm:grid-cols-2 lg:grid-cols-4">
          {Array.from({ length: 4 }).map((_, i) => (
            <Skeleton key={i} className="h-28 w-full rounded-lg" />
          ))}
        </div>
      ) : (
        <div className="grid grid-cols-2 gap-4 sm:grid-cols-2 lg:grid-cols-4">
          <Card>
            <CardHeader className="pb-2">
              <CardTitle className="text-sm font-medium text-muted-foreground">Pass Rate</CardTitle>
            </CardHeader>
            <CardContent>
              <div className="flex items-center gap-2">
                <CheckCircle2 size={20} className="text-green-600" />
                <span className={`text-2xl font-bold ${passRate !== null && passRate >= 90 ? 'text-green-600' : passRate !== null && passRate >= 70 ? 'text-amber-600' : passRate !== null ? 'text-red-600' : ''}`}>
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
                <p className="mt-1 text-xs text-muted-foreground">
                  {stat.passed}p / {stat.failed}f / {stat.broken}b / {stat.skipped}s
                </p>
              )}
            </CardContent>
          </Card>

          <Card>
            <CardHeader className="pb-2">
              <CardTitle className="text-sm font-medium text-muted-foreground">Last Duration</CardTitle>
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
              {reports.length > 0 && (
                <p className="mt-1 text-xs text-muted-foreground">{reports.length} report{reports.length !== 1 ? 's' : ''} total</p>
              )}
            </CardContent>
          </Card>
        </div>
      )}

      {/* Environment & Categories & Flaky Tests — G1/G2/A1 */}
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-3">
        <EnvironmentCard projectId={projectId} />
        <CategoriesCard projectId={projectId} />
        <FlakyTestsCard projectId={projectId} />
      </div>

      {/* Report history table */}
      {isLoading ? (
        <div className="space-y-2">
          {Array.from({ length: 4 }).map((_, i) => (
            <Skeleton key={i} className="h-12 w-full" />
          ))}
        </div>
      ) : reports.length === 0 ? (
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
        <div className="rounded-lg border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Report</TableHead>
                <TableHead>Generated</TableHead>
                <TableHead className="text-center">Total</TableHead>
                <TableHead className="text-center text-green-600">Passed</TableHead>
                <TableHead className="text-center text-red-600">Failed</TableHead>
                <TableHead className="text-center text-amber-600">Broken</TableHead>
                <TableHead className="text-center text-gray-500">Skipped</TableHead>
                <TableHead className="text-center">Pass rate</TableHead>
                <TableHead className="text-center">Stability</TableHead>
                <TableHead className="text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {reports.map((r) => {
                const rStat = r.statistic
                const rPassRate = rStat ? calcPassRate(rStat.passed, rStat.total) : null
                const reportUrl = `/projects/${encodeURIComponent(projectId)}/reports/${encodeURIComponent(r.report_id)}`

                return (
                  <TableRow key={r.report_id} className="cursor-pointer hover:bg-muted/50">
                    <TableCell>
                      <div className="flex items-center gap-2">
                        <Link
                          to={reportUrl}
                          className="font-mono text-sm font-medium text-primary hover:underline"
                        >
                          #{r.report_id}
                        </Link>
                        {r.is_latest && (
                          <Badge variant="secondary" className="text-xs">
                            latest
                          </Badge>
                        )}
                      </div>
                    </TableCell>
                    <TableCell className="text-sm text-muted-foreground">
                      {r.generated_at ? formatDate(r.generated_at) : '—'}
                    </TableCell>
                    <TableCell className="text-center font-mono text-sm">
                      {rStat?.total ?? '—'}
                    </TableCell>
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
                            onClick={() => setDeleteReportId(r.report_id)}
                          >
                            <Trash2 size={12} />
                          </Button>
                        )}
                      </div>
                    </TableCell>
                  </TableRow>
                )
              })}
            </TableBody>
          </Table>
        </div>
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
          <GenerateReportDialog projectId={projectId} open={generateOpen} onOpenChange={setGenerateOpen} />
          <CleanDialog projectId={projectId} mode="results" open={cleanResultsOpen} onOpenChange={setCleanResultsOpen} />
          <CleanDialog projectId={projectId} mode="history" open={cleanHistoryOpen} onOpenChange={setCleanHistoryOpen} />
        </>
      )}

      {/* Delete confirmation */}
      <AlertDialog
        open={deleteReportId !== null}
        onOpenChange={(open) => { if (!open) setDeleteReportId(null) }}
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
