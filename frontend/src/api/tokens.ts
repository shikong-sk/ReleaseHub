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


// Token 健康检查结果
export interface TokenHealthResult {
  valid: boolean
  rateLimit?: {
    limit: number
    remaining: number
    resetAt: number
    used: number
  }
  error?: string
}

// Token Rate Limit 结果
export interface TokenRateLimitResult {
  limit: number
  remaining: number
  resetAt: number
  used: number
}

export function checkTokenHealth(id: number): Promise<TokenHealthResult> {
  return getJson<TokenHealthResult>(`/api/tokens/${id}/health`)
}

export function checkTokenRateLimit(id: number): Promise<TokenRateLimitResult | null> {
  return getJson<TokenRateLimitResult | null>(`/api/tokens/${id}/rate-limit`)
}
