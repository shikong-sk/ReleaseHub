import { getJson, requestJson } from './http'
import type { FileListResponse, FileTreeResponse } from '@/types/file'

export function listFiles(): Promise<FileListResponse> {
  return getJson<FileListResponse>('/api/files')
}

export function assetFileURL(assetId: number): string {
  return `/api/assets/${assetId}/file`
}

export function getFileTree(): Promise<FileTreeResponse> {
  return getJson<FileTreeResponse>('/api/files/tree')
}

export function getRepositoryFileTree(repositoryId: number, storageId?: number | null): Promise<FileTreeResponse> {
  let url = `/api/files/tree?repositoryId=${repositoryId}`
  if (storageId != null && storageId !== undefined && storageId > 0) {
    url += `&storageId=${storageId}`
  }
  return getJson<FileTreeResponse>(url)
}
