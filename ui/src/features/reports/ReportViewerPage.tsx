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
    projectsResp?.data?.find((p: { project_id: string }) => p.project_id === projectId)
      ?.report_type ?? 'allure'

  if (!projectId || !reportId) return null

  const reportUrl = `${env.apiUrl}/projects/${encodeURIComponent(projectId)}/reports/${encodeURIComponent(reportId)}/index.html`

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
        title={`${reportType === 'playwright' ? 'Playwright' : 'Allure'} report #${reportId} — ${projectId}`}
        className="flex-1 border-0"
        sandbox="allow-scripts allow-same-origin allow-popups allow-forms allow-downloads"
      />
    </div>
  )
}
