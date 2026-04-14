import { useState, useMemo, Fragment } from 'react'
import { Link } from 'react-router'
import { ExternalLink, Trash2, GitBranch, ChevronRight, Clapperboard } from 'lucide-react'
import { env } from '@/lib/env'
import { isSafeUrl } from '@/lib/url'
import { formatDate, calcPassRate } from '@/lib/utils'
import { getPassRateColorClass, STATUS_TEXT_CLASSES } from '@/lib/status-colors'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Checkbox } from '@/components/ui/checkbox'
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
  groupBy: 'none' | 'commit' | 'branch'
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
  isAdmin: boolean
  onDeleteReport: (reportId: string) => void
  selectedBuilds: Set<string>
  onToggleBuild: (id: string) => void
}) {
  const rStat = r.statistic
  const rPassRate = rStat ? calcPassRate(rStat.passed, rStat.total) : null
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
      <TableCell className="text-center font-mono text-sm">{rStat?.total ?? '—'}</TableCell>
      <TableCell className={`text-center font-mono text-sm ${STATUS_TEXT_CLASSES.passed}`}>
        {rStat?.passed ?? '—'}
      </TableCell>
      <TableCell className={`text-center font-mono text-sm ${STATUS_TEXT_CLASSES.failed}`}>
        {rStat?.failed ?? '—'}
      </TableCell>
      <TableCell className={`text-center font-mono text-sm ${STATUS_TEXT_CLASSES.broken}`}>
        {rStat?.broken ?? '—'}
      </TableCell>
      <TableCell className="text-muted-foreground text-center font-mono text-sm">
        {rStat?.skipped ?? '—'}
      </TableCell>
      <TableCell className="text-center">
        {rPassRate !== null ? (
          <span className={`font-semibold ${getPassRateColorClass(rPassRate)}`}>{rPassRate}%</span>
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

// Must match <TableHead> count in ReportHistoryTable header
const TABLE_COL_COUNT = 11

export function ReportHistoryTable({
  projectId,
  reports,
  isAdmin,
  onDeleteReport,
  selectedBuilds,
  onToggleBuild,
  groupBy,
}: ReportHistoryTableProps) {
  const [expandedKeys, setExpandedKeys] = useState<Set<string>>(new Set())
  const [prevGroupBy, setPrevGroupBy] = useState<'none' | 'commit' | 'branch'>('none')

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
      const latestDate = groupReports.reduce<string | null>((best, r) => {
        if (!r.generated_at) return best
        if (!best) return r.generated_at
        return r.generated_at > best ? r.generated_at : best
      }, null)
      const label = groupBy === 'commit' ? key.slice(0, 7) : key
      groupList.push({ key, label, reports: groupReports, latestDate })
    }

    return { groups: groupList, ungrouped: ungroupedList }
  }, [reports, groupBy])

  // Expand all groups when groupBy changes — render-phase state update avoids
  // setState-in-effect and the extra commit cycle it causes.
  if (prevGroupBy !== groupBy) {
    setPrevGroupBy(groupBy)
    setExpandedKeys(groupBy !== 'none' ? new Set(groups.map((g) => g.key)) : new Set())
  }

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
    <div className="space-y-2" data-testid="report-list">
      {/* Table */}
      <div className="rounded-lg border">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="w-10" />
              <TableHead>Report</TableHead>
              <TableHead>Generated</TableHead>
              <TableHead className="text-center">Total</TableHead>
              <TableHead className={`text-center ${STATUS_TEXT_CLASSES.passed}`}>Passed</TableHead>
              <TableHead className={`text-center ${STATUS_TEXT_CLASSES.failed}`}>Failed</TableHead>
              <TableHead className={`text-center ${STATUS_TEXT_CLASSES.broken}`}>Broken</TableHead>
              <TableHead className={`text-center ${STATUS_TEXT_CLASSES.skipped}`}>
                Skipped
              </TableHead>
              <TableHead className="text-center">Pass rate</TableHead>
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
