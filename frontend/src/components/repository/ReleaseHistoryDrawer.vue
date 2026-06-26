<script setup lang="ts">
import { computed, shallowRef, watch } from 'vue'
import { NButton, NDataTable, NDrawer, NDrawerContent, NTag, NSpin, NAlert } from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import { Download } from 'lucide-vue-next'

import { listRepositoryReleases, listReleaseAssets } from '@/api/releases'
import type { Release, Asset, Repository } from '@/types/release'

const show = defineModel<boolean>('show', { required: true })

const props = defineProps<{
  repository: Repository | null
}>()

const releases = shallowRef<Release[]>([])
const loading = shallowRef(false)
const error = shallowRef<string | null>(null)
const expandedReleaseId = shallowRef<number | null>(null)
const assets = shallowRef<Asset[]>([])
const assetsLoading = shallowRef(false)

const title = computed(() =>
  props.repository ? `${props.repository.owner}/${props.repository.repo} - Release 历史` : 'Release 历史'
)

const columns = computed<DataTableColumns<Release>>(() => [
  {
    title: '版本',
    key: 'tag',
    width: 160,
    render: (row) =>
      row.isLatest
        ? h('div', { style: 'display: flex; gap: 6px; align-items: center;' }, [
            h('span', row.tag),
            h(NTag, { size: 'small', type: 'success' }, { default: () => 'Latest' })
          ])
        : row.tag
  },
  { title: '名称', key: 'name', ellipsis: { tooltip: true } },
  {
    title: '发布时间',
    key: 'publishedAt',
    width: 180,
    render: (row) => (row.publishedAt ? new Date(row.publishedAt).toLocaleString() : '-')
  },
  {
    title: '状态',
    key: 'syncStatus',
    width: 100,
    render: (row) => h(NTag, { size: 'small', type: row.syncStatus === 'checked' ? 'success' : 'default' }, { default: () => row.syncStatus })
  },
  {
    title: '操作',
    key: 'actions',
    width: 100,
    render: (row) =>
      h(NButton, {
        size: 'small',
        secondary: true,
        onClick: () => toggleAssets(row)
      }, {
        default: () => expandedReleaseId.value === row.id ? '收起资产' : '查看资产'
      })
  }
])

watch(() => props.repository, async (repo) => {
  releases.value = []
  assets.value = []
  expandedReleaseId.value = null
  if (!repo) return
  loading.value = true
  error.value = null
  try {
    const result = await listRepositoryReleases(repo.id)
    releases.value = result.items
  } catch (err) {
    error.value = err instanceof Error ? err.message : '加载 Release 失败'
  } finally {
    loading.value = false
  }
})

async function toggleAssets(release: Release) {
  if (expandedReleaseId.value === release.id) {
    expandedReleaseId.value = null
    assets.value = []
    return
  }
  expandedReleaseId.value = release.id
  assetsLoading.value = true
  assets.value = []
  try {
    const result = await listReleaseAssets(release.id)
    assets.value = result.items
  } catch (err) {
    assets.value = []
  } finally {
    assetsLoading.value = false
  }
}

function statusLabel(status: string) {
  const map: Record<string, string> = {
    pending: '待下载', skipped: '已跳过', downloading: '下载中',
    downloaded: '已下载', verified: '已验证', failed: '失败', deleted: '已删除'
  }
  return map[status] ?? status
}

function statusTagType(status: string) {
  if (status === 'verified' || status === 'downloaded') return 'success'
  if (status === 'failed') return 'error'
  if (status === 'pending') return 'warning'
  return 'default'
}

function formatBytes(size: number) {
  if (size < 1024) return `${size} B`
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`
  return `${(size / 1024 / 1024).toFixed(1)} MB`
}
</script>

<template>
  <NDrawer v-model:show="show" :width="640" placement="right">
    <NDrawerContent :title="title" closable>
      <NAlert v-if="error" type="error" closable>{{ error }}</NAlert>
      <NSpin :show="loading">
        <NDataTable
          :columns="columns"
          :data="releases"
          :row-key="(row: Release) => row.id"
          :pagination="{ pageSize: 10 }"
        />
      </NSpin>

      <div v-if="expandedReleaseId && assets.length > 0" class="assets-panel">
        <h4>资产列表</h4>
        <div class="asset-list">
          <div v-for="asset in assets" :key="asset.id" class="asset-item">
            <NTag size="small" :type="statusTagType(asset.status)">{{ statusLabel(asset.status) }}</NTag>
            <span class="asset-name">{{ asset.name }}</span>
            <span class="asset-size">{{ formatBytes(asset.size) }}</span>
            <a v-if="asset.status === 'verified'" :href="`/api/assets/${asset.id}/file`" class="asset-download">
              <NButton size="tiny" type="primary" secondary>下载</NButton>
            </a>
          </div>
        </div>
      </div>
      <NSpin v-if="assetsLoading" :show="true" style="margin-top: 12px" />
    </NDrawerContent>
  </NDrawer>
</template>

<style scoped>
.assets-panel {
  margin-top: 16px;
  padding-top: 12px;
  border-top: 1px solid #e5e7eb;
}

.assets-panel h4 {
  margin: 0 0 8px;
  font-size: 14px;
  color: #344054;
}

.asset-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.asset-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 4px 0;
}

.asset-name {
  flex: 1;
  font-size: 13px;
  color: #344054;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.asset-size {
  font-size: 12px;
  color: #667085;
}

.asset-download {
  text-decoration: none;
}
</style>
