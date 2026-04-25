import { apiClient } from './client'
import type {
  ApiResponse,
  User,
  UserListResponse,
  CreateUserRequest,
  UpdateUserRequest,
  CreateUserResponse,
  UserRole,
  ChangePasswordRequest,
  ResetPasswordResponse,
} from '@/types/api'

export interface FetchUsersParams {
  limit?: number
  offset?: number
  search?: string
  role?: UserRole | ''
  active?: boolean | null
}

export async function fetchUsers(params: FetchUsersParams = {}): Promise<UserListResponse> {
  const res = await apiClient.get<ApiResponse<UserListResponse>>('/users', {
    params: params as Record<string, unknown>,
  })
  return res.data.data
}

export async function fetchUser(id: number): Promise<User> {
  const res = await apiClient.get<ApiResponse<User>>(`/users/${id}`)
  return res.data.data
}

export async function createUser(body: CreateUserRequest): Promise<CreateUserResponse> {
  const res = await apiClient.post<ApiResponse<CreateUserResponse>>('/users', body)
  return res.data.data
}

export async function updateUserRole(id: number, role: UserRole): Promise<User> {
  const body: UpdateUserRequest = { role }
  const res = await apiClient.patch<ApiResponse<User>>(`/users/${id}`, body)
  return res.data.data
}

export async function updateUserActive(id: number, active: boolean): Promise<User> {
  const body: UpdateUserRequest = { active }
  const res = await apiClient.patch<ApiResponse<User>>(`/users/${id}`, body)
  return res.data.data
}

export async function deactivateUser(id: number): Promise<void> {
  await apiClient.delete(`/users/${id}`)
}

export async function fetchMe(): Promise<User> {
  const res = await apiClient.get<ApiResponse<User>>('/users/me')
  return res.data.data
}

export async function updateMe(body: Pick<UpdateUserRequest, 'name'>): Promise<User> {
  const res = await apiClient.patch<ApiResponse<User>>('/users/me', body)
  return res.data.data
}

export async function changeMyPassword(body: ChangePasswordRequest): Promise<void> {
  await apiClient.post('/users/me/password', body)
}

export async function resetUserPassword(id: number): Promise<ResetPasswordResponse> {
  const res = await apiClient.post<ApiResponse<ResetPasswordResponse>>(`/users/${id}/password`)
  return res.data.data
}
