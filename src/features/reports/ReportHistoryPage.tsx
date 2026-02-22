import { useState, useEffect } from 'react'
import { Link, useParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import {
  ChevronLeft,
  Upload,
  Play,
  Trash2,
  Clock,
  RefreshCw,
  ExternalLink,
  FileText,
} from 'lucide-react'
import { fetchReportSummary, getEmailableReportUrl } from '@/api/reports'
import { getConfig } from '@/api/system'
import { useAuthStore } from '@/store/auth'
import { env } from '@/lib/env'
import { formatDate, formatDuration, calcPassRate } from '@/lib/utils'
import type { ReportItem } from '@/types/api'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { SendResultsDialog } from './SendResultsDialog'
import { GenerateReportDialog } from './GenerateReportDialog'
import { CleanDialog } from './CleanDialog'

export function ReportHistoryPage() {
  const { id: projectId } = useParams<{ id: string }>()
  const isAdmin = useAuthStore((s) => s.isAdmin)
  const [sendOpen, setSendOpen] = useState(false)
  const [generateOpen, setGenerateOpen] = useState(false)
  const [cleanResultsOpen, setCleanResultsOpen] = useState(false)
  const [cleanHistoryOpen, setCleanHistoryOpen] = useState(false)
  const [reports, setReports] = useState<ReportItem[]>([])
  const [loadingReports, setLoadingReports] = useState(true)

  // Get config to know how many history items to probe
  const { data: configData } = useQuery({
    queryKey: ['config'],
    queryFn: getConfig,
    staleTime: 60_000,
  })
  const keepLatest = configData?.data.keep_history_latest ?? 20

  // Discover reports by probing numbered paths + latest
  useEffect(() => {
    if (!projectId) return

    setLoadingReports(true)
    const results: ReportItem[] = []

    const probeAll = async () => {
      // Probe "latest" first
      const latestSummary = await fetchReportSummary(projectId, 'latest')
      if (latestSummary) {
        results.push({
          reportId: 'latest',
          isLatest: true,
          summary: latestSummary,
          generatedAt: latestSummary.time?.stop
            ? new Date(latestSummary.time.stop).toISOString()
            : undefined,
          durationMs: latestSummary.time?.duration,
        })
      }

      // Probe numbered history (from highest to lowest)
      for (let i = keepLatest; i >= 1; i--) {
        const summary = await fetchReportSummary(projectId, String(i))
        if (summary) {
          results.push({
            reportId: String(i),
            isLatest: false,
            summary,
            generatedAt: summary.time?.stop
              ? new Date(summary.time.stop).toISOString()
              : undefined,
            durationMs: summary.time?.duration,
          })
        }
      }

      setReports(results)
      setLoadingReports(false)
    }

    probeAll()
  }, [projectId, keepLatest])

  if (!projectId) return null

  const latestReportUrl = `${env.apiUrl}/projects/${encodeURIComponent(projectId)}/reports/latest/index.html`

  return (
    <div className="space-y-6">
      {/* Breadcrumb + header */}
      <div>
        <Button asChild variant="ghost" size="sm" className="-ml-2 mb-2">
          <Link to="/">
            <ChevronLeft size={14} />
            Projects
          </Link>
        </Button>
        <div className="flex items-center justify-between gap-4">
          <div>
            <h1 className="font-mono text-2xl font-semibold">{projectId}</h1>
            <p className="text-sm text-muted-foreground">Report history</p>
          </div>
          {/* Admin action bar */}
          {isAdmin() && (
            <div className="flex flex-wrap items-center gap-2">
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
          )}
        </div>
      </div>

      {/* Quick actions for latest report */}
      <div className="flex gap-2">
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

      {/* Report history table */}
      {loadingReports ? (
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
                <TableHead className="text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {reports.map((r) => {
                const stat = r.summary?.statistic
                const passRate = stat ? calcPassRate(stat.passed, stat.total) : null
                const reportUrl = `/projects/${encodeURIComponent(projectId)}/reports/${encodeURIComponent(r.reportId)}`

                return (
                  <TableRow key={r.reportId} className="cursor-pointer hover:bg-muted/50">
                    <TableCell>
                      <div className="flex items-center gap-2">
                        <Link
                          to={reportUrl}
                          className="font-mono text-sm font-medium text-primary hover:underline"
                        >
                          #{r.reportId}
                        </Link>
                        {r.isLatest && (
                          <Badge variant="secondary" className="text-xs">
                            latest
                          </Badge>
                        )}
                      </div>
                    </TableCell>
                    <TableCell className="text-sm text-muted-foreground">
                      {r.generatedAt ? formatDate(r.generatedAt) : '—'}
                    </TableCell>
                    <TableCell className="text-center font-mono text-sm">
                      {stat?.total ?? '—'}
                    </TableCell>
                    <TableCell className="text-center font-mono text-sm text-green-600">
                      {stat?.passed ?? '—'}
                    </TableCell>
                    <TableCell className="text-center font-mono text-sm text-red-600">
                      {stat?.failed ?? '—'}
                    </TableCell>
                    <TableCell className="text-center font-mono text-sm text-amber-600">
                      {stat?.broken ?? '—'}
                    </TableCell>
                    <TableCell className="text-center font-mono text-sm text-muted-foreground">
                      {stat?.skipped ?? '—'}
                    </TableCell>
                    <TableCell className="text-center">
                      {passRate !== null ? (
                        <span
                          className={
                            passRate >= 90
                              ? 'font-semibold text-green-600'
                              : passRate >= 70
                                ? 'font-semibold text-amber-600'
                                : 'font-semibold text-red-600'
                          }
                        >
                          {passRate}%
                        </span>
                      ) : (
                        '—'
                      )}
                    </TableCell>
                    <TableCell className="text-right">
                      <div className="flex justify-end gap-1">
                        <Button asChild size="sm" variant="ghost">
                          <Link to={reportUrl}>View</Link>
                        </Button>
                        <Button asChild size="sm" variant="ghost">
                          <a
                            href={`${env.apiUrl}/projects/${encodeURIComponent(projectId)}/reports/${encodeURIComponent(r.reportId)}/index.html`}
                            target="_blank"
                            rel="noopener noreferrer"
                            aria-label="Open in new tab"
                          >
                            <ExternalLink size={12} />
                          </a>
                        </Button>
                      </div>
                    </TableCell>
                  </TableRow>
                )
              })}
            </TableBody>
          </Table>
        </div>
      )}

      {/* Duration summary for latest */}
      {reports[0]?.durationMs && (
        <p className="flex items-center gap-1 text-xs text-muted-foreground">
          <Clock size={12} />
          Latest suite duration:{' '}
          <span className="font-mono">{formatDuration(reports[0].durationMs)}</span>
        </p>
      )}

      {/* Admin dialogs */}
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
    </div>
  )
}
