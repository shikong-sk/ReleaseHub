import { getJson, requestJson } from './http'
import type {
  AssetDownloadResult,
  AssetListResponse,
  Release,
  ReleaseListResponse
} from '@/types/release'

export function listRepositoryReleases(repositoryId: number): Promise<ReleaseListResponse> {
  return getJson<ReleaseListResponse>(`/api/repositories/${repositoryId}/releases`)
}

export function getRelease(id: number): Promise<Release> {
  return getJson<Release>(`/api/releases/${id}`)
}

export function listReleaseAssets(releaseId: number): Promise<AssetListResponse> {
  return getJson<AssetListResponse>(`/api/releases/${releaseId}/assets`)
}

export function downloadAsset(assetId: number): Promise<AssetDownloadResult> {
  return requestJson<AssetDownloadResult>(`/api/assets/${assetId}/download`, {
    method: 'POST'
  })
}
