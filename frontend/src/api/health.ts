import { getJson } from './http'
import type { HealthStatus } from '@/types/health'

export function fetchHealth(): Promise<HealthStatus> {
  return getJson<HealthStatus>('/api/health')
}
