import { defineStore } from 'pinia'
import { shallowRef } from 'vue'

import { createAPIKey, deleteAPIKey, listAPIKeys } from '@/api/apikeys'
import type { APIKeyItem, CreatedAPIKey, CreateAPIKeyPayload } from '@/types/apikey'

export const useAPIKeysStore = defineStore('apikeys', () => {
  const items = shallowRef<APIKeyItem[]>([])
  const loading = shallowRef(false)
  const saving = shallowRef(false)
  const error = shallowRef<string | null>(null)

  async function refresh() {
    loading.value = true
    error.value = null
    try {
      const result = await listAPIKeys()
      items.value = result.items
    } catch (err) {
      error.value = err instanceof Error ? err.message : '加载 API Key 失败'
      throw err
    } finally {
      loading.value = false
    }
  }

  async function create(payload: CreateAPIKeyPayload): Promise<CreatedAPIKey> {
    saving.value = true
    try {
      const created = await createAPIKey(payload)
      items.value = [created, ...items.value]
      return created
    } finally {
      saving.value = false
    }
  }

  async function remove(id: number) {
    saving.value = true
    try {
      await deleteAPIKey(id)
      items.value = items.value.filter((item) => item.id !== id)
    } finally {
      saving.value = false
    }
  }

  return { items, loading, saving, error, refresh, create, remove }
})
