import { defineStore } from 'pinia'
import { shallowRef } from 'vue'

import { listNotifications, createNotification, updateNotification, deleteNotification, testNotification } from '@/api/notifications'
import type { NotificationItem, NotificationPayload } from '@/types/notification'

export const useNotificationsStore = defineStore('notifications', () => {
  const items = shallowRef<NotificationItem[]>([])
  const loading = shallowRef(false)
  const saving = shallowRef(false)
  const error = shallowRef<string | null>(null)

  async function refresh() {
    loading.value = true
    error.value = null
    try {
      const response = await listNotifications()
      items.value = response.items
    } catch (err) {
      error.value = err instanceof Error ? err.message : '通知列表加载失败'
    } finally {
      loading.value = false
    }
  }

  async function create(payload: NotificationPayload) {
    saving.value = true
    error.value = null
    try {
      const item = await createNotification(payload)
      items.value = [item, ...items.value]
      return item
    } finally {
      saving.value = false
    }
  }

  async function update(id: number, payload: NotificationPayload) {
    saving.value = true
    error.value = null
    try {
      const item = await updateNotification(id, payload)
      items.value = items.value.map((n) => (n.id === id ? item : n))
      return item
    } finally {
      saving.value = false
    }
  }

  async function remove(id: number) {
    saving.value = true
    error.value = null
    try {
      await deleteNotification(id)
      items.value = items.value.filter((n) => n.id !== id)
    } finally {
      saving.value = false
    }
  }

  async function testSend(id: number) {
    return testNotification(id)
  }

  return { items, loading, saving, error, refresh, create, update, remove, testSend }
})
