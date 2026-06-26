export interface FileItem {
  assetId: number
  releaseId: number
  repositoryId: number
  owner: string
  repo: string
  tag: string
  name: string
  size: number
  sha256: string
  storagePath: string
  downloadedAt: string
}

export interface FileListResponse {
  items: FileItem[]
}
