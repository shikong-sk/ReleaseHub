import { getJson, requestJson } from './http'
import type { CheckAllReleaseResult, CheckReleaseResult } from '@/types/release'
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

export async function syncRepository(id: number): Promise<void> {
  await requestJson(`/api/repositories/${id}/sync`, {
    method: 'POST'
  })
}


export async function syncRepositoryByTag(id: number, tag: string): Promise<void> {
  await requestJson(`/api/repositories/${id}/sync-tag`, {
    method: 'POST',
    body: { tag }
  })
}


export async function listRemoteTags(id: number): Promise<string[]> {
  const resp = await getJson<{ tags: string[] }>(`/api/repositories/${id}/remote-tags`)
  return resp.tags
}
