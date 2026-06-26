import { getJson, requestJson } from './http'

export interface LoginPayload {
  username: string
  password: string
}

export interface UserInfo {
  id: number
  username: string
  role: string
  enabled: boolean
  lastLoginAt: string | null
  createdAt: string
}

export interface LoginResult {
  token: string
  user: UserInfo
  expireAt: string
}

export function login(payload: LoginPayload): Promise<LoginResult> {
  return requestJson<LoginResult>('/api/auth/login', {
    method: 'POST',
    body: payload
  })
}

export function getMe(): Promise<UserInfo> {
  return getJson<UserInfo>('/api/auth/me')
}

export function changePassword(oldPassword: string, newPassword: string): Promise<{ message: string }> {
  return requestJson<{ message: string }>('/api/auth/change-password', {
    method: 'POST',
    body: { oldPassword, newPassword }
  })
}

export function listUsers(): Promise<{ items: UserInfo[] }> {
  return getJson<{ items: UserInfo[] }>('/api/users')
}

export function createUser(payload: { username: string; password: string; role: string }): Promise<UserInfo> {
  return requestJson<UserInfo>('/api/users', {
    method: 'POST',
    body: payload
  })
}

export function updateUser(id: number, payload: { role?: string; enabled?: boolean }): Promise<UserInfo> {
  return requestJson<UserInfo>(`/api/users/${id}`, {
    method: 'PATCH',
    body: payload
  })
}

export async function deleteUser(id: number): Promise<void> {
  await requestJson<void>(`/api/users/${id}`, {
    method: 'DELETE'
  })
}
