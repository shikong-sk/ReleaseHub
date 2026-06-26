import { getJson } from './http'

export interface TaskLogItem {
  id: number
  taskId: number
  level: string
  message: string
  timestamp: string
}

export function listTaskLogs(taskId: number, limit = 100): Promise<{ items: TaskLogItem[] }> {
  return getJson<{ items: TaskLogItem[] }>(`/api/tasks/${taskId}/logs?limit=${limit}`)
}
