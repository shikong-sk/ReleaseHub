export interface HealthStatus {
  status: 'ok' | 'degraded' | string
  service: string
  checks: Record<string, string>
  checkedAt: string
}
