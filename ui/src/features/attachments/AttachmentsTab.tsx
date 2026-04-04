import { useState } from 'react'
import { useParams, useNavigate } from 'react-router'
import { useQuery } from '@tanstack/react-query'
import { Paperclip, ChevronDown, ChevronRight } from 'lucide-react'
import { fetchAttachments } from '@/api/attachments'
import { fetchReportHistory } from '@/api/reports'
import { queryKeys } from '@/lib/query-keys'
import { Skeleton } from '@/components/ui/skeleton'
import { Button } from '@/components/ui/button'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { AttachmentRow } from './AttachmentRow'
import { AttachmentLightbox } from './AttachmentLightbox'
import { isPlaywrightTrace } from '@/features/trace/utils'
import type { AttachmentEntry, AttachmentGroup } from '@/types/api'

const MIME_FILTERS = [
  { label: 'All', value: '' },
  { label: 'Images', value: 'image' },
  { label: 'Logs', value: 'text' },
  { label: 'Traces', value: 'trace' },
  { label: 'Other', value: 'other' },
] as const

type MimeFilter = (typeof MIME_FILTERS)[number]['value']

const statusStyles: Record<string, string> = {
  passed: 'text-emerald-600 dark:text-emerald-400',
  failed: 'text-red-600 dark:text-red-400',
  broken: 'text-amber-600 dark:text-amber-400',
  skipped: 'text-gray-500 dark:text-gray-400',
  unknown: 'text-gray-500 dark:text-gray-400',
}

function filterAttachments(
  attachments: AttachmentEntry[],
  mimeFilter: MimeFilter,
): AttachmentEntry[] {
  if (mimeFilter === '') return attachments
  if (mimeFilter === 'image') return attachments.filter((a) => a.mime_type.startsWith('image/'))
  if (mimeFilter === 'text') return attachments.filter((a) => a.mime_type.startsWith('text/'))
  if (mimeFilter === 'trace')
    return attachments.filter((a) => isPlaywrightTrace(a.name, a.mime_type))
  if (mimeFilter === 'other')
    return attachments.filter(
      (a) =>
        !a.mime_type.startsWith('image/') &&
        !a.mime_type.startsWith('text/') &&
        !isPlaywrightTrace(a.name, a.mime_type),
    )
  return attachments
}

export function AttachmentsTab() {
  const { id: projectId } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const [selectedAttachment, setSelectedAttachment] = useState<AttachmentEntry | null>(null)
  const [mimeFilter, setMimeFilter] = useState<MimeFilter>('')
  const [collapsedGroups, setCollapsedGroups] = useState<Set<string>>(new Set())
  const [selectedReport, setSelectedReport] = useState('latest')

  // Fetch report list for the dropdown.
  const { data: reportsData } = useQuery({
    queryKey: queryKeys.reportHistory(projectId!, 1),
    queryFn: () => fetchReportHistory(projectId!, 1, 50),
    enabled: !!projectId,
    staleTime: 30_000,
  })

  const reports = reportsData?.data?.reports ?? []

  const { data, isLoading } = useQuery({
    queryKey: queryKeys.attachments(projectId!, selectedReport),
    queryFn: () => fetchAttachments(projectId!, selectedReport),
    enabled: !!projectId,
    staleTime: 10_000,
  })

  if (!projectId) return null

  if (isLoading) {
    return (
      <div className="space-y-4">
        <Skeleton className="h-8 w-48" />
        <div className="space-y-2">
          {Array.from({ length: 6 }).map((_, i) => (
            <Skeleton key={i} className="h-6 w-full rounded" />
          ))}
        </div>
      </div>
    )
  }

  const groups = data?.groups ?? []
  const total = data?.total ?? 0

  // Resolve display label for the selected report.
  const reportLabel =
    selectedReport === 'latest'
      ? reports.length > 0
        ? `#${reports[0].report_id} (latest)`
        : 'latest'
      : `#${selectedReport}`

  const toggleGroup = (testName: string) => {
    setCollapsedGroups((prev) => {
      const next = new Set(prev)
      if (next.has(testName)) {
        next.delete(testName)
      } else {
        next.add(testName)
      }
      return next
    })
  }

  // Filter groups and their attachments.
  const filteredGroups: AttachmentGroup[] = groups
    .map((group) => ({
      ...group,
      attachments: filterAttachments(group.attachments, mimeFilter),
    }))
    .filter((group) => group.attachments.length > 0)

  return (
    <div className="space-y-4">
      <div>
        <h1 className="font-mono text-2xl font-semibold">{projectId}</h1>
        <p className="text-muted-foreground text-sm">
          Attachments · Report {reportLabel} · {total} total
        </p>
      </div>

      <div className="flex flex-wrap items-center gap-2">
        {MIME_FILTERS.map(({ label, value }) => (
          <Button
            key={value}
            variant={mimeFilter === value ? 'default' : 'outline'}
            size="sm"
            onClick={() => setMimeFilter(value)}
          >
            {label}
          </Button>
        ))}
        <div className="ml-auto">
          <Select value={selectedReport} onValueChange={setSelectedReport}>
            <SelectTrigger className="w-44">
              <SelectValue placeholder="Select report" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="latest">Latest</SelectItem>
              {reports.map((r) => (
                <SelectItem key={r.report_id} value={r.report_id}>
                  #{r.report_id}
                  {r.is_latest ? ' (latest)' : ''}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      </div>

      {filteredGroups.length === 0 ? (
        <div className="flex flex-col items-center gap-3 rounded-lg border border-dashed py-16 text-center">
          <Paperclip className="text-muted-foreground/40 h-8 w-8" />
          <p className="font-medium">No attachments found</p>
          <p className="text-muted-foreground text-sm">
            {mimeFilter !== ''
              ? 'Try selecting a different filter.'
              : 'Generate a report with attachments to see them here.'}
          </p>
        </div>
      ) : (
        <div className="space-y-3">
          {filteredGroups.map((group) => {
            const isCollapsed = collapsedGroups.has(group.test_name)
            return (
              <div key={group.test_name} className="rounded-lg border">
                <button
                  type="button"
                  className="hover:bg-accent/50 flex w-full items-center gap-2 px-4 py-3 text-left transition-colors"
                  onClick={() => toggleGroup(group.test_name)}
                >
                  {isCollapsed ? (
                    <ChevronRight className="text-muted-foreground h-4 w-4 shrink-0" />
                  ) : (
                    <ChevronDown className="text-muted-foreground h-4 w-4 shrink-0" />
                  )}
                  <span className="truncate text-sm font-medium">{group.test_name}</span>
                  <span
                    className={`shrink-0 text-xs font-medium capitalize ${statusStyles[group.test_status] ?? ''}`}
                  >
                    {group.test_status}
                  </span>
                  <span className="text-muted-foreground ml-auto shrink-0 text-xs">
                    {group.attachments.length} file{group.attachments.length !== 1 ? 's' : ''}
                  </span>
                </button>
                {!isCollapsed && (
                  <div className="border-t px-4 py-3">
                    <div className="grid grid-cols-[1.25rem_1fr_auto_auto_auto] items-center gap-x-3 gap-y-1">
                      {group.attachments.map((attachment) => (
                        <AttachmentRow
                          key={attachment.id}
                          attachment={attachment}
                          onView={() => {
                            if (isPlaywrightTrace(attachment.name, attachment.mime_type)) {
                              void navigate(
                                `/projects/${encodeURIComponent(projectId)}/trace/${encodeURIComponent(attachment.source)}?reportId=${encodeURIComponent(selectedReport)}`,
                              )
                              return
                            }
                            setSelectedAttachment(attachment)
                          }}
                        />
                      ))}
                    </div>
                  </div>
                )}
              </div>
            )
          })}
        </div>
      )}

      {selectedAttachment && (
        <AttachmentLightbox
          attachment={selectedAttachment}
          open={!!selectedAttachment}
          onOpenChange={(open) => {
            if (!open) setSelectedAttachment(null)
          }}
        />
      )}
    </div>
  )
}
