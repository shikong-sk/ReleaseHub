<script setup lang="ts">
import { computed, onMounted, shallowRef } from 'vue'
import { NAlert, NButton, NCard, NCollapse, NCollapseItem, NGrid, NGi, NInput, NPopconfirm, NStatistic, NTag, useMessage } from 'naive-ui'
import { RefreshCw, Search, Database, Trash2 } from 'lucide-vue-next'

import FileTable from '@/components/file/FileTable.vue'
import { useFilesStore } from '@/stores/files'
import { search as apiSearch, type SearchResult } from '@/api/search'
import { runReconcile, type ReconcileResult } from '@/api/reconcile'
import { deleteAsset } from '@/api/releases'
import { useAuthStore } from '@/stores/auth'
import type { FileItem } from '@/types/file'

const filesStore = useFilesStore()
const localSearch = shallowRef('')
const globalQuery = shallowRef('')
const message = useMessage()
const globalLoading = shallowRef(false)
const globalError = shallowRef<string | null>(null)
const globalResult = shallowRef<SearchResult | null>(null)
const reconcileLoading = shallowRef(false)
const reconcileError = shallowRef<string | null>(null)
const reconcileResult = shallowRef<ReconcileResult | null>(null)
const authStore = useAuthStore()

const filteredFiles = computed(() => {
  const keyword = localSearch.value.trim().toLowerCase()
  if (!keyword) {
    return filesStore.items
  }

  return filesStore.items.filter((item) =>
    `${item.owner}/${item.repo}/${item.tag}/${item.name}`.toLowerCase().includes(keyword)
  )
})

onMounted(() => {
  void filesStore.refresh()
})

function formatBytes(size: number) {
  if (size < 1024) {
    return `${size} B`
  }
  if (size < 1024 * 1024) {
    return `${(size / 1024).toFixed(1)} KB`
  }
  return `${(size / 1024 / 1024).toFixed(1)} MB`
}

async function handleReconcile() {
  reconcileLoading.value = true
  reconcileError.value = null
  try {
    reconcileResult.value = await runReconcile()
  } catch (err) {
    reconcileError.value = err instanceof Error ? err.message : '对账失败'
  } finally {
    reconcileLoading.value = false
  }
}

async function handleDeleteFile(file: FileItem) {
  try {
    await deleteAsset(file.assetId)
    message.success(`${file.name} 已删除`)
    void filesStore.refresh()
  } catch (err) {
    message.error(err instanceof Error ? err.message : '删除失败')
  }
}

async function handleGlobalSearch() {
  const q = globalQuery.value.trim()
  if (!q) return
  globalLoading.value = true
  globalError.value = null
  try {
    globalResult.value = await apiSearch(q, 30)
  } catch (err) {
    globalError.value = err instanceof Error ? err.message : '搜索失败'
  } finally {
    globalLoading.value = false
  }
}
</script>

<template>
  <main class="files-page">
    <section class="files-heading">
      <div>
        <h1>文件</h1>
        <p>浏览已同步到本地存储的 Release Assets。</p>
      </div>
      <NButton secondary :loading="filesStore.loading" @click="filesStore.refresh">
        <template #icon>
          <RefreshCw />
        </template>
        刷新
      </NButton>
      <NButton v-if="authStore.canAdmin" secondary :loading="reconcileLoading" @click="handleReconcile">
        <template #icon><Database /></template>
        存储对账
      </NButton>
    </section>

    <NGrid cols="1 s:2 l:4" responsive="screen" :x-gap="16" :y-gap="16">
      <NGi>
        <NCard :bordered="false">
          <NStatistic label="文件数" :value="filesStore.items.length" />
        </NCard>
      </NGi>
      <NGi>
        <NCard :bordered="false">
          <NStatistic label="总大小" :value="formatBytes(filesStore.totalSize)" />
        </NCard>
      </NGi>
    </NGrid>

    <NCard :bordered="false" title="全局搜索">
      <div class="search-row">
        <NInput v-model:value="globalQuery" clearable placeholder="搜索仓库、Release 或资产" @keydown.enter="handleGlobalSearch" />
        <NButton type="primary" :loading="globalLoading" @click="handleGlobalSearch">
          <template #icon><Search /></template>
          搜索
        </NButton>
      </div>
      <NAlert v-if="globalError" type="error" closable>{{ globalError }}</NAlert>
      <div v-if="globalResult" class="search-results">
        <NCollapse>
          <NCollapseItem :title="`仓库 (${globalResult.repositories.length})`" v-if="globalResult.repositories.length">
            <div v-for="repo in globalResult.repositories" :key="repo.id" class="search-item">
              <NTag size="small">仓库</NTag>
              <span>{{ repo.owner }}/{{ repo.repo }}</span>
              <NTag size="small" :type="repo.lastStatus === 'healthy' ? 'success' : 'default'">{{ repo.lastStatus }}</NTag>
            </div>
          </NCollapseItem>
          <NCollapseItem :title="`Release (${globalResult.releases.length})`" v-if="globalResult.releases.length">
            <div v-for="rel in globalResult.releases" :key="rel.id" class="search-item">
              <NTag size="small">Release</NTag>
              <span>{{ rel.tag }}</span>
            </div>
          </NCollapseItem>
          <NCollapseItem :title="`资产 (${globalResult.assets.length})`" v-if="globalResult.assets.length">
            <div v-for="asset in globalResult.assets" :key="asset.id" class="search-item">
              <NTag size="small" :type="asset.status === 'verified' ? 'success' : 'default'">{{ asset.status }}</NTag>
              <span>{{ asset.name }}</span>
            </div>
          </NCollapseItem>
        </NCollapse>
        <p v-if="globalResult.total === 0" class="no-result">未找到匹配结果。</p>
      </div>
    </NCard>

    <NAlert v-if="filesStore.error" type="error" closable>
      {{ filesStore.error }}
    </NAlert>

    <NCard v-if="reconcileResult" :bordered="false" title="存储对账结果">
      <NAlert v-if="reconcileError" type="error" closable>{{ reconcileError }}</NAlert>
      <p v-if="reconcileResult.missingInStorage.length" style="color: #cf1322">
        存储缺失 {{ reconcileResult.missingInStorage.length }} 个文件：
        {{ reconcileResult.missingInStorage.join('、') }}
      </p>
      <p v-if="reconcileResult.missingInDB.length" style="color: #fa8c16">
        数据库缺失 {{ reconcileResult.missingInDB.length }} 个文件：
        {{ reconcileResult.missingInDB.join('、') }}
      </p>
      <p v-if="!reconcileResult.missingInStorage.length && !reconcileResult.missingInDB.length && !reconcileResult.orphanedAssets.length" style="color: #52c41a">
        存储与数据库完全一致。
      </p>
      <p v-if="reconcileResult.orphanedAssets.length" style="color: #8c8c8c">
        {{ reconcileResult.orphanedAssets.length }} 个孤立资产记录待清理。
      </p>
    </NCard>

    <NCard :bordered="false" title="本地文件">
      <NInput v-model:value="localSearch" class="file-search" clearable placeholder="搜索 owner/repo/tag/name" />
      <FileTable class="file-table" :files="filteredFiles" :loading="filesStore.loading" :can-write="authStore.canWrite" @delete="handleDeleteFile" />
    </NCard>
  </main>
</template>

<style scoped>
.files-page {
  display: flex;
  flex-direction: column;
  gap: 20px;
  max-width: 1180px;
  margin: 0 auto;
}

.files-heading {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}

.files-heading h1 {
  margin: 0;
  color: #101828;
  font-size: 28px;
  font-weight: 700;
  letter-spacing: 0;
}

.files-heading p {
  margin: 6px 0 0;
  color: #667085;
}

.search-row {
  display: flex;
  gap: 8px;
  max-width: 480px;
}

.search-results {
  margin-top: 12px;
}

.search-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 4px 0;
  font-size: 13px;
  color: #344054;
}

.no-result {
  color: #667085;
  font-size: 13px;
  margin: 8px 0 0;
}

.file-search {
  max-width: 420px;
}

.file-table {
  margin-top: 16px;
}
</style>
