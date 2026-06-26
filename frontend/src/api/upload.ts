import { requestJson } from './http'
import type { Asset } from '@/types/release'

export async function uploadAsset(repositoryId: number, releaseId: number, file: File): Promise<Asset> {
  const formData = new FormData()
  formData.append('file', file)
  formData.append('repository_id', String(repositoryId))
  formData.append('release_id', String(releaseId))

  const response = await fetch('/api/assets/upload', {
    method: 'POST',
    body: formData
  })

  if (!response.ok) {
    const payload = await response.json().catch(() => ({ error: `上传失败: HTTP ${response.status}` }))
    throw new Error(payload.error ?? `上传失败: HTTP ${response.status}`)
  }

  return response.json() as Promise<Asset>
}
