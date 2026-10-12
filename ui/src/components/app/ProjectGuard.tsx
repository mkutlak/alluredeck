import { useEffect, useRef } from 'react'
import { Navigate, Outlet, useLocation, useNavigate, useParams } from 'react-router'
import { Loader2 } from 'lucide-react'
import { useProjectFromParam } from '@/lib/resolveProject'
import { useUIStore } from '@/store/ui'
import { toast } from '@/components/ui/use-toast'

export function ProjectGuard() {
  const { id } = useParams<{ id: string }>()
  const { project, isLoading } = useProjectFromParam(id)
  const navigate = useNavigate()
  const { pathname, search, hash } = useLocation()
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
    void navigate('/', { replace: true })
  }, [isLoading, project, id, navigate, clearLastProjectId])

  if (isLoading) {
    return (
      <div className="flex h-64 items-center justify-center">
        <Loader2 className="text-muted-foreground h-8 w-8 animate-spin" />
      </div>
    )
  }

  if (!project) return null

  // A slug URL is not unique (child slugs repeat across groups) and the nested routes
  // read the param as an id, so move to the numeric URL, keeping the tab, query and hash.
  if (id !== String(project.project_id)) {
    const to = pathname.replace(/^\/projects\/[^/]+/, `/projects/${project.project_id}`)
    return <Navigate replace to={to + search + hash} />
  }

  return <Outlet />
}
