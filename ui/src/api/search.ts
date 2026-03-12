import { apiClient } from './client'
import type { ApiResponse, SearchData } from '@/types/api'

export async function search(params: {
  q: string
  limit?: number
}): Promise<ApiResponse<SearchData>> {
  const res = await apiClient.get<ApiResponse<SearchData>>('/search', {
    params: {
      q: params.q,
      ...(params.limit !== undefined ? { limit: params.limit } : {}),
    },
  })
  return res.data
}
