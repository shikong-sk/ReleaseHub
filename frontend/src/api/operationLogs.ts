import { getJson } from './http'

export interface OperationLogItem {
  id: number
  actor: string
  action: string
  resource: string
  detail: string
  status: string
  clientIp: string
  createdAt: string
}

export interface OperationLogListResponse {
  items: OperationLogItem[]
  total: number
  page: number
  pageSize: number
}

export interface OperationLogListParams {
  actor?: string
  action?: string
  status?: string
  keyword?: string
  page?: number
  pageSize?: number
}

export function listOperationLogs(params?: OperationLogListParams): Promise<OperationLogListResponse> {
  if (!params) return getJson<OperationLogListResponse>('/api/operation-logs')
  const qs = new URLSearchParams()
  for (const [k, v] of Object.entries(params)) {
    if (v !== undefined && v !== null && v !== '') qs.set(k, String(v))
  }
  const query = qs.toString()
  return getJson<OperationLogListResponse>(query ? `/api/operation-logs?${query}` : '/api/operation-logs')
}
