import { apiClient } from './client'
import type { ApiResponse, DashboardData } from '@/types/api'

export async function fetchDashboard(): Promise<DashboardData> {
  const res = await apiClient.get<ApiResponse<DashboardData>>('/dashboard')
  return res.data.data
}
