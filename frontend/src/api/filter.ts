import { requestJson } from './http'

export interface FilterPreviewPayload {
  assetNames: string[]
  filterMode: string
  includePatterns: string
  excludePatterns: string
}

export interface FilterPreviewResult {
  name: string
  matched: boolean
}

export interface FilterPreviewResponse {
  results: FilterPreviewResult[]
  error?: string
}

export function previewFilter(payload: FilterPreviewPayload): Promise<FilterPreviewResponse> {
  return requestJson<FilterPreviewResponse>('/api/filter/preview', {
    method: 'POST',
    body: payload
  })
}
