import { getJson } from './http'
import type { Repository } from '@/types/repository'
import type { Release } from '@/types/release'
import type { Asset, AssetStatus } from '@/types/release'

export interface SearchParams {
  q: string
  limit?: number
  repositoryId?: number
  status?: AssetStatus
  dateFrom?: string
  dateTo?: string
}

export interface SearchResult {
  repositories: Repository[]
  releases: Release[]
  assets: Asset[]
  total: number
}

export function search(params: SearchParams): Promise<SearchResult> {
  const sp = new URLSearchParams()
  sp.set('q', params.q)
  if (params.limit) sp.set('limit', String(params.limit))
  if (params.repositoryId) sp.set('repositoryId', String(params.repositoryId))
  if (params.status) sp.set('status', params.status)
  if (params.dateFrom) sp.set('dateFrom', params.dateFrom)
  if (params.dateTo) sp.set('dateTo', params.dateTo)
  return getJson<SearchResult>(`/api/search?${sp.toString()}`)
}
