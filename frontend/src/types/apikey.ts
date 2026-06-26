export interface APIKeyItem {
  id: number
  name: string
  keyHint: string
  scope: string
  enabled: boolean
  lastUsedAt: string | null
  createdAt: string
}

export interface CreateAPIKeyPayload {
  name: string
  scope?: string
}

export interface CreatedAPIKey extends APIKeyItem {
  key: string
}

export interface APIKeyListResponse {
  items: APIKeyItem[]
}
