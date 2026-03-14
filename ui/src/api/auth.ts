import { apiClient } from './client'
import type { ApiResponse, LoginData, LoginRequest, SessionData } from '@/types/api'

export async function login(credentials: LoginRequest): Promise<ApiResponse<LoginData>> {
  const res = await apiClient.post<ApiResponse<LoginData>>('/login', credentials)
  return res.data
}

export async function logout(): Promise<void> {
  await apiClient.delete('/logout')
}

export async function getSession(): Promise<ApiResponse<SessionData>> {
  const res = await apiClient.get<ApiResponse<SessionData>>('/auth/session')
  return res.data
}
