export type TaskStatus = 'pending' | 'running' | 'succeeded' | 'failed' | 'canceled' | string

export interface Task {
  id: number
  type: string
  repositoryId: number | null
  releaseId: number | null
  assetId: number | null
  status: TaskStatus
  priority: number
  attempt: number
  maxAttempts: number
  scheduledAt: string | null
  startedAt: string | null
  finishedAt: string | null
  errorMessage: string
  createdAt: string
  updatedAt: string
}

export interface TaskListResponse {
  items: Task[]
}
