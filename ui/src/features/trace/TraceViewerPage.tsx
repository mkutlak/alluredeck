import { useParams, useSearchParams, useNavigate, Link } from 'react-router'
import { useQuery } from '@tanstack/react-query'
import { ChevronLeft, ChevronRight, Download, X, Loader2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { env } from '@/lib/env'
import { queryKeys } from '@/lib/query-keys'
import { fetchAttachments, attachmentFileUrl } from '@/api/attachments'
import { isPlaywrightTrace } from './utils'
import type { AttachmentEntry } from '@/types/api'

// Strip /api/v1 (or /api/v1/) suffix to get the API server root.
function apiBaseUrl(): string {
  return env.apiUrl.replace(/\/api\/v1\/?$/, '')
}

const statusVariant: Record<string, 'passed' | 'failed' | 'broken' | 'skipped' | 'default'> = {
  passed: 'passed',
  failed: 'failed',
  broken: 'broken',
  skipped: 'skipped',
}

export default function TraceViewerPage() {
  const { id: projectId, source } = useParams<{ id: string; source: string }>()
  const [searchParams] = useSearchParams()
  const navigate = useNavigate()

  const reportId = searchParams.get('reportId') ?? 'latest'

  const { data, isLoading, isError } = useQuery({
    queryKey: queryKeys.attachments(projectId!, reportId),
    queryFn: () => fetchAttachments(projectId!, reportId),
    enabled: !!projectId,
    staleTime: 10_000,
  })

  if (!projectId || !source) return null

  // Collect all trace attachments for the current build (for prev/next navigation).
  const allTraces: AttachmentEntry[] = (data?.groups ?? []).flatMap((g) =>
    g.attachments.filter((a) => isPlaywrightTrace(a.name, a.mime_type)),
  )

  const currentIndex = allTraces.findIndex((a) => a.source === decodeURIComponent(source))
  const current = allTraces[currentIndex] ?? null

  // Find the group that owns this attachment (for test name + status).
  const ownerGroup = (data?.groups ?? []).find((g) =>
    g.attachments.some((a) => a.source === decodeURIComponent(source)),
  )

  const attachmentUrl = attachmentFileUrl(projectId, reportId, decodeURIComponent(source))
  const traceIndexUrl = `${apiBaseUrl()}/trace/index.html?trace=${encodeURIComponent(attachmentUrl)}`

  function navigateToTrace(att: AttachmentEntry) {
    void navigate(
      `/projects/${encodeURIComponent(projectId!)}/trace/${encodeURIComponent(att.source)}?reportId=${encodeURIComponent(reportId)}`,
    )
  }

  const prevTrace = currentIndex > 0 ? allTraces[currentIndex - 1] : null
  const nextTrace =
    currentIndex >= 0 && currentIndex < allTraces.length - 1 ? allTraces[currentIndex + 1] : null

  return (
    <div className="-m-6 flex h-[calc(100vh-3.5rem)] flex-col">
      {/* Context header */}
      <div className="bg-background flex shrink-0 items-center gap-2 border-b px-4 py-2">
        {/* Back link */}
        <Button asChild variant="ghost" size="sm">
          <Link to={`/projects/${projectId}/attachments`}>
            <ChevronLeft size={14} />
            {projectId}
          </Link>
        </Button>
        <span className="text-muted-foreground text-sm">/</span>

        {isLoading ? (
          <Loader2 className="text-muted-foreground h-4 w-4 animate-spin" />
        ) : ownerGroup ? (
          <div className="flex min-w-0 items-center gap-2">
            <span className="truncate text-sm font-medium" title={ownerGroup.test_name}>
              {ownerGroup.test_name}
            </span>
            <Badge
              variant={statusVariant[ownerGroup.test_status] ?? 'default'}
              className="shrink-0 capitalize"
            >
              {ownerGroup.test_status}
            </Badge>
            <span className="text-muted-foreground shrink-0 font-mono text-sm">
              Build #{reportId}
            </span>
          </div>
        ) : (
          <span className="font-mono text-sm">{decodeURIComponent(source)}</span>
        )}

        <div className="flex-1" />

        {/* Prev / next trace navigation */}
        <Button
          variant="outline"
          size="sm"
          disabled={!prevTrace}
          onClick={() => prevTrace && navigateToTrace(prevTrace)}
          title="Previous trace"
        >
          <ChevronLeft size={14} />
        </Button>
        <Button
          variant="outline"
          size="sm"
          disabled={!nextTrace}
          onClick={() => nextTrace && navigateToTrace(nextTrace)}
          title="Next trace"
        >
          <ChevronRight size={14} />
        </Button>

        {/* Download */}
        <Button asChild variant="outline" size="sm">
          <a href={`${attachmentUrl}?dl=1`} download={current?.name ?? decodeURIComponent(source)}>
            <Download size={14} />
            Download
          </a>
        </Button>

        {/* Close */}
        <Button
          variant="ghost"
          size="sm"
          onClick={() => void navigate(`/projects/${projectId}/attachments`)}
          title="Close"
        >
          <X size={14} />
        </Button>
      </div>

      {/* Error state */}
      {isError && (
        <div className="flex flex-1 flex-col items-center justify-center gap-3 text-center">
          <p className="text-destructive font-medium">Failed to load attachment metadata</p>
          <p className="text-muted-foreground text-sm">
            The attachment may not exist or you may not have permission to view it.
          </p>
        </div>
      )}

      {/* Iframe — fills remaining height */}
      {!isError && (
        <iframe
          key={traceIndexUrl}
          src={traceIndexUrl}
          title={`Playwright trace — ${decodeURIComponent(source)}`}
          className="flex-1 border-0"
          sandbox="allow-scripts allow-same-origin allow-popups allow-forms allow-downloads"
        />
      )}
    </div>
  )
}
