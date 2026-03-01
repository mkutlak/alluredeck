import { useState } from 'react'
import { NavLink } from 'react-router'
import { MoreHorizontal, Trash2 } from 'lucide-react'
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
  const { latest_build, sparkline } = project
  const passRate = latest_build?.pass_rate ?? 0

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
                      ? 'bg-green-600 text-white hover:bg-green-700'
                      : passRate >= 70
                        ? 'bg-amber-500 text-white hover:bg-amber-600'
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
        </CardHeader>
        <CardContent className="flex flex-1 flex-col gap-3">
          {sparkline.length > 0 && <PassRateSparkline data={sparkline} />}

          {latest_build ? (
            <div className="space-y-1 text-sm text-muted-foreground">
              <div className="flex justify-between">
                <span>Tests</span>
                <span className="font-medium text-foreground">{latest_build.statistics.total}</span>
              </div>
              {latest_build.statistics.failed + latest_build.statistics.broken > 0 && (
                <div className="flex justify-between">
                  <span>Failures</span>
                  <span className="font-medium text-destructive">
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
                <span className="font-medium text-foreground">
                  {formatDuration(latest_build.duration_ms)}
                </span>
              </div>
              <div className="flex justify-between">
                <span>Last run</span>
                <span className="font-medium text-foreground">
                  {formatDate(latest_build.created_at)}
                </span>
              </div>
              {latest_build.ci_branch && (
                <div className="flex justify-between">
                  <span>Branch</span>
                  <span className="font-medium text-foreground">{latest_build.ci_branch}</span>
                </div>
              )}
            </div>
          ) : (
            <p className="text-sm text-muted-foreground">No runs yet</p>
          )}

          <NavLink
            to={`/projects/${project.project_id}`}
            className="mt-auto text-sm text-primary hover:underline"
          >
            View project
          </NavLink>
        </CardContent>
      </Card>

      {isAdmin() && (
        <DeleteProjectDialog
          projectId={project.project_id}
          open={deleteOpen}
          onOpenChange={setDeleteOpen}
        />
      )}
    </>
  )
}
