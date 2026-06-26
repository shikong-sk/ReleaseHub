export type NotificationType = 'gotify' | 'webhook' | 'telegram' | 'email'

export interface NotificationItem {
  id: number
  name: string
  type: NotificationType
  serverUrl: string
  tokenHint: string
  enabled: boolean
  events: string
  createdAt: string
  updatedAt: string
}

export interface NotificationListResponse {
  items: NotificationItem[]
}

export interface NotificationPayload {
  name: string
  type: NotificationType
  serverUrl?: string
  token?: string
  enabled?: boolean
  events?: string
}
