export type ProxyType = 'http' | 'https' | 'socks5'

export interface ProxyItem {
  id: number
  name: string
  type: ProxyType
  host: string
  port: number
  username: string
  testUrl: string
  lastTestStatus: string
  lastTestMessage: string
  lastTestLatencyMs: number
  lastTestedAt: string
  createdAt: string
  updatedAt: string
}

export interface ProxyListResponse {
  items: ProxyItem[]
}

export interface ProxyPayload {
  name: string
  type: ProxyType
  host: string
  port: number
  username?: string
  password?: string
  testUrl?: string
}

export interface ProxyTestResponse {
  status: string
  message: string
  proxy?: ProxyItem
}
