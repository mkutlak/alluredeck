import { useState } from 'react'
import { useParams } from 'react-router'
import { MoreHorizontal, Upload, Trash2 } from 'lucide-react'
import { useQuery } from '@tanstack/react-query'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { useAuthStore, selectIsAdmin, selectIsEditor } from '@/store/auth'
import { SendResultsDialog } from '@/features/reports/SendResultsDialog'
import { CleanDialog } from '@/features/reports/CleanDialog'
import { projectIndexOptions } from '@/lib/queries/projects'
import { formatProjectLabel } from '@/lib/projectLabel'
import { resolveProjectFromParam } from '@/lib/resolveProject'

export function ProjectActionsMenu() {
  const { id: projectId } = useParams<{ id: string }>()
  const isAdmin = useAuthStore(selectIsAdmin)
  const isEditor = useAuthStore(selectIsEditor)
  const { data: projectsResp } = useQuery(projectIndexOptions())
  const reportType =
    projectsResp?.data?.find((p: { project_id: number }) => p.project_id === Number(projectId))
      ?.report_type ?? 'allure'
  const isAllure = reportType !== 'playwright'
  // How the dialogs name the project in their text and toasts: parent/child, never the bare id.
  const label =
    formatProjectLabel(
      resolveProjectFromParam(projectId, projectsResp?.data),
      projectsResp?.data,
    ) || projectId
  const [sendOpen, setSendOpen] = useState(false)
  const [cleanResultsOpen, setCleanResultsOpen] = useState(false)
  const [cleanHistoryOpen, setCleanHistoryOpen] = useState(false)

  if (!projectId || !isEditor) return null

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button size="icon" variant="outline" aria-label="Project actions">
            <MoreHorizontal size={14} />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          {isAllure && (
            <DropdownMenuItem onSelect={() => setSendOpen(true)}>
              <Upload size={14} />
              Send results
            </DropdownMenuItem>
          )}
          {isAdmin && (
            <>
              <DropdownMenuItem
                className="text-warning focus:text-warning"
                onSelect={() => setCleanResultsOpen(true)}
              >
                <Trash2 size={14} />
                Clean results
              </DropdownMenuItem>
              <DropdownMenuItem
                className="text-destructive focus:text-destructive"
                onSelect={() => setCleanHistoryOpen(true)}
              >
                <Trash2 size={14} />
                Clean history
              </DropdownMenuItem>
            </>
          )}
        </DropdownMenuContent>
      </DropdownMenu>

      <SendResultsDialog
        projectId={Number(projectId)}
        label={label}
        open={sendOpen}
        onOpenChange={setSendOpen}
      />
      <CleanDialog
        projectId={projectId}
        label={label}
        mode="results"
        open={cleanResultsOpen}
        onOpenChange={setCleanResultsOpen}
      />
      <CleanDialog
        projectId={projectId}
        label={label}
        mode="history"
        open={cleanHistoryOpen}
        onOpenChange={setCleanHistoryOpen}
      />
    </>
  )
}
