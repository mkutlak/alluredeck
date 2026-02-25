import { useParams } from 'react-router'
import { useQuery } from '@tanstack/react-query'
import { ExternalLink, FileText, Upload, Play, Trash2, CheckCircle2, XCircle, Clock, BarChart3 } from 'lucide-react'
import { useState } from 'react'
import { fetchReportHistory, getEmailableReportUrl } from '@/api/reports'
import { useAuthStore } from '@/store/auth'
import { env } from '@/lib/env'
import { formatDate, formatDuration, calcPassRate } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { SendResultsDialog } from '@/features/reports/SendResultsDialog'
import { GenerateReportDialog } from '@/features/reports/GenerateReportDialog'
import { CleanDialog } from '@/features/reports/CleanDialog'

export function OverviewTab() {
  const { id: projectId } = useParams<{ id: string }>()
  const isAdmin = useAuthStore((s) => s.isAdmin)
  const [sendOpen, setSendOpen] = useState(false)
  const [generateOpen, setGenerateOpen] = useState(false)
  const [cleanResultsOpen, setCleanResultsOpen] = useState(false)
  const [cleanHistoryOpen, setCleanHistoryOpen] = useState(false)

  const { data: historyData, isLoading } = useQuery({
    queryKey: ['report-history', projectId],
    queryFn: () => fetchReportHistory(projectId!),
    enabled: !!projectId,
    staleTime: 10_000,
  })

  if (!projectId) return null

  const reports = historyData?.reports ?? []
  const latest = reports[0]
  const stat = latest?.statistic
  const passRate = stat ? calcPassRate(stat.passed, stat.total) : null
  const latestReportUrl = `${env.apiUrl}/projects/${encodeURIComponent(projectId)}/reports/latest/index.html`

  return (
    <div className="space-y-6">
      {/* Page title */}
      <div>
        <h1 className="font-mono text-2xl font-semibold">{projectId}</h1>
        <p className="text-sm text-muted-foreground">Overview</p>
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

      {isAdmin() && (
        <>
          <SendResultsDialog projectId={projectId} open={sendOpen} onOpenChange={setSendOpen} />
          <GenerateReportDialog projectId={projectId} open={generateOpen} onOpenChange={setGenerateOpen} />
          <CleanDialog projectId={projectId} mode="results" open={cleanResultsOpen} onOpenChange={setCleanResultsOpen} />
          <CleanDialog projectId={projectId} mode="history" open={cleanHistoryOpen} onOpenChange={setCleanHistoryOpen} />
        </>
      )}
    </div>
  )
}
