import { apiClient } from './client'
import type { ApiResponse, BuildFailedTest } from '@/types/api'

export async function fetchBuildFailedTests(
  projectId: number,
  buildId: number,
  limit = 50,
): Promise<BuildFailedTest[]> {
  const res = await apiClient.get<ApiResponse<BuildFailedTest[]>>(
    `/projects/${encodeURIComponent(projectId)}/builds/${encodeURIComponent(buildId)}/tests`,
    { params: { status: 'failed', limit } },
  )
  return res.data.data
}
