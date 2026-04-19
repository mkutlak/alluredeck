import { useEffect, useRef } from 'react'
import { Outlet, useNavigate, useParams } from 'react-router'
import { Loader2 } from 'lucide-react'
import { useProjectFromParam } from '@/lib/resolveProject'
import { useUIStore } from '@/store/ui'
import { toast } from '@/components/ui/use-toast'

export function ProjectGuard() {
  const { id } = useParams<{ id: string }>()
  const { project, isLoading } = useProjectFromParam(id)
  const navigate = useNavigate()
  const clearLastProjectId = useUIStore((s) => s.clearLastProjectId)
  const redirected = useRef(false)

  useEffect(() => {
    if (isLoading || project || !id || redirected.current) return

    redirected.current = true
    clearLastProjectId()
    toast({
      title: 'Project not found',
      description: `No project with id "${id}".`,
      variant: 'destructive',
    })
    navigate('/', { replace: true })
  }, [isLoading, project, id, navigate, clearLastProjectId])

  if (isLoading) {
    return (
      <div className="flex h-64 items-center justify-center">
        <Loader2 className="text-muted-foreground h-8 w-8 animate-spin" />
      </div>
    )
  }

  if (!project) return null

  return <Outlet />
}
