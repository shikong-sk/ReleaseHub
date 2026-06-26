import { defineStore } from 'pinia'
import { computed, shallowRef } from 'vue'

import { listFiles } from '@/api/files'
import type { FileItem } from '@/types/file'

export const useFilesStore = defineStore('files', () => {
  const items = shallowRef<FileItem[]>([])
  const loading = shallowRef(false)
  const error = shallowRef<string | null>(null)

  const totalSize = computed(() => items.value.reduce((sum, item) => sum + item.size, 0))

  async function refresh() {
    loading.value = true
    error.value = null

    try {
      const response = await listFiles()
      items.value = response.items
    } catch (err) {
      error.value = err instanceof Error ? err.message : '文件列表加载失败'
    } finally {
      loading.value = false
    }
  }

  return {
    items,
    loading,
    error,
    totalSize,
    refresh
  }
})
