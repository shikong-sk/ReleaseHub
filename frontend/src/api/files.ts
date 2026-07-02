import { getJson, requestJson, downloadFile } from './http'
import type { FileListResponse, FileTreeResponse } from '@/types/file'

export function listFiles(): Promise<FileListResponse> {
  return getJson<FileListResponse>('/api/files')
}

export function assetFileURL(assetId: number): string {
  return `/api/assets/${assetId}/file`
}

// 下载资产文件（带认证头，解决 <a href> 不带 Authorization 导致 401 的问题）
export async function downloadAssetFile(assetId: number, filename?: string): Promise<void> {
  await downloadFile(assetFileURL(assetId), filename)
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
