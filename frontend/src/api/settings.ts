import { getJson } from './http'

export interface AppConfig {
  schedulerEnabled: boolean
  schedulerTickSeconds: number
  storageDataDir: string
  githubApiBaseUrl: string
}

export function getAppConfig(): Promise<AppConfig> {
  return getJson<AppConfig>('/api/config')
}
