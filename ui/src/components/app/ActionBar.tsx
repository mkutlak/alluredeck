import { useState } from 'react'
import { useParams } from 'react-router'
import { Upload, Trash2 } from 'lucide-react'
import { useQuery } from '@tanstack/react-query'
import { Button } from '@/components/ui/button'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { useAuthStore, selectIsAdmin, selectIsEditor } from '@/store/auth'
import { SendResultsDialog } from '@/features/reports/SendResultsDialog'
import { CleanDialog } from '@/features/reports/CleanDialog'
import { projectListOptions } from '@/lib/queries/projects'

export function ActionBar() {
  const { id: projectId } = useParams<{ id: string }>()
  const isAdmin = useAuthStore(selectIsAdmin)
  const isEditor = useAuthStore(selectIsEditor)
  const { data: projectsResp } = useQuery(projectListOptions())
  const reportType =
    projectsResp?.data?.find((p: { project_id: string }) => p.project_id === projectId)
      ?.report_type ?? 'allure'
  const isAllure = reportType !== 'playwright'
  const [sendOpen, setSendOpen] = useState(false)
  const [cleanResultsOpen, setCleanResultsOpen] = useState(false)
  const [cleanHistoryOpen, setCleanHistoryOpen] = useState(false)

  if (!projectId || !isEditor) return null

  return (
    <div className="bg-muted/30 flex items-center justify-end gap-2 border-b px-6 py-2">
      {isAllure && (
        <>
          <Tooltip>
            <TooltipTrigger asChild>
              <Button size="sm" variant="outline" onClick={() => setSendOpen(true)}>
                <Upload size={14} />
                Send results
              </Button>
            </TooltipTrigger>
            <TooltipContent>Upload Allure result files</TooltipContent>
          </Tooltip>

        </>
      )}

      {isAdmin && (
        <>
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                size="sm"
                variant="outline"
                className="text-amber-600 hover:text-amber-700"
                onClick={() => setCleanResultsOpen(true)}
              >
                <Trash2 size={14} />
                Clean results
              </Button>
            </TooltipTrigger>
            <TooltipContent>Delete pending result files</TooltipContent>
          </Tooltip>

          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                size="sm"
                variant="outline"
                className="text-destructive hover:text-destructive"
                onClick={() => setCleanHistoryOpen(true)}
              >
                <Trash2 size={14} />
                Clean history
              </Button>
            </TooltipTrigger>
            <TooltipContent>Delete all report history</TooltipContent>
          </Tooltip>
        </>
      )}

      <SendResultsDialog projectId={projectId} open={sendOpen} onOpenChange={setSendOpen} />
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
    </div>
  )
}
