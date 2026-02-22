import { apiClient } from './client'
import type { ApiResponse, ConfigData, VersionData } from '@/types/api'

export async function getVersion(): Promise<ApiResponse<VersionData>> {
  const res = await apiClient.get<ApiResponse<VersionData>>('/version')
  return res.data
}

export async function getConfig(): Promise<ApiResponse<ConfigData>> {
  const res = await apiClient.get<ApiResponse<ConfigData>>('/config')
  return res.data
}
