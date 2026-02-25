import { Link, useParams } from 'react-router'
import { ChevronLeft, ExternalLink, FileText } from 'lucide-react'
import { env } from '@/lib/env'
import { getEmailableReportUrl } from '@/api/reports'
import { Button } from '@/components/ui/button'
import { useAuthStore } from '@/store/auth'

export function ReportViewerPage() {
  const { id: projectId, reportId } = useParams<{ id: string; reportId: string }>()
  const isAdmin = useAuthStore((s) => s.isAdmin)

  if (!projectId || !reportId) return null

  const reportUrl = `${env.apiUrl}/projects/${encodeURIComponent(projectId)}/reports/${encodeURIComponent(reportId)}/index.html`

  return (
    <div className="-m-6 flex h-[calc(100vh-3.5rem)] flex-col">
      {/* Toolbar */}
      <div className="flex shrink-0 items-center gap-3 border-b bg-background px-4 py-2">
        <Button asChild variant="ghost" size="sm">
          <Link to={`/projects/${projectId}/history`}>
            <ChevronLeft size={14} />
            {projectId}
          </Link>
        </Button>
        <span className="text-sm text-muted-foreground">/</span>
        <span className="font-mono text-sm font-medium">Report #{reportId}</span>

        <div className="flex-1" />

        <Button asChild variant="outline" size="sm">
          <a href={reportUrl} target="_blank" rel="noopener noreferrer">
            <ExternalLink size={14} />
            Open in new tab
          </a>
        </Button>
        <Button asChild variant="ghost" size="sm">
          <a
            href={getEmailableReportUrl(projectId)}
            target="_blank"
            rel="noopener noreferrer"
          >
            <FileText size={14} />
            Emailable report
          </a>
        </Button>
        {isAdmin() && (
          <Button asChild variant="ghost" size="sm">
            <a href={getEmailableReportUrl(projectId)} download>
              Download
            </a>
          </Button>
        )}
      </div>

      {/* Iframe — full remaining height */}
      <iframe
        src={reportUrl}
        title={`Allure report #${reportId} — ${projectId}`}
        className="flex-1 border-0"
        sandbox="allow-scripts allow-same-origin allow-popups allow-forms"
      />
    </div>
  )
}
