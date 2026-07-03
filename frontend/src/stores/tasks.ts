import { defineStore } from 'pinia'
import { computed, shallowRef } from 'vue'

import { listTasks } from '@/api/tasks'
import type { Task } from '@/types/task'

export const useTasksStore = defineStore('tasks', () => {
  const items = shallowRef<Task[]>([])
  const loading = shallowRef(false)
  const error = shallowRef<string | null>(null)
  // 后端返回的全表任务总数（items 最多 200 条，total 才是真实总数）
  const total = shallowRef<number>(0)

  const failedCount = computed(() => items.value.filter((item) => item.status === 'failed').length)
  const runningCount = computed(() => items.value.filter((item) => item.status === 'running').length)
  const pendingCount = computed(() => items.value.filter((item) => item.status === 'pending').length)

  async function refresh(options: { silent?: boolean } = {}) {
    if (!options.silent) {
      loading.value = true
    }
    error.value = null

    try {
      const response = await listTasks()
      items.value = response.items
      total.value = response.total ?? response.items.length
    } catch (err) {
      error.value = err instanceof Error ? err.message : '任务列表加载失败'
    } finally {
      if (!options.silent) {
        loading.value = false
      }
    }
  }

  return {
    items,
    total,
    loading,
    error,
    failedCount,
    runningCount,
    pendingCount,
    refresh
  }
})
