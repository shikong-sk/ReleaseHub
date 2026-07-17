import { getJson } from './http'
import { requestJson } from './http'

export interface AppConfig {
  schedulerEnabled: boolean
  schedulerTickSeconds: number
  schedulerMaxConcurrent: number
  storageDataDir: string
  githubApiBaseUrl: string
  authEnabled: boolean
  syncerMaxConcurrentTasks: number
  syncerMaxConcurrentDownloads: number
  downloadMaxSpeedBytes: number
  aria2RPC: string
  aria2Secret: string
  aria2HTTP: string
  aria2Dir: string
  taskLogRetentionDays: number
  operationLogRetentionDays: number
}

export function getAppConfig(): Promise<AppConfig> {
  return getJson<AppConfig>('/api/config')
}

// 可运行时更新的配置项（与后端 UpdateConfig 对应）
export interface AppConfigUpdate {
  schedulerEnabled?: boolean
  schedulerTickSeconds?: number
  schedulerMaxConcurrent?: number
  githubApiBaseUrl?: string
  authEnabled?: boolean
  syncerMaxConcurrentTasks?: number
  syncerMaxConcurrentDownloads?: number
  downloadMaxSpeedBytes?: number
  aria2RPC?: string
  aria2Secret?: string
  aria2Dir?: string
  taskLogRetentionDays?: number
  operationLogRetentionDays?: number
}

export function updateAppConfig(update: AppConfigUpdate): Promise<AppConfig> {
  return requestJson<AppConfig>('/api/config', { method: 'PUT', body: update })
}
