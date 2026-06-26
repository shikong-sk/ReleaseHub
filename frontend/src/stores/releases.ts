import { defineStore } from 'pinia'
import { shallowRef } from 'vue'

import { downloadAsset } from '@/api/releases'
import type { Asset, CheckReleaseResult } from '@/types/release'

export const useReleasesStore = defineStore('releases', () => {
  const latestCheck = shallowRef<CheckReleaseResult | null>(null)
  const downloadingAssetId = shallowRef<number | null>(null)

  function setLatestCheck(result: CheckReleaseResult) {
    latestCheck.value = result
  }

  async function download(asset: Asset) {
    downloadingAssetId.value = asset.id

    try {
      const result = await downloadAsset(asset.id)
      if (latestCheck.value) {
        latestCheck.value = {
          ...latestCheck.value,
          assets: latestCheck.value.assets.map((item) =>
            item.id === result.asset.id ? result.asset : item
          )
        }
      }
      return result.asset
    } finally {
      downloadingAssetId.value = null
    }
  }

  return {
    latestCheck,
    downloadingAssetId,
    setLatestCheck,
    download
  }
})
