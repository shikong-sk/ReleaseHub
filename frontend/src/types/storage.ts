export type StorageType = 'local' | 's3' | 'webdav'

export interface StorageItem {
  id: number
  name: string
  type: StorageType
  basePath: string
  isDefault: boolean
  endpoint: string
  bucket: string
  region: string
  accessKeyHint: string
  username: string
  remoteUrl: string
  createdAt: string
  updatedAt: string
}

export interface StorageListResponse {
  items: StorageItem[]
}

export interface StoragePayload {
  name: string
  type: StorageType
  basePath?: string
  isDefault?: boolean
  endpoint?: string
  bucket?: string
  region?: string
  accessKey?: string
  secretKey?: string
  username?: string
  password?: string
  remoteUrl?: string
}
