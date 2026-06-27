import type { Repository } from './repository'
import type { Task } from './task'

export type AssetStatus =
  | 'pending'
  | 'skipped'
  | 'downloading'
  | 'downloaded'
  | 'verified'
  | 'failed'
  | 'deleted'
  | string

export interface Release {
  id: number
  repositoryId: number
  providerReleaseId: number
  tag: string
  name: string
  publishedAt: string | null
  body: string
  htmlUrl: string
  apiUrl: string
  isLatest: boolean
  isPinned: boolean
  syncStatus: string
  createdAt: string
  updatedAt: string
}

export interface Asset {
  id: number
  releaseId: number
  providerAssetId: number
  name: string
  size: number
  contentType: string
  downloadUrl: string
  browserDownloadUrl: string
  storagePath: string
  sha256: string
  status: AssetStatus
  errorMessage: string
  downloadedAt: string | null
  createdAt: string
  updatedAt: string
}

export interface ReleaseListResponse {
  items: Release[]
}

export interface AssetListResponse {
  items: Asset[]
}

export interface CheckReleaseResult {
  repository: Repository
  release: Release
  assets: Asset[]
}

export interface CheckAllReleaseResult {
  repository: Repository
  releases: number
  newReleases: number
  totalAssets: number
  pendingAssets: number
  skippedAssets: number
  task: Task
}

export interface AssetDownloadResult {
  asset: Asset
  task: Task
}

export interface SyncReleaseResult {
  repository: Repository
  release: Release
  assets: Asset[]
  task: Task
  checkTask: Task
  downloadResults: AssetDownloadResult[]
  failedAssets: Array<{
    assetId: number
    name: string
    error: string
  }>
}
