import { useState } from 'react'
import { Link, useParams } from 'react-router'
import { ChevronLeft, ExternalLink } from 'lucide-react'
import { useQuery } from '@tanstack/react-query'
import { env } from '@/lib/env'
import { Button } from '@/components/ui/button'
import { projectListOptions } from '@/lib/queries/projects'

export function ReportViewerPage() {
  const { id: projectId, reportId } = useParams<{ id: string; reportId: string }>()
  const { data: projectsResp } = useQuery(projectListOptions())
  const reportType =
    projectsResp?.data?.find((p: { project_id: number; slug: string }) => p.slug === projectId)
      ?.report_type ?? 'allure'

  const defaultMode: 'playwright' | 'allure' = reportType === 'playwright' ? 'playwright' : 'allure'
  const [userOverride, setUserOverride] = useState<'playwright' | 'allure' | null>(null)
  const viewMode = userOverride ?? defaultMode

  if (!projectId || !reportId) return null

  const reportUrl =
    viewMode === 'playwright'
      ? `${env.apiUrl}/projects/${encodeURIComponent(projectId)}/playwright-reports/${encodeURIComponent(reportId)}/index.html`
      : `${env.apiUrl}/projects/${encodeURIComponent(projectId)}/reports/${encodeURIComponent(reportId)}/index.html`

  const iframeTitle =
    viewMode === 'playwright'
      ? `Playwright report #${reportId} — ${projectId}`
      : `Allure report #${reportId} — ${projectId}`

  return (
    <div className="-m-6 flex h-[calc(100vh-3.5rem)] flex-col">
      {/* Toolbar */}
      <div className="bg-background flex shrink-0 items-center gap-3 border-b px-4 py-2">
        <Button asChild variant="ghost" size="sm">
          <Link to={`/projects/${projectId}`}>
            <ChevronLeft size={14} />
            {projectId}
          </Link>
        </Button>
        <span className="text-muted-foreground text-sm">/</span>
        <span className="font-mono text-sm font-medium">Report #{reportId}</span>

        <div className="flex-1" />

        {reportType === 'playwright' && (
          <div className="flex items-center rounded-md border">
            <Button
              variant="ghost"
              size="sm"
              className={`rounded-r-none border-r px-3 py-1 text-xs ${viewMode === 'playwright' ? 'bg-muted font-semibold' : 'font-normal'}`}
              onClick={() => setUserOverride('playwright')}
              aria-pressed={viewMode === 'playwright'}
              data-testid="view-toggle-playwright"
            >
              Playwright
            </Button>
            <Button
              variant="ghost"
              size="sm"
              className={`rounded-l-none px-3 py-1 text-xs ${viewMode === 'allure' ? 'bg-muted font-semibold' : 'font-normal'}`}
              onClick={() => setUserOverride('allure')}
              aria-pressed={viewMode === 'allure'}
              data-testid="view-toggle-allure"
            >
              Allure
            </Button>
          </div>
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
        src={reportUrl}
        title={iframeTitle}
        className="flex-1 border-0"
        sandbox="allow-scripts allow-same-origin allow-popups allow-forms allow-downloads"
        data-testid="allure-iframe"
      />
    </div>
  )
}
