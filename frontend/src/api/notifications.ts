import { getJson, requestJson } from './http'
import type { NotificationItem, NotificationListResponse, NotificationPayload } from '@/types/notification'

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
