import { useEffect } from 'react'
import { useParams } from 'react-router'
import { useQuery } from '@tanstack/react-query'
import { useUIStore } from '@/store/ui'
import { projectIndexOptions } from '@/lib/queries'

interface ActiveProjectResult {
  projectId: string | null
  isFromUrl: boolean
  isLoading: boolean
}

export function useActiveProject(): ActiveProjectResult {
  const { id: urlProjectId } = useParams<{ id: string }>()
  const lastProjectId = useUIStore((s) => s.lastProjectId)
  const setLastProjectId = useUIStore((s) => s.setLastProjectId)

  const { data, isLoading } = useQuery({ ...projectIndexOptions(), enabled: !urlProjectId })

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
  const firstProject = projects[0] != null ? projects[0].slug : null

  return {
    projectId: firstProject,
    isFromUrl: false,
    isLoading: !firstProject && isLoading,
  }
}
