import { getJson, requestJson } from './http'

export interface TokenItem {
  id: number
  name: string
  tokenHint: string
  createdAt: string
  updatedAt: string
}

export interface TokenListResponse {
  items: TokenItem[]
}

export function listTokens(): Promise<TokenListResponse> {
  return getJson<TokenListResponse>('/api/tokens')
}

export function createToken(payload: { name: string; token: string }): Promise<TokenItem> {
  return requestJson<TokenItem>('/api/tokens', {
    method: 'POST',
    body: payload
  })
}

export async function deleteToken(id: number): Promise<void> {
  await requestJson<void>(`/api/tokens/${id}`, {
    method: 'DELETE'
  })
}
