import { getJson } from './http'
import type { Repository } from '@/types/repository'
import type { Release } from '@/types/release'
import type { Asset, AssetStatus } from '@/types/release'

export interface SearchResult {
  repositories: Repository[]
  releases: Release[]
  assets: Asset[]
  total: number
}

export function search(query: string, limit = 20): Promise<SearchResult> {
  return getJson<SearchResult>(`/api/search?q=${encodeURIComponent(query)}&limit=${limit}`)
}
