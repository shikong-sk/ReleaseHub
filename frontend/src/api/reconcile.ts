import { requestJson } from './http'

export interface ReconcileItem {
  storageName: string
  storageType: string
  path: string
  owner?: string
  repo?: string
  tag?: string
  filename?: string
  size?: number
  assetId?: number
}

export interface ReconcileResult {
  dryRun: boolean
  missingInStorage: ReconcileItem[]
  missingInDB: ReconcileItem[]
  repairedInDB: ReconcileItem[]
  resetToPending: ReconcileItem[]
  storageScanErrors: string[]
  totalStorageFiles: number
  totalDBAssets: number
}

export function runReconcile(dryRun: boolean = true): Promise<ReconcileResult> {
  return requestJson<ReconcileResult>('/api/reconcile', {
    method: 'POST',
    body: { dryRun }
  })
}
