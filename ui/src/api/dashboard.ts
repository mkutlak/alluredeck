import { apiClient } from './client'
import type { ApiResponse, DashboardData } from '@/types/api'

export async function fetchDashboard(tag?: string): Promise<DashboardData> {
  const res = await apiClient.get<ApiResponse<DashboardData>>('/dashboard', {
    params: tag ? { tag } : undefined,
  })
  return res.data.data
}
