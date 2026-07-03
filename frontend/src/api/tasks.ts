import { getJson } from './http'
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
