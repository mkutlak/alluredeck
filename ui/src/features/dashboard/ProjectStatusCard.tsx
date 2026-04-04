import { useState } from 'react'
import { NavLink } from 'react-router'
import { FolderInput, FolderOutput, MoreHorizontal, Pencil, Trash2 } from 'lucide-react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Card, CardContent, CardHeader } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { formatDate, formatDuration } from '@/lib/utils'
import { getPassRateBadgeClass } from '@/lib/status-colors'
import { PassRateSparkline } from './PassRateSparkline'
import { DeleteProjectDialog } from '@/features/projects/DeleteProjectDialog'
import { RenameProjectDialog } from '@/features/projects/RenameProjectDialog'
import { SetParentDialog } from '@/features/projects/SetParentDialog'
import { clearProjectParent } from '@/api/projects'
import { queryKeys } from '@/lib/query-keys'
import { useAuthStore, selectIsAdmin } from '@/store/auth'
import type { DashboardProjectEntry } from '@/types/api'

// Isolated component so useQueryClient/useMutation are only called when this
// menu item is actually present in the tree (i.e. only for non-group projects
// whose dropdown is open). Tests that render ProjectStatusCard without a
// QueryClientProvider remain unaffected because they never open the dropdown.
function RemoveFromGroupItem({ projectId }: { projectId: string }) {
  const qc = useQueryClient()
  const { mutate } = useMutation({
    mutationFn: () => clearProjectParent(projectId),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.projects })
      void qc.invalidateQueries({ queryKey: queryKeys.dashboard() })
    },
  })

  return (
    <DropdownMenuItem onClick={() => mutate()}>
      <FolderOutput size={14} />
      Remove from group
    </DropdownMenuItem>
  )
}

interface AdminActionsProps {
  project: DashboardProjectEntry
  showRemoveFromGroup: boolean
}

function AdminActions({ project, showRemoveFromGroup }: AdminActionsProps) {
  const [deleteOpen, setDeleteOpen] = useState(false)
  const [renameOpen, setRenameOpen] = useState(false)
  const [moveOpen, setMoveOpen] = useState(false)

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            variant="ghost"
            size="icon"
            className="h-6 w-6 shrink-0 opacity-0 transition-opacity group-hover:opacity-100"
            aria-label="Project actions"
          >
            <MoreHorizontal size={14} />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuItem onClick={() => setRenameOpen(true)}>
            <Pencil size={14} />
            Rename project
          </DropdownMenuItem>
          <DropdownMenuItem onClick={() => setMoveOpen(true)}>
            <FolderInput size={14} />
            Move to group...
          </DropdownMenuItem>
          {showRemoveFromGroup && <RemoveFromGroupItem projectId={project.project_id} />}
          <DropdownMenuItem
            className="text-destructive focus:text-destructive"
            onClick={() => setDeleteOpen(true)}
          >
            <Trash2 size={14} />
            Delete project
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>

      {renameOpen && (
        <RenameProjectDialog
          projectId={project.project_id}
          open={renameOpen}
          onOpenChange={setRenameOpen}
        />
      )}
      {deleteOpen && (
        <DeleteProjectDialog
          projectId={project.project_id}
          open={deleteOpen}
          onOpenChange={setDeleteOpen}
        />
      )}
      {moveOpen && (
        <SetParentDialog
          projectId={project.project_id}
          open={moveOpen}
          onOpenChange={setMoveOpen}
        />
      )}
    </>
  )
}

interface Props {
  project: DashboardProjectEntry
}

export function ProjectStatusCard({ project }: Props) {
  const isAdmin = useAuthStore(selectIsAdmin)
  const { latest_build, sparkline } = project
  const passRate = latest_build?.pass_rate ?? 0
  // Show "Remove from group" only for child projects (not group nodes themselves)
  const showRemoveFromGroup = !project.is_group && project.children === undefined

  return (
    <>
      <Card className="group flex flex-col">
        <CardHeader className="pb-2">
          <div className="flex items-center justify-between gap-2">
            <NavLink
              to={`/projects/${project.project_id}`}
              className="truncate font-semibold hover:underline"
              onClick={(e) => e.stopPropagation()}
            >
              {project.project_id}
            </NavLink>
            <div className="flex items-center gap-1">
              {latest_build ? (
                <Badge
                  variant={
                    passRate >= 90 ? 'default' : passRate >= 70 ? 'secondary' : 'destructive'
                  }
                  className={getPassRateBadgeClass(passRate)}
                >
                  {passRate.toFixed(0)}%
                </Badge>
              ) : (
                <Badge variant="secondary">No builds</Badge>
              )}
              {isAdmin && (
                <AdminActions project={project} showRemoveFromGroup={showRemoveFromGroup} />
              )}
            </div>
          </div>
        </CardHeader>
        <CardContent className="flex flex-1 flex-col gap-3">
          {sparkline.length > 0 && <PassRateSparkline data={sparkline} />}

          {latest_build ? (
            <div className="text-muted-foreground space-y-1 text-sm">
              <div className="flex justify-between">
                <span>Tests</span>
                <span className="text-foreground font-medium">{latest_build.statistics.total}</span>
              </div>
              {latest_build.statistics.failed + latest_build.statistics.broken > 0 && (
                <div className="flex justify-between">
                  <span>Failures</span>
                  <span className="text-destructive font-medium">
                    {latest_build.statistics.failed + latest_build.statistics.broken}
                  </span>
                </div>
              )}
              {latest_build.flaky_count > 0 && (
                <div className="flex justify-between">
                  <span>Flaky</span>
                  <span className="font-medium">{latest_build.flaky_count}</span>
                </div>
              )}
              <div className="flex justify-between">
                <span>Duration</span>
                <span className="text-foreground font-medium">
                  {formatDuration(latest_build.duration_ms)}
                </span>
              </div>
              <div className="flex justify-between">
                <span>Last run</span>
                <span className="text-foreground font-medium">
                  {formatDate(latest_build.created_at)}
                </span>
              </div>
              {latest_build.ci_branch && (
                <div className="flex justify-between">
                  <span>Branch</span>
                  <span className="text-foreground font-medium">{latest_build.ci_branch}</span>
                </div>
              )}
            </div>
          ) : (
            <p className="text-muted-foreground text-sm">No runs yet</p>
          )}
        </CardContent>
      </Card>
    </>
  )
}
