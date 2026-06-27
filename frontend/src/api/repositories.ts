import { getJson, requestJson } from './http'
import type { CheckAllReleaseResult, CheckReleaseResult, SyncReleaseResult } from '@/types/release'
import type { Repository, RepositoryListResponse, RepositoryPayload } from '@/types/repository'

export function listRepositories(): Promise<RepositoryListResponse> {
  return getJson<RepositoryListResponse>('/api/repositories')
}

export function createRepository(payload: RepositoryPayload): Promise<Repository> {
  return requestJson<Repository>('/api/repositories', {
    method: 'POST',
    body: payload
  })
}

export function updateRepository(id: number, payload: Partial<RepositoryPayload>): Promise<Repository> {
  return requestJson<Repository>(`/api/repositories/${id}`, {
    method: 'PATCH',
    body: payload
  })
}

export async function deleteRepository(id: number): Promise<void> {
  await requestJson<void>(`/api/repositories/${id}`, {
    method: 'DELETE'
  })
}

export function checkRepository(id: number): Promise<CheckReleaseResult> {
  return requestJson<CheckReleaseResult>(`/api/repositories/${id}/check`, {
    method: 'POST'
  })
}

export function checkAllRepository(id: number): Promise<CheckAllReleaseResult> {
  return requestJson<CheckAllReleaseResult>(`/api/repositories/${id}/check-all`, {
    method: 'POST'
  })
}

export function syncRepository(id: number): Promise<SyncReleaseResult> {
  return requestJson<SyncReleaseResult>(`/api/repositories/${id}/sync`, {
    method: 'POST'
  })
}


export function syncRepositoryByTag(id: number, tag: string): Promise<SyncReleaseResult> {
  return requestJson<SyncReleaseResult>(`/api/repositories/${id}/sync-tag`, {
    method: 'POST',
    body: { tag }
  })
}
