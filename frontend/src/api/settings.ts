import { getJson } from './http'

export interface AppConfig {
  schedulerEnabled: boolean
  schedulerTickSeconds: number
  schedulerMaxConcurrent: number
  storageDataDir: string
  githubApiBaseUrl: string
  authEnabled: boolean
}

export function getAppConfig(): Promise<AppConfig> {
  return getJson<AppConfig>('/api/config')
}
