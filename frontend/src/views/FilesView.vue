<script setup lang="ts">
import { computed, onMounted, shallowRef } from 'vue'
import { NAlert, NButton, NCard, NCollapse, NCollapseItem, NGrid, NGi, NInput, NRadioButton, NRadioGroup, NSpace, NStatistic, NSwitch, NTag, useMessage } from 'naive-ui'
import { RefreshCw, Search, Database, Trash2, Wrench } from 'lucide-vue-next'

import FileTable from '@/components/file/FileTable.vue'
import FileTreePanel from '@/components/file/FileTreePanel.vue'
import { useFilesStore } from '@/stores/files'
import { getFileTree } from '@/api/files'
import type { FileTreeNode } from '@/types/file'
import { search as apiSearch, type SearchResult } from '@/api/search'
import { runReconcile, type ReconcileItem, type ReconcileResult } from '@/api/reconcile'
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
const reconcileDryRun = shallowRef(true)
const authStore = useAuthStore()

const viewMode = shallowRef<'tree' | 'table'>('tree')
const fileTree = shallowRef<FileTreeNode[]>([])
const fileTreeLoading = shallowRef(false)

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
  void loadFileTree()
})

async function loadFileTree() {
  fileTreeLoading.value = true
  try {
    const result = await getFileTree()
    fileTree.value = result.tree
  } catch {
    // 树加载失败不影响平铺视图
  } finally {
    fileTreeLoading.value = false
  }
}

function formatBytes(size: number) {
  if (size < 1024) {
    return `${size} B`
  }
  if (size < 1024 * 1024) {
    return `${(size / 1024).toFixed(1)} KB`
  }
  return `${(size / 1024 / 1024).toFixed(1)} MB`
}

function formatItemPath(item: ReconcileItem): string {
  if (item.owner && item.repo && item.tag && item.filename) {
    return `${item.owner}/${item.repo}/${item.tag}/${item.filename}`
  }
  return item.path
}

async function handleReconcile() {
  reconcileLoading.value = true
  reconcileError.value = null
  try {
    reconcileResult.value = await runReconcile(reconcileDryRun.value)
    if (reconcileDryRun.value) {
      const hasIssues = reconcileResult.value.missingInStorage.length > 0 || reconcileResult.value.missingInDB.length > 0
      if (hasIssues) {
        message.info('预检完成，发现不一致项。关闭预检模式后可执行修复。')
      } else {
        message.success('预检完成，存储与数据库完全一致。')
      }
    } else {
      const repaired = reconcileResult.value.repairedInDB.length
      const reset = reconcileResult.value.resetToPending.length
      message.success(`修复完成：修复 ${repaired} 条记录，重置 ${reset} 条记录。`)
      // 修复后刷新文件列表和树
      void filesStore.refresh()
      void loadFileTree()
    }
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
      <NSpace align="center" :size="12">
        <NButton secondary :loading="filesStore.loading" @click="filesStore.refresh">
          <template #icon><RefreshCw /></template>
          刷新
        </NButton>
        <template v-if="authStore.canAdmin">
          <NSpace align="center" :size="6">
            <NSwitch v-model:value="reconcileDryRun" size="small" />
            <span class="switch-label">预检模式</span>
          </NSpace>
          <NButton
            secondary
            :type="reconcileDryRun ? 'default' : 'warning'"
            :loading="reconcileLoading"
            @click="handleReconcile"
          >
            <template #icon><Database /></template>
            {{ reconcileDryRun ? '对账预检' : '对账修复' }}
          </NButton>
        </template>
      </NSpace>
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

    <NCard v-if="reconcileResult" :bordered="false" :title="reconcileResult.dryRun ? '对账预检结果' : '对账修复结果'">
      <template #header-extra>
        <NSpace :size="8">
          <NTag :type="reconcileResult.dryRun ? 'info' : 'warning'" size="small">
            {{ reconcileResult.dryRun ? '预检' : '已修复' }}
          </NTag>
        </NSpace>
      </template>

      <NAlert v-if="reconcileError" type="error" closable>{{ reconcileError }}</NAlert>

      <!-- 扫描错误 -->
      <NAlert v-if="reconcileResult.storageScanErrors.length" type="warning" style="margin-bottom: 12px">
        <template #header>扫描异常（{{ reconcileResult.storageScanErrors.length }}）</template>
        <ul style="margin: 4px 0 0; padding-left: 16px">
          <li v-for="(err, idx) in reconcileResult.storageScanErrors" :key="idx">{{ err }}</li>
        </ul>
      </NAlert>

      <!-- 统计概览 -->
      <NGrid cols="2 s:4" responsive="screen" :x-gap="12" :y-gap="8" style="margin-bottom: 16px">
        <NGi>
          <NStatistic label="存储文件数" :value="reconcileResult.totalStorageFiles" />
        </NGi>
        <NGi>
          <NStatistic label="数据库记录数" :value="reconcileResult.totalDBAssets" />
        </NGi>
      </NGrid>

      <!-- 存储缺失文件（DB有记录但存储无文件） -->
      <template v-if="reconcileResult.missingInStorage.length">
        <h4 style="color: #cf1322; margin: 0 0 8px">存储缺失（{{ reconcileResult.missingInStorage.length }}）</h4>
        <p style="color: #667085; font-size: 13px; margin: 0 0 8px">数据库中有记录但存储中找不到对应文件，{{ reconcileResult.dryRun ? '预检模式不会修改数据' : '已将状态重置为待下载' }}。</p>
        <div class="reconcile-list">
          <div v-for="item in reconcileResult.missingInStorage" :key="item.path" class="reconcile-item">
            <NTag size="small" type="error">缺失</NTag>
            <span class="reconcile-path">{{ formatItemPath(item) }}</span>
            <NTag v-if="item.storageName" size="small" :bordered="false">{{ item.storageName }}</NTag>
          </div>
        </div>
      </template>

      <!-- 数据库缺失记录（存储有文件但DB无记录） -->
      <template v-if="reconcileResult.missingInDB.length">
        <h4 style="color: #fa8c16; margin: 16px 0 8px">数据库缺失（{{ reconcileResult.missingInDB.length }}）</h4>
        <p style="color: #667085; font-size: 13px; margin: 0 0 8px">存储中有文件但数据库中无对应记录，{{ reconcileResult.dryRun ? '预检模式不会修改数据' : '已在数据库中补建记录' }}。</p>
        <div class="reconcile-list">
          <div v-for="item in reconcileResult.missingInDB" :key="item.path" class="reconcile-item">
            <NTag size="small" type="warning">缺记录</NTag>
            <span class="reconcile-path">{{ formatItemPath(item) }}</span>
            <NTag v-if="item.storageName" size="small" :bordered="false">{{ item.storageName }}</NTag>
            <span v-if="item.size" class="reconcile-size">{{ formatBytes(item.size) }}</span>
          </div>
        </div>
      </template>

      <!-- 已修复记录 -->
      <template v-if="!reconcileResult.dryRun && reconcileResult.repairedInDB.length">
        <h4 style="color: #52c41a; margin: 16px 0 8px">已修复数据库记录（{{ reconcileResult.repairedInDB.length }}）</h4>
        <div class="reconcile-list">
          <div v-for="item in reconcileResult.repairedInDB" :key="item.path" class="reconcile-item">
            <NTag size="small" type="success">已修复</NTag>
            <span class="reconcile-path">{{ formatItemPath(item) }}</span>
          </div>
        </div>
      </template>

      <!-- 已重置为待下载 -->
      <template v-if="!reconcileResult.dryRun && reconcileResult.resetToPending.length">
        <h4 style="color: #1677ff; margin: 16px 0 8px">已重置为待下载（{{ reconcileResult.resetToPending.length }}）</h4>
        <div class="reconcile-list">
          <div v-for="item in reconcileResult.resetToPending" :key="item.path" class="reconcile-item">
            <NTag size="small" type="info">待重下载</NTag>
            <span class="reconcile-path">{{ formatItemPath(item) }}</span>
          </div>
        </div>
      </template>

      <!-- 完全一致 -->
      <NAlert
        v-if="!reconcileResult.missingInStorage.length && !reconcileResult.missingInDB.length && !reconcileResult.storageScanErrors.length"
        type="success"
        style="margin-top: 8px"
      >
        存储与数据库完全一致。
      </NAlert>
    </NCard>

    <NCard :bordered="false" title="本地文件">
      <template #header-extra>
        <NRadioGroup v-model:value="viewMode" size="small">
          <NRadioButton value="tree">树状</NRadioButton>
          <NRadioButton value="table">列表</NRadioButton>
        </NRadioGroup>
      </template>
      <template v-if="viewMode === 'tree'">
        <FileTreePanel
          :tree="fileTree"
          :loading="fileTreeLoading"
          :can-write="authStore.canWrite"
          @refresh="loadFileTree"
        />
      </template>
      <template v-else>
        <NInput v-model:value="localSearch" class="file-search" clearable placeholder="搜索 owner/repo/tag/name" />
        <FileTable class="file-table" :files="filteredFiles" :loading="filesStore.loading" :can-write="authStore.canWrite" @delete="handleDeleteFile" />
      </template>
    </NCard>
  </main>
</template>

<style scoped>
.files-page {
  display: flex;
  flex-direction: column;
  gap: 20px;
  width: 100%;
  min-width: 0;
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

.switch-label {
  font-size: 13px;
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

.reconcile-list {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.reconcile-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 4px 8px;
  font-size: 13px;
  color: #344054;
  border-radius: 4px;
}

.reconcile-item:hover {
  background-color: #f5f5f5;
}

.reconcile-path {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-family: monospace;
  font-size: 12px;
}

.reconcile-size {
  font-size: 12px;
  color: #8c8c8c;
  white-space: nowrap;
}
</style>
