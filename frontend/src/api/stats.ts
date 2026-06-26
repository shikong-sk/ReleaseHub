import { getJson } from './http'

export interface DashboardStats {
  totalRepositories: number
  enabledRepositories: number
  healthyRepositories: number
  failedRepositories: number
  totalReleases: number
  totalAssets: number
  downloadedAssets: number
  failedAssets: number
  totalStorageBytes: number
  pendingTasks: number
  runningTasks: number
  failedTasks: number
}

export interface TrendPoint {
  date: string
  count: number
}

export interface TrendStats {
  releases: TrendPoint[]
  assets: TrendPoint[]
}

export function getDashboardStats(): Promise<DashboardStats> {
  return getJson<DashboardStats>('/api/stats/dashboard')
}

export function getTrendStats(days = 30): Promise<TrendStats> {
  return getJson<TrendStats>(`/api/stats/trend?days=${days}`)
}
