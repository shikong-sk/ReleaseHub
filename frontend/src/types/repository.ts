export type RepositoryStatus = 'unknown' | 'healthy' | 'failed' | string
export type RepositoryFilterMode = 'glob' | 'regex'

export interface Repository {
  id: number
  provider: string
  owner: string
  repo: string
  enabled: boolean
  githubTokenId: number | null
  storageId: number | null
  storageIds: number[]
  proxyId: number | null
  providerApiBaseUrl: string
  intervalSeconds: number
  filterMode: RepositoryFilterMode
  assetIncludePatterns: string
  assetExcludePatterns: string
  tagFilterMode: string
  tagIncludePattern: string
  tagExcludePattern: string
  retentionKeepLatest: number
  lastCheckAt: string | null
  lastReleaseTag: string
  lastStatus: RepositoryStatus
  createdAt: string
  updatedAt: string
}

export interface RepositoryListResponse {
  items: Repository[]
}

export interface RepositoryPayload {
  owner: string
  repo: string
  githubTokenId?: number | null
  storageId?: number | null
  storageIds?: number[]
  proxyId?: number | null
  providerApiBaseUrl?: string
  enabled?: boolean
  provider?: string
  intervalSeconds: number
  filterMode: RepositoryFilterMode
  assetIncludePatterns: string
  assetExcludePatterns: string
  tagFilterMode?: string
  tagIncludePattern?: string
  tagExcludePattern?: string
  retentionKeepLatest: number
}

export type RepositoryFormMode = 'create' | 'edit'
