import { useEffect } from 'react'
import { useParams } from 'react-router'
import { useQuery } from '@tanstack/react-query'
import { getProjects } from '@/api/projects'
import { useUIStore } from '@/store/ui'
import { queryKeys } from '@/lib/query-keys'

interface ActiveProjectResult {
  projectId: string | null
  isFromUrl: boolean
  isLoading: boolean
}

export function useActiveProject(): ActiveProjectResult {
  const { id: urlProjectId } = useParams<{ id: string }>()
  const lastProjectId = useUIStore((s) => s.lastProjectId)
  const setLastProjectId = useUIStore((s) => s.setLastProjectId)

  const { data, isLoading } = useQuery({
    queryKey: queryKeys.projects,
    queryFn: () => getProjects(),
    staleTime: 30_000,
    enabled: !urlProjectId,
  })

  useEffect(() => {
    if (urlProjectId) {
      setLastProjectId(urlProjectId)
    }
  }, [urlProjectId, setLastProjectId])

  if (urlProjectId) {
    return { projectId: urlProjectId, isFromUrl: true, isLoading: false }
  }

  if (lastProjectId) {
    return { projectId: lastProjectId, isFromUrl: false, isLoading: false }
  }

  const projects = data?.data ?? []
  const firstProject = projects[0]?.project_id ?? null

  return {
    projectId: firstProject,
    isFromUrl: false,
    isLoading: !firstProject && isLoading,
  }
}
