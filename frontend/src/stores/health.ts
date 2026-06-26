import { defineStore } from 'pinia'
import { computed, shallowRef } from 'vue'

import { fetchHealth } from '@/api/health'
import type { HealthStatus } from '@/types/health'

export const useHealthStore = defineStore('health', () => {
  const status = shallowRef<HealthStatus | null>(null)
  const loading = shallowRef(false)
  const error = shallowRef<string | null>(null)

  const isHealthy = computed(() => status.value?.status === 'ok')
  const databaseStatus = computed(() => status.value?.checks.database ?? 'unknown')

  async function refresh() {
    loading.value = true
    error.value = null

    try {
      status.value = await fetchHealth()
    } catch (err) {
      error.value = err instanceof Error ? err.message : '健康检查请求失败'
    } finally {
      loading.value = false
    }
  }

  return {
    status,
    loading,
    error,
    isHealthy,
    databaseStatus,
    refresh
  }
})
