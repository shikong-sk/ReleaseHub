import { defineStore } from 'pinia'
import { shallowRef } from 'vue'

import { createStorage, deleteStorage, listStorages, testStorageConnection, updateStorage } from '@/api/storages'
import type { StorageItem, StoragePayload } from '@/types/storage'

export const useStoragesStore = defineStore('storages', () => {
  const items = shallowRef<StorageItem[]>([])
  const loading = shallowRef(false)
  const saving = shallowRef(false)
  const error = shallowRef<string | null>(null)

  async function refresh() {
    loading.value = true
    error.value = null
    try {
      const response = await listStorages()
      items.value = response.items
    } catch (err) {
      error.value = err instanceof Error ? err.message : '存储列表加载失败'
    } finally {
      loading.value = false
    }
  }

  async function create(payload: StoragePayload) {
    saving.value = true
    error.value = null
    try {
      const item = await createStorage(payload)
      items.value = [item, ...items.value]
      return item
    } finally {
      saving.value = false
    }
  }

  async function update(id: number, payload: Partial<StoragePayload>) {
    saving.value = true
    error.value = null
    try {
      const item = await updateStorage(id, payload)
      items.value = items.value.map((s) => (s.id === id ? item : s))
      return item
    } finally {
      saving.value = false
    }
  }

  async function remove(id: number) {
    saving.value = true
    error.value = null
    try {
      await deleteStorage(id)
      items.value = items.value.filter((s) => s.id !== id)
    } finally {
      saving.value = false
    }
  }

  async function testConnection(id: number) {
    return testStorageConnection(id)
  }

  return { items, loading, saving, error, refresh, create, update, remove, testConnection }
})
