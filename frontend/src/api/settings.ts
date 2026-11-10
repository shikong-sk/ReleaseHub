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
  // 下载请求 User-Agent：preset 为预设标识，userAgent 为自定义值（仅 custom 生效）
  userAgentPreset: string
  userAgent: string
  // 内置 UA 预设列表（后端下发，前端不硬编码）
  userAgentPresets: UserAgentPreset[]
}

export interface UserAgentPreset {
  value: string
  label: string
  ua: string
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
  userAgentPreset?: string
  userAgent?: string
  taskLogRetentionDays?: number
  operationLogRetentionDays?: number
}

export function updateAppConfig(update: AppConfigUpdate): Promise<AppConfig> {
  return requestJson<AppConfig>('/api/config', { method: 'PUT', body: update })
}
