import { defineStore } from 'pinia'
import { shallowRef } from 'vue'

import { createToken, deleteToken, listTokens } from '@/api/tokens'
import type { TokenItem } from '@/types/token'

export const useTokensStore = defineStore('tokens', () => {
  const items = shallowRef<TokenItem[]>([])
  const loading = shallowRef(false)
  const saving = shallowRef(false)
  const error = shallowRef<string | null>(null)

  async function refresh() {
    loading.value = true
    error.value = null
    try {
      const response = await listTokens()
      items.value = response.items
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Token 列表加载失败'
    } finally {
      loading.value = false
    }
  }

  async function create(payload: { name: string; token: string }) {
    saving.value = true
    error.value = null
    try {
      const token = await createToken(payload)
      items.value = [token, ...items.value]
    } finally {
      saving.value = false
    }
  }

  async function remove(id: number) {
    saving.value = true
    error.value = null
    try {
      await deleteToken(id)
      items.value = items.value.filter((item) => item.id !== id)
    } finally {
      saving.value = false
    }
  }

  return { items, loading, saving, error, refresh, create, remove }
})
