import { getJson } from './http'
import type { FileListResponse } from '@/types/file'

export function listFiles(): Promise<FileListResponse> {
  return getJson<FileListResponse>('/api/files')
}

export function assetFileURL(assetId: number): string {
  return `/api/assets/${assetId}/file`
}
