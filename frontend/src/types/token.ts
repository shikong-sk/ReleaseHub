export interface TokenItem {
  id: number
  name: string
  tokenHint: string
  createdAt: string
  updatedAt: string
}

export type TokenListResponse = {
  items: TokenItem[]
}
