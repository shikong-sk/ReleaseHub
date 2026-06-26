import { defineStore } from 'pinia'
import { computed, shallowRef } from 'vue'

import { listTasks } from '@/api/tasks'
import type { Task } from '@/types/task'

export const useTasksStore = defineStore('tasks', () => {
  const items = shallowRef<Task[]>([])
  const loading = shallowRef(false)
  const error = shallowRef<string | null>(null)

  const failedCount = computed(() => items.value.filter((item) => item.status === 'failed').length)
  const runningCount = computed(() => items.value.filter((item) => item.status === 'running').length)

  async function refresh() {
    loading.value = true
    error.value = null

    try {
      const response = await listTasks()
      items.value = response.items
    } catch (err) {
      error.value = err instanceof Error ? err.message : '任务列表加载失败'
    } finally {
      loading.value = false
    }
  }

  return {
    items,
    loading,
    error,
    failedCount,
    runningCount,
    refresh
  }
})
