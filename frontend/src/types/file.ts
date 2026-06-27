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
  storageId: number | null
  storageName: string
  storageType: string
}

export interface FileTreeNode {
  key: string
  label: string
  isLeaf: boolean
  prefix?: string
  children?: FileTreeNode[]

  // 存储层附加字段
  storageId?: number

  // 仓库层附加字段
  repositoryId?: number
  fileCount?: number

  // 版本层附加字段
  releaseId?: number

  // 文件叶节点附加字段
  status?: string
  assetId?: number
  size?: number
  sha256?: string
  storagePath?: string
  downloadedAt?: string
}

export interface FileTreeResponse {
  tree: FileTreeNode[]
}

export interface FileListResponse {
  items: FileItem[]
}
