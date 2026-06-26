import { getJson, requestJson } from './http'
import type { StorageItem, StorageListResponse, StoragePayload } from '@/types/storage'

export function listStorages(): Promise<StorageListResponse> {
  return getJson<StorageListResponse>('/api/storages')
}

export function createStorage(payload: StoragePayload): Promise<StorageItem> {
  return requestJson<StorageItem>('/api/storages', {
    method: 'POST',
    body: payload
  })
}

export function updateStorage(id: number, payload: Partial<StoragePayload>): Promise<StorageItem> {
  return requestJson<StorageItem>(`/api/storages/${id}`, {
    method: 'PATCH',
    body: payload
  })
}

export async function deleteStorage(id: number): Promise<void> {
  await requestJson<void>(`/api/storages/${id}`, {
    method: 'DELETE'
  })
}

export function testStorageConnection(id: number): Promise<{ status: string; message: string }> {
  return requestJson<{ status: string; message: string }>(`/api/storages/${id}/test`, {
    method: 'POST'
  })
}
