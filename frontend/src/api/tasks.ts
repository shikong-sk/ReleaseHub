import { getJson, requestJson } from './http'
import type { Task, TaskListResponse } from '@/types/task'

export interface TaskListParams {
  status?: string
  type?: string
  repositoryId?: number
  keyword?: string
  page?: number
  pageSize?: number
}

export function listTasks(params?: TaskListParams): Promise<TaskListResponse> {
  if (!params) return getJson<TaskListResponse>('/api/tasks')
  const qs = new URLSearchParams()
  for (const [k, v] of Object.entries(params)) {
    if (v !== undefined && v !== null && v !== '') qs.set(k, String(v))
  }
  const query = qs.toString()
  return getJson<TaskListResponse>(query ? `/api/tasks?${query}` : '/api/tasks')
}

export function getTask(id: number): Promise<Task> {
  return getJson<Task>(`/api/tasks/${id}`)
}

// 取消/停止指定任务（running 中断下载，pending 标记跳过）
export function cancelTask(id: number): Promise<{ ok: boolean }> {
  return requestJson(`/api/tasks/${id}/cancel`, { method: 'POST' })
}

// 清理所有失败状态的任务及其关联日志
export function clearFailedTasks(): Promise<{ deleted: number }> {
  return requestJson('/api/tasks/failed', { method: 'DELETE' })
}
