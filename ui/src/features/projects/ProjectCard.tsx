import { useState } from 'react'
import { Link } from 'react-router'
import { FolderOpen, Trash2, MoreHorizontal } from 'lucide-react'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { DeleteProjectDialog } from './DeleteProjectDialog'
import { useAuthStore } from '@/store/auth'

interface ProjectCardProps {
  projectId: string
}

export function ProjectCard({ projectId }: ProjectCardProps) {
  const isAdmin = useAuthStore((s) => s.isAdmin)
  const [deleteOpen, setDeleteOpen] = useState(false)

  return (
    <>
      <Card className="group relative transition-shadow hover:shadow-md">
        <CardHeader className="pb-2">
          <div className="flex items-start justify-between gap-2">
            <div className="flex min-w-0 items-center gap-2">
              <FolderOpen size={16} className="text-muted-foreground shrink-0" />
              <CardTitle className="truncate text-sm font-medium">{projectId}</CardTitle>
            </div>
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
        </CardHeader>
        <CardContent>
          <Badge variant="secondary" className="font-mono text-xs">
            {projectId}
          </Badge>
          <div className="mt-4">
            <Button asChild size="sm" variant="outline" className="w-full">
              <Link to={`/projects/${projectId}`}>View reports</Link>
            </Button>
          </div>
        </CardContent>
      </Card>

      {isAdmin() && (
        <DeleteProjectDialog projectId={projectId} open={deleteOpen} onOpenChange={setDeleteOpen} />
      )}
    </>
  )
}
