export type TaskStatus = 'pending' | 'running' | 'succeeded' | 'failed' | 'canceled' | string

export interface Task {
  id: number
  type: string
  repositoryId: number | null
  repositoryName: string
  releaseId: number | null
  releaseTag: string
  assetId: number | null
  assetName: string
  storagePath: string
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
  total: number
}
