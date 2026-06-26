export type ProxyType = 'http' | 'https' | 'socks5'

export interface ProxyItem {
  id: number
  name: string
  type: ProxyType
  host: string
  port: number
  username: string
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
}
