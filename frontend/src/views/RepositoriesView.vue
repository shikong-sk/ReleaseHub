<script setup lang="ts">
import { computed, onMounted, onUnmounted, shallowRef } from 'vue'
import { NAlert, NCard, NGrid, NGi, NStatistic, useMessage } from 'naive-ui'

import AssetPanel from '@/components/repository/AssetPanel.vue'
import RepositoryFilesDrawer from '@/components/repository/RepositoryFilesDrawer.vue'
import ReleaseHistoryDrawer from '@/components/repository/ReleaseHistoryDrawer.vue'
import RepositoryFormDrawer from '@/components/repository/RepositoryFormDrawer.vue'
import RepositoryTable from '@/components/repository/RepositoryTable.vue'
import RepositoryToolbar from '@/components/repository/RepositoryToolbar.vue'
import { useAuthStore } from '@/stores/auth'
import { useRepositoriesStore } from '@/stores/repositories'
import { useReleasesStore } from '@/stores/releases'
import { deleteAsset, listReleaseAssets } from '@/api/releases'
import type { Asset } from '@/types/release'
import type { Repository, RepositoryFormMode, RepositoryPayload } from '@/types/repository'

const authStore = useAuthStore()
const repositoryStore = useRepositoriesStore()
const releaseStore = useReleasesStore()
const message = useMessage()

const search = shallowRef('')
const drawerVisible = shallowRef(false)
const formMode = shallowRef<RepositoryFormMode>('create')
const editingRepository = shallowRef<Repository | null>(null)
const historyRepository = shallowRef<Repository | null>(null)
const showHistory = shallowRef(false)
const filesRepository = shallowRef<Repository | null>(null)
const showFiles = shallowRef(false)
let assetsRefreshTimer: number | undefined

const filteredRepositories = computed(() => {
  const keyword = search.value.trim().toLowerCase()
  if (!keyword) {
    return repositoryStore.items
  }

  return repositoryStore.items.filter((item) =>
    `${item.owner}/${item.repo}`.toLowerCase().includes(keyword)
  )
})

onMounted(() => {
  void repositoryStore.refresh()
  assetsRefreshTimer = window.setInterval(() => {
    void refreshLatestAssets()
  }, 3000)
})

onUnmounted(() => {
  if (assetsRefreshTimer) {
    window.clearInterval(assetsRefreshTimer)
  }
})

function openHistory(repository: Repository) {
  historyRepository.value = repository
  showHistory.value = true
}

function openFiles(repository: Repository) {
  filesRepository.value = repository
  showFiles.value = true
}

async function refreshLatestAssets() {
  const latest = releaseStore.latestCheck
  if (!latest) return

  try {
    const result = await listReleaseAssets(latest.release.id)
    releaseStore.setLatestCheck({
      ...latest,
      assets: result.items
    })
  } catch {
    // 轮询刷新失败不打断用户当前操作。
  }
}

function openCreateDrawer() {
  formMode.value = 'create'
  editingRepository.value = null
  drawerVisible.value = true
}

function openEditDrawer(repository: Repository) {
  formMode.value = 'edit'
  editingRepository.value = repository
  drawerVisible.value = true
}

async function submitRepository(payload: RepositoryPayload) {
  try {
    if (formMode.value === 'create') {
      await repositoryStore.create(payload)
      message.success('仓库已新增')
    } else if (editingRepository.value) {
      await repositoryStore.update(editingRepository.value.id, {
        githubTokenId: payload.githubTokenId,
        storageId: payload.storageId,
        proxyId: payload.proxyId,
        providerApiBaseUrl: payload.providerApiBaseUrl,
        enabled: payload.enabled,
        intervalSeconds: payload.intervalSeconds,
        filterMode: payload.filterMode,
        assetIncludePatterns: payload.assetIncludePatterns,
        assetExcludePatterns: payload.assetExcludePatterns,
        retentionKeepLatest: payload.retentionKeepLatest
      })
      message.success('仓库已更新')
    }
    drawerVisible.value = false
  } catch (err) {
    message.error(err instanceof Error ? err.message : '保存仓库失败')
  }
}

async function toggleRepository(repository: Repository) {
  try {
    await repositoryStore.toggleEnabled(repository)
  } catch (err) {
    message.error(err instanceof Error ? err.message : '更新启用状态失败')
  }
}

async function removeRepository(repository: Repository) {
  try {
    await repositoryStore.remove(repository.id)
    message.success('仓库已删除')
  } catch (err) {
    message.error(err instanceof Error ? err.message : '删除仓库失败')
  }
}

async function checkRepository(repository: Repository) {
  try {
    const result = await repositoryStore.checkLatest(repository)
    releaseStore.setLatestCheck(result)
    const pendingCount = result.assets.filter((asset) => asset.status === 'pending').length
    message.success(`发现 ${result.release.tag}，${pendingCount} 个资产待下载`)
  } catch (err) {
    message.error(err instanceof Error ? err.message : '检查 Release 失败')
  }
}

async function checkAllRepository(repository: Repository) {
  try {
    const result = await repositoryStore.checkAll(repository)
    message.success(
      `全量检查完成：${result.releases} 个 Release，${result.newReleases} 个新增，${result.pendingAssets} 个资产待下载`
    )
  } catch (err) {
    message.error(err instanceof Error ? err.message : '全量检查失败')
  }
}

async function syncRepository(repository: Repository) {
  try {
    const result = await repositoryStore.syncLatest(repository)
    releaseStore.setLatestCheck(result)
    message.success(`已同步 ${result.release.tag}，下载 ${result.downloadResults.length} 个资产`)
  } catch (err) {
    message.error(err instanceof Error ? err.message : '同步 Release 失败')
  }
}

async function downloadAsset(asset: Asset) {
  try {
    const downloadedAsset = await releaseStore.download(asset)
    message.success(`已下载 ${downloadedAsset.name}`)
  } catch (err) {
    message.error(err instanceof Error ? err.message : '下载资产失败')
  }
}

async function handleDeleteAsset(asset: Asset) {
  try {
    await deleteAsset(asset.id)
    message.success(`已删除 ${asset.name}`)
    await refreshLatestAssets()
  } catch (err) {
    message.error(err instanceof Error ? err.message : '删除资产失败')
  }
}

async function retryAsset(asset: Asset) {
  try {
    const downloadedAsset = await releaseStore.redownload(asset)
    message.success(`已重新下载 ${downloadedAsset.name}`)
  } catch (err) {
    message.error(err instanceof Error ? err.message : '重试下载失败')
  }
}
</script>

<template>
  <main class="repositories-page">
    <section class="repositories-heading">
      <h1>仓库</h1>
      <p>管理需要同步的 GitHub Release 仓库与资产过滤策略。</p>
    </section>

    <NGrid cols="1 s:2 l:4" responsive="screen" :x-gap="16" :y-gap="16">
      <NGi>
        <NCard :bordered="false">
          <NStatistic label="总仓库" :value="repositoryStore.totalCount" />
        </NCard>
      </NGi>
      <NGi>
        <NCard :bordered="false">
          <NStatistic label="已启用" :value="repositoryStore.enabledCount" />
        </NCard>
      </NGi>
    </NGrid>

    <NAlert v-if="repositoryStore.error" type="error" closable>
      {{ repositoryStore.error }}
    </NAlert>

    <NCard :bordered="false">
      <RepositoryToolbar
        v-model:search="search"
        :loading="repositoryStore.loading"
        :can-write="authStore.canWrite"
        @create="openCreateDrawer"
        @refresh="repositoryStore.refresh"
      />

      <RepositoryTable
        class="repositories-table"
        :repositories="filteredRepositories"
        :loading="repositoryStore.loading"
        :saving="repositoryStore.saving"
        :checking-id="repositoryStore.checkingId"
        :checking-all-id="repositoryStore.checkingAllId"
        :syncing-id="repositoryStore.syncingId"
        :can-write="authStore.canWrite"
        @edit="openEditDrawer"
        @toggle="toggleRepository"
        @remove="removeRepository"
        @check="checkRepository"
        @check-all="checkAllRepository"
        @sync="syncRepository"
        @history="openHistory"
        @files="openFiles"
      />
    </NCard>

    <AssetPanel
      :result="releaseStore.latestCheck"
      :downloading-asset-id="releaseStore.downloadingAssetId"
      @download="downloadAsset"
      @retry="retryAsset"
      @delete="handleDeleteAsset"
      @refresh="refreshLatestAssets"
    />

    <ReleaseHistoryDrawer
      v-model:show="showHistory"
      :repository="historyRepository"
    />

    <RepositoryFilesDrawer
      v-model:show="showFiles"
      :repository="filesRepository"
    />

    <RepositoryFormDrawer
      v-model:show="drawerVisible"
      :mode="formMode"
      :repository="editingRepository"
      :saving="repositoryStore.saving"
      @submit="submitRepository"
    />
  </main>
</template>

<style scoped>
.repositories-page {
  display: flex;
  flex-direction: column;
  gap: 20px;
  width: 100%;
  min-width: 0;
}

.repositories-heading {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.repositories-heading h1 {
  margin: 0;
  color: #101828;
  font-size: 28px;
  font-weight: 700;
  letter-spacing: 0;
}

.repositories-heading p {
  margin: 0;
  color: #667085;
}

.repositories-table {
  margin-top: 16px;
}
</style>
