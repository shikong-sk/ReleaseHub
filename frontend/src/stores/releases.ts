import { defineStore } from 'pinia'
import { shallowRef } from 'vue'

import { downloadAsset, redownloadAsset } from '@/api/releases'
import type { Asset, CheckReleaseResult } from '@/types/release'

export const useReleasesStore = defineStore('releases', () => {
  const latestCheck = shallowRef<CheckReleaseResult | null>(null)
  const downloadingAssetId = shallowRef<number | null>(null)

  function setLatestCheck(result: CheckReleaseResult) {
    latestCheck.value = result
  }

  /** 更新 latestCheck 中指定资产的数据 */
  function updateAssetInLatestCheck(assetId: number, newAsset: Asset) {
    if (latestCheck.value) {
      latestCheck.value = {
        ...latestCheck.value,
        assets: latestCheck.value.assets.map((item) =>
          item.id === assetId ? newAsset : item
        )
      }
    }
  }

  async function download(asset: Asset) {
    downloadingAssetId.value = asset.id

    try {
      const result = await downloadAsset(asset.id)
      updateAssetInLatestCheck(asset.id, result.asset)
      return result.asset
    } finally {
      downloadingAssetId.value = null
    }
  }

  async function redownload(asset: Asset) {
    downloadingAssetId.value = asset.id

    try {
      const result = await redownloadAsset(asset.id)
      updateAssetInLatestCheck(asset.id, result.asset)
      return result.asset
    } finally {
      downloadingAssetId.value = null
    }
  }

  return {
    latestCheck,
    downloadingAssetId,
    setLatestCheck,
    download,
    redownload
  }
})
