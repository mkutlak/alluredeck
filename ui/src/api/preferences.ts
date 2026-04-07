import { apiClient } from './client'
import type { ApiResponse } from '@/types/api'

export interface ServerPreferences {
  preferences: Record<string, unknown>
  updated_at: string
}

export async function fetchPreferences(): Promise<ApiResponse<ServerPreferences>> {
  const res = await apiClient.get<ApiResponse<ServerPreferences>>('/preferences')
  return res.data
}

export async function updatePreferences(
  preferences: Record<string, unknown>,
): Promise<ApiResponse<ServerPreferences>> {
  const res = await apiClient.put<ApiResponse<ServerPreferences>>('/preferences', { preferences })
  return res.data
}
