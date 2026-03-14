import { apiClient } from './client'
import type { ApiResponse, APIKey, APIKeyCreated, CreateAPIKeyRequest } from '@/types/api'

export async function fetchAPIKeys(): Promise<APIKey[]> {
  const res = await apiClient.get<ApiResponse<APIKey[]>>('/api-keys')
  return res.data.data
}

export async function createAPIKey(
  data: CreateAPIKeyRequest,
): Promise<{ apiKey: APIKeyCreated; message: string }> {
  const res = await apiClient.post<ApiResponse<APIKeyCreated>>('/api-keys', data)
  return { apiKey: res.data.data, message: res.data.metadata.message }
}

export async function deleteAPIKey(id: number): Promise<void> {
  await apiClient.delete(`/api-keys/${id}`)
}
