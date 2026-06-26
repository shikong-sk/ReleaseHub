import { defineStore } from 'pinia'
import { computed, shallowRef } from 'vue'

import {
  checkAllRepository,
  checkRepository,
  createRepository,
  deleteRepository,
  listRepositories,
  syncRepository,
  updateRepository
} from '@/api/repositories'
import type { Repository, RepositoryPayload } from '@/types/repository'
import type { CheckAllReleaseResult, CheckReleaseResult, SyncReleaseResult } from '@/types/release'

export const useRepositoriesStore = defineStore('repositories', () => {
  const items = shallowRef<Repository[]>([])
  const loading = shallowRef(false)
  const saving = shallowRef(false)
  const checkingId = shallowRef<number | null>(null)
  const checkingAllId = shallowRef<number | null>(null)
  const syncingId = shallowRef<number | null>(null)
  const error = shallowRef<string | null>(null)

  const enabledCount = computed(() => items.value.filter((item) => item.enabled).length)
  const totalCount = computed(() => items.value.length)

  async function refresh() {
    loading.value = true
    error.value = null

    try {
      const response = await listRepositories()
      items.value = response.items
    } catch (err) {
      error.value = err instanceof Error ? err.message : '仓库列表加载失败'
    } finally {
      loading.value = false
    }
  }

  async function create(payload: RepositoryPayload) {
    saving.value = true
    error.value = null

    try {
      const repository = await createRepository(payload)
      items.value = [repository, ...items.value]
      return repository
    } finally {
      saving.value = false
    }
  }

  async function update(id: number, payload: Partial<RepositoryPayload>) {
    saving.value = true
    error.value = null

    try {
      const repository = await updateRepository(id, payload)
      items.value = items.value.map((item) => (item.id === id ? repository : item))
      return repository
    } finally {
      saving.value = false
    }
  }

  async function remove(id: number) {
    saving.value = true
    error.value = null

    try {
      await deleteRepository(id)
      items.value = items.value.filter((item) => item.id !== id)
    } finally {
      saving.value = false
    }
  }

  async function toggleEnabled(repository: Repository) {
    return update(repository.id, {
      enabled: !repository.enabled
    })
  }

  async function checkLatest(repository: Repository): Promise<CheckReleaseResult> {
    checkingId.value = repository.id
    error.value = null

    try {
      const result = await checkRepository(repository.id)
      items.value = items.value.map((item) =>
        item.id === repository.id ? result.repository : item
      )
      return result
    } finally {
      checkingId.value = null
    }
  }

  async function checkAll(repository: Repository): Promise<CheckAllReleaseResult> {
    checkingAllId.value = repository.id
    error.value = null

    try {
      const result = await checkAllRepository(repository.id)
      items.value = items.value.map((item) =>
        item.id === repository.id ? result.repository : item
      )
      return result
    } finally {
      checkingAllId.value = null
    }
  }

  async function syncLatest(repository: Repository): Promise<SyncReleaseResult> {
    syncingId.value = repository.id
    error.value = null

    try {
      const result = await syncRepository(repository.id)
      items.value = items.value.map((item) =>
        item.id === repository.id ? result.repository : item
      )
      return result
    } finally {
      syncingId.value = null
    }
  }

  return {
    items,
    loading,
    saving,
    checkingId,
    checkingAllId,
    syncingId,
    error,
    enabledCount,
    totalCount,
    refresh,
    create,
    update,
    remove,
    toggleEnabled,
    checkLatest,
    checkAll,
    syncLatest
  }
})
