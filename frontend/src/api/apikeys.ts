import { getJson, requestJson } from './http'
import type { APIKeyListResponse, CreatedAPIKey, CreateAPIKeyPayload } from '@/types/apikey'

export function listAPIKeys(): Promise<APIKeyListResponse> {
  return getJson<APIKeyListResponse>('/api/apikeys')
}

export function createAPIKey(payload: CreateAPIKeyPayload): Promise<CreatedAPIKey> {
  return requestJson<CreatedAPIKey>('/api/apikeys', {
    method: 'POST',
    body: payload
  })
}

export async function deleteAPIKey(id: number): Promise<void> {
  await requestJson<void>(`/api/apikeys/${id}`, {
    method: 'DELETE'
  })
}
