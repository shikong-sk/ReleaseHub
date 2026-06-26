import { requestJson } from './http'

export interface ReconcileResult {
  missingInStorage: string[]
  missingInDB: string[]
  orphanedAssets: number[]
}

export function runReconcile(): Promise<ReconcileResult> {
  return requestJson<ReconcileResult>('/api/reconcile', {
    method: 'POST'
  })
}
