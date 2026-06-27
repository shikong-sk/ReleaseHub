import { defineStore } from 'pinia'
import { shallowRef } from 'vue'

import { listProxies, createProxy, updateProxy, deleteProxy, testProxyConnection } from '@/api/proxies'
import type { ProxyItem, ProxyPayload } from '@/types/proxy'

export const useProxiesStore = defineStore('proxies', () => {
  const items = shallowRef<ProxyItem[]>([])
  const loading = shallowRef(false)
  const saving = shallowRef(false)
  const error = shallowRef<string | null>(null)

  async function refresh() {
    loading.value = true
    error.value = null
    try {
      const response = await listProxies()
      items.value = response.items
    } catch (err) {
      error.value = err instanceof Error ? err.message : '代理列表加载失败'
    } finally {
      loading.value = false
    }
  }

  async function create(payload: ProxyPayload) {
    saving.value = true
    error.value = null
    try {
      const item = await createProxy(payload)
      items.value = [item, ...items.value]
      return item
    } finally {
      saving.value = false
    }
  }

  async function update(id: number, payload: ProxyPayload) {
    saving.value = true
    error.value = null
    try {
      const item = await updateProxy(id, payload)
      items.value = items.value.map((p) => (p.id === id ? item : p))
      return item
    } finally {
      saving.value = false
    }
  }

  async function remove(id: number) {
    saving.value = true
    error.value = null
    try {
      await deleteProxy(id)
      items.value = items.value.filter((p) => p.id !== id)
    } finally {
      saving.value = false
    }
  }

  async function testConnection(id: number, testUrl?: string) {
    try {
      const result = await testProxyConnection(id, testUrl)
      if (result.proxy) {
        items.value = items.value.map((p) => (p.id === id ? result.proxy as ProxyItem : p))
      } else {
        await refresh()
      }
      return result
    } catch (err) {
      await refresh()
      throw err
    }
  }

  return { items, loading, saving, error, refresh, create, update, remove, testConnection }
})
