import { useState } from 'react'
import { useParams } from 'react-router'
import { ExternalLink } from 'lucide-react'
import { env } from '@/lib/env'
import { Button } from '@/components/ui/button'
import { Segmented, type SegmentedOption } from '@/components/ui/segmented'
import { useProjectFromParam } from '@/lib/resolveProject'
import { formatProjectLabel } from '@/lib/projectLabel'

// This page deviates from PageHeader because it wraps an embedded third-party
// report in an iframe that needs the maximum available height — a compact
// single-row toolbar is used instead of the standard page header.
export function ReportViewerPage() {
  const { id: projectId, reportId } = useParams<{ id: string; reportId: string }>()
  const { project, projects } = useProjectFromParam(projectId)
  const reportType = project?.report_type ?? 'allure'

  const defaultMode: 'playwright' | 'allure' = reportType === 'playwright' ? 'playwright' : 'allure'
  const [userOverride, setUserOverride] = useState<'playwright' | 'allure' | null>(null)
  const viewMode = userOverride ?? defaultMode
  const [iframeReloadKey, setIframeReloadKey] = useState(0)

  if (!projectId || !reportId) return null

  const reportUrl =
    viewMode === 'playwright'
      ? `${env.apiUrl}/projects/${encodeURIComponent(projectId)}/playwright-reports/${encodeURIComponent(reportId)}/index.html`
      : `${env.apiUrl}/projects/${encodeURIComponent(projectId)}/reports/${encodeURIComponent(reportId)}/index.html`

  const iframeTitle =
    viewMode === 'playwright'
      ? `Playwright report #${reportId} — ${projectId}`
      : `Allure report #${reportId} — ${projectId}`

  const viewToggleOptions: SegmentedOption<'playwright' | 'allure'>[] = [
    { value: 'playwright', label: 'Playwright', 'data-testid': 'view-toggle-playwright' },
    { value: 'allure', label: 'Allure', 'data-testid': 'view-toggle-allure' },
  ]

  return (
    <div className="-m-6 flex h-[calc(100vh-6rem)] flex-col">
      {/* Toolbar */}
      <div className="bg-background flex shrink-0 items-center gap-3 border-b px-4 py-2">
        <Button
          variant="ghost"
          size="sm"
          onClick={() => setIframeReloadKey((k) => k + 1)}
          title="Return to report home"
          className="font-mono"
          data-testid="report-home-link"
        >
          Report #{reportId}
        </Button>
        <span className="text-muted-foreground font-mono text-sm">
          {formatProjectLabel(project, projects)}
        </span>

        <div className="flex-1" />

        {reportType === 'playwright' && (
          <Segmented
            value={viewMode}
            onValueChange={setUserOverride}
            options={viewToggleOptions}
            aria-label="Report view"
          />
        )}

        <Button asChild variant="outline" size="sm">
          <a href={reportUrl} target="_blank" rel="noopener noreferrer">
            <ExternalLink size={14} />
            Open in new tab
          </a>
        </Button>
      </div>

      {/* Iframe — full remaining height */}
      <iframe
        key={`${viewMode}-${iframeReloadKey}`}
        src={reportUrl}
        title={iframeTitle}
        className="flex-1 border-0"
        sandbox="allow-scripts allow-same-origin allow-popups allow-forms allow-downloads"
        data-testid="allure-iframe"
      />
    </div>
  )
}
