import { Link } from 'react-router'
import { ExternalLink, Trash2, GitBranch, Clapperboard } from 'lucide-react'
import { env } from '@/lib/env'
import { isSafeUrl } from '@/lib/url'
import { formatDate, calcPassRate, formatPassRate } from '@/lib/utils'
import { getPassRateColorClass } from '@/lib/status-colors'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { StatusDistributionBar } from '@/components/ui/StatusDistributionBar'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import type { ReportHistoryEntry } from '@/types/api'

export interface ReportHistoryTableProps {
  projectId: string
  reports: ReportHistoryEntry[]
  isAdmin: boolean
  onDeleteReport: (reportId: string) => void
  selectedBuilds: Set<string>
  onToggleBuild: (id: string) => void
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
  isAdmin: boolean
  onDeleteReport: (reportId: string) => void
  selectedBuilds: Set<string>
  onToggleBuild: (id: string) => void
}) {
  const rStat = r.statistic
  const rPassRate = rStat ? calcPassRate(rStat.passed, rStat.total, rStat.skipped) : null
  const reportUrl = `/projects/${encodeURIComponent(projectId)}/reports/${encodeURIComponent(r.report_id)}`

  return (
    <TableRow
      className="hover:bg-muted/50 cursor-pointer"
      data-testid="report-row"
      data-report-id={r.report_id}
    >
      <TableCell onClick={(e) => e.stopPropagation()}>
        <Checkbox
          checked={selectedBuilds.has(r.report_id)}
          onCheckedChange={() => onToggleBuild(r.report_id)}
          disabled={!selectedBuilds.has(r.report_id) && selectedBuilds.size >= 2}
          aria-label={`Select report #${r.report_id}`}
        />
      </TableCell>
      <TableCell>
        <div className="flex items-center gap-1.5">
          <Link
            to={reportUrl}
            className="text-primary font-mono text-sm font-medium hover:underline"
          >
            #{r.report_id}
          </Link>
          {r.has_playwright_report && (
            <span
              title="Has Playwright report"
              aria-label="Has Playwright report"
              className="text-muted-foreground"
            >
              <Clapperboard size={12} />
            </span>
          )}
        </div>
      </TableCell>
      <TableCell className="text-muted-foreground text-sm">
        {r.generated_at ? formatDate(r.generated_at) : '—'}
      </TableCell>
      <TableCell>
        {rStat ? (
          <StatusDistributionBar
            passed={rStat.passed}
            failed={rStat.failed}
            broken={rStat.broken}
            skipped={rStat.skipped}
          />
        ) : (
          <span className="text-muted-foreground text-sm">—</span>
        )}
      </TableCell>
      <TableCell className="text-center">
        {rStat && rPassRate !== null ? (
          <span className={`font-semibold ${getPassRateColorClass(rPassRate)}`}>
            {formatPassRate(rStat.passed, rStat.total, rStat.skipped)}
          </span>
        ) : (
          '—'
        )}
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
          {isAdmin && !r.is_latest && (
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

export function ReportHistoryTable({
  projectId,
  reports,
  isAdmin,
  onDeleteReport,
  selectedBuilds,
  onToggleBuild,
}: ReportHistoryTableProps) {
  return (
    <div className="space-y-2" data-testid="report-list">
      <div className="rounded-lg border">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="w-10" />
              <TableHead>Report</TableHead>
              <TableHead>Generated</TableHead>
              <TableHead>Results</TableHead>
              <TableHead className="text-center">Pass rate</TableHead>
              <TableHead className="text-center">CI</TableHead>
              <TableHead className="text-right">Actions</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {reports.map((r) => (
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
