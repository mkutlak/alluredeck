import { apiClient } from './client'
import type { ApiResponse, ConfigData } from '@/types/api'

export async function getConfig(): Promise<ApiResponse<ConfigData>> {
  const res = await apiClient.get<ApiResponse<ConfigData>>('/config')
  return res.data
}
