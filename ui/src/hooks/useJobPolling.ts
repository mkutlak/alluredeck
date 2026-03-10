import { useEffect } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { getJobStatus } from '@/api/reports'
import { invalidateProjectQueries } from '@/lib/query-keys'
import type { JobStatus } from '@/types/api'

const TERMINAL_STATUSES: JobStatus[] = ['completed', 'failed']

interface UseJobPollingResult {
  status: JobStatus | undefined
  output: string | undefined
  error: string | undefined
  isPolling: boolean
  isCompleted: boolean
  isFailed: boolean
}

export function useJobPolling(projectId: string, jobId: string | null): UseJobPollingResult {
  const queryClient = useQueryClient()

  const query = useQuery({
    queryKey: ['job-status', projectId, jobId],
    queryFn: () => getJobStatus(projectId, jobId!).then((r) => r.data),
    enabled: jobId !== null,
    refetchInterval: (query) => {
      const status = query.state.data?.status
      if (!status || TERMINAL_STATUSES.includes(status)) return false
      return 2000
    },
  })

  const status = query.data?.status
  const isCompleted = status === 'completed'
  const isFailed = status === 'failed'
  const isPolling = jobId !== null && !isCompleted && !isFailed

  // Invalidate all project queries when job completes
  useEffect(() => {
    if (isCompleted) {
      void invalidateProjectQueries(queryClient, projectId)
    }
  }, [isCompleted, projectId, queryClient])

  return {
    status,
    output: query.data?.output,
    error: query.data?.error,
    isPolling,
    isCompleted,
    isFailed,
  }
}
