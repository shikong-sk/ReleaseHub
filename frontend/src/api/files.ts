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

export function getRepositoryFileTree(repositoryId: number): Promise<FileTreeResponse> {
  return getJson<FileTreeResponse>(`/api/files/tree?repositoryId=${repositoryId}`)
}
