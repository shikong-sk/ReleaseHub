import { getJson, requestJson } from './http'
import type { NotificationItem, NotificationListResponse, NotificationPayload, NotificationLog, NotificationLogListResponse } from '@/types/notification'

export function listNotifications(): Promise<NotificationListResponse> {
  return getJson<NotificationListResponse>('/api/notifications')
}

export function createNotification(payload: NotificationPayload): Promise<NotificationItem> {
  return requestJson<NotificationItem>('/api/notifications', {
    method: 'POST',
    body: payload
  })
}

export function getNotification(id: number): Promise<NotificationItem> {
  return getJson<NotificationItem>(`/api/notifications/${id}`)
}

export function updateNotification(id: number, payload: NotificationPayload): Promise<NotificationItem> {
  return requestJson<NotificationItem>(`/api/notifications/${id}`, {
    method: 'PATCH',
    body: payload
  })
}

export async function deleteNotification(id: number): Promise<void> {
  await requestJson<void>(`/api/notifications/${id}`, {
    method: 'DELETE'
  })
}

export function testNotification(id: number): Promise<{ status: string; message: string }> {
  return requestJson<{ status: string; message: string }>(`/api/notifications/${id}/test`, {
    method: 'POST'
  })
}

export function listNotificationLogs(notificationId: number, limit = 50): Promise<NotificationLogListResponse> {
  return getJson<NotificationLogListResponse>(`/api/notifications/${notificationId}/logs?limit=${limit}`)
}

export function listAllNotificationLogs(params?: { limit?: number; event?: string }): Promise<NotificationLogListResponse> {
  const sp = new URLSearchParams()
  if (params?.limit) sp.set('limit', String(params.limit))
  if (params?.event) sp.set('event', params.event)
  const qs = sp.toString()
  return getJson<NotificationLogListResponse>(`/api/notification-logs${qs ? '?' + qs : ''}`)
}
