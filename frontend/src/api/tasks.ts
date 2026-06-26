import { getJson } from './http'
import type { Task, TaskListResponse } from '@/types/task'

export function listTasks(): Promise<TaskListResponse> {
  return getJson<TaskListResponse>('/api/tasks')
}

export function getTask(id: number): Promise<Task> {
  return getJson<Task>(`/api/tasks/${id}`)
}
