import { defineStore } from 'pinia'
import { computed, shallowRef } from 'vue'

import { listTasks, type TaskListParams } from '@/api/tasks'
import type { Task } from '@/types/task'

export const useTasksStore = defineStore('tasks', () => {
  // 全量统计快照（不受筛选影响，供统计面板使用）
  const stats = shallowRef<Task[]>([])
  const total = shallowRef<number>(0)

  // 筛选后的列表（供表格使用）
  const items = shallowRef<Task[]>([])
  const listTotal = shallowRef<number>(0)

  const loading = shallowRef(false)
  const error = shallowRef<string | null>(null)
  // 当前生效的筛选条件，refreshList 时携带
  const filters = shallowRef<TaskListParams>({})

  // 统计面板：始终基于全量快照，与筛选列表解耦
  const failedCount = computed(() => stats.value.filter((item) => item.status === 'failed').length)
  const runningCount = computed(() => stats.value.filter((item) => item.status === 'running').length)
  const pendingCount = computed(() => stats.value.filter((item) => item.status === 'pending').length)

  // 刷新全量统计（不带筛选），供统计面板使用
  async function refreshStats() {
    try {
      const resp = await listTasks({ pageSize: 500 })
      stats.value = resp.items
      total.value = resp.total ?? resp.items.length
    } catch {
      // 统计刷新失败不阻塞，沿用上次快照
    }
  }

  // 刷新筛选列表，仅在筛选变化时调用
  async function refreshList() {
    loading.value = true
    error.value = null
    try {
      const response = await listTasks(filters.value)
      items.value = response.items
      listTotal.value = response.total ?? response.items.length
    } catch (err) {
      error.value = err instanceof Error ? err.message : '任务列表加载失败'
    } finally {
      loading.value = false
    }
  }

  // 刷新：全量统计 + 筛选列表。
  // silent 为 true（定时刷新）时不重置 loading，避免表格闪烁。
  async function refresh(options: { silent?: boolean } = {}) {
    if (!options.silent) {
      loading.value = true
    }
    error.value = null
    try {
      await Promise.all([refreshStats(), refreshList()])
    } finally {
      if (!options.silent) {
        loading.value = false
      }
    }
  }

  function setFilters(next: TaskListParams) {
    filters.value = { ...next }
  }

  return {
    items,
    total,
    listTotal,
    loading,
    error,
    failedCount,
    runningCount,
    pendingCount,
    filters,
    refresh,
    refreshList,
    setFilters
  }
})
