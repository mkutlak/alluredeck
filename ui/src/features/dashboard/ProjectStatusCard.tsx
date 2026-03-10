import { useState } from 'react'
import { NavLink } from 'react-router'
import { MoreHorizontal, Tags, Trash2 } from 'lucide-react'
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
import { PassRateSparkline } from './PassRateSparkline'
import { DeleteProjectDialog } from '@/features/projects/DeleteProjectDialog'
import { EditTagsDialog } from '@/features/projects/EditTagsDialog'
import { useAuthStore } from '@/store/auth'
import type { DashboardProjectEntry } from '@/types/api'

interface Props {
  project: DashboardProjectEntry
}

function passRateBadgeVariant(rate: number): 'default' | 'secondary' | 'destructive' {
  if (rate >= 90) return 'default'
  if (rate >= 70) return 'secondary'
  return 'destructive'
}

export function ProjectStatusCard({ project }: Props) {
  const isAdmin = useAuthStore((s) => s.isAdmin)
  const [deleteOpen, setDeleteOpen] = useState(false)
  const [editTagsOpen, setEditTagsOpen] = useState(false)
  const { latest_build, sparkline } = project
  const passRate = latest_build?.pass_rate ?? 0
  const tags = project.tags ?? []

  return (
    <>
      <Card className="group flex flex-col">
        <CardHeader className="pb-2">
          <div className="flex items-center justify-between gap-2">
            <span className="truncate font-semibold">{project.project_id}</span>
            <div className="flex items-center gap-1">
              {latest_build ? (
                <Badge
                  variant={passRateBadgeVariant(passRate)}
                  className={
                    passRate >= 90
                      ? 'bg-[#40a02b] text-white hover:bg-[#40a02b]/90 dark:bg-[#a6e3a1] dark:text-[#1e1e2e] dark:hover:bg-[#a6e3a1]/90'
                      : passRate >= 70
                        ? 'bg-[#fe640b] text-white hover:bg-[#fe640b]/90 dark:bg-[#fab387] dark:text-[#1e1e2e] dark:hover:bg-[#fab387]/90'
                        : undefined
                  }
                >
                  {passRate.toFixed(0)}%
                </Badge>
              ) : (
                <Badge variant="secondary">No builds</Badge>
              )}
              {isAdmin() && (
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
                    <DropdownMenuItem onClick={() => setEditTagsOpen(true)}>
                      <Tags size={14} />
                      Edit tags
                    </DropdownMenuItem>
                    <DropdownMenuItem
                      className="text-destructive focus:text-destructive"
                      onClick={() => setDeleteOpen(true)}
                    >
                      <Trash2 size={14} />
                      Delete project
                    </DropdownMenuItem>
                  </DropdownMenuContent>
                </DropdownMenu>
              )}
            </div>
          </div>
          {tags.length > 0 && (
            <div className="flex flex-wrap gap-1 pt-1">
              {tags.map((tag) => (
                <Badge key={tag} variant="outline" className="text-xs font-normal">
                  {tag}
                </Badge>
              ))}
            </div>
          )}
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

          <NavLink
            to={`/projects/${project.project_id}`}
            className="text-primary mt-auto text-sm hover:underline"
          >
            View project
          </NavLink>
        </CardContent>
      </Card>

      {isAdmin() && (
        <>
          <DeleteProjectDialog
            projectId={project.project_id}
            open={deleteOpen}
            onOpenChange={setDeleteOpen}
          />
          {editTagsOpen && (
            <EditTagsDialog
              projectId={project.project_id}
              currentTags={tags}
              open={editTagsOpen}
              onOpenChange={setEditTagsOpen}
            />
          )}
        </>
      )}
    </>
  )
}
