<script setup lang="ts">
import { computed, h, shallowRef, watch } from 'vue'
import { NButton, NDataTable, NDrawer, NDrawerContent, NInput, NModal, NPopconfirm, NSpace, NTag, NSpin, NAlert, useMessage } from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import { Pin, PinOff } from 'lucide-vue-next'

import { listRepositoryReleases, listReleaseAssets, pinRelease, unpinRelease } from '@/api/releases'
import { syncRepositoryByTag } from '@/api/repositories'
import type { Release, Asset } from '@/types/release'
import type { Repository } from '@/types/repository'

const show = defineModel<boolean>('show', { required: true })

const props = defineProps<{
  repository: Repository | null
  canWrite: boolean
}>()

const emit = defineEmits<{
  synced: []
}>()

const message = useMessage()
const releases = shallowRef<Release[]>([])
const loading = shallowRef(false)
const error = shallowRef<string | null>(null)
const expandedReleaseId = shallowRef<number | null>(null)
const assets = shallowRef<Asset[]>([])
const assetsLoading = shallowRef(false)
const syncingTag = shallowRef<string | null>(null)
const showSyncTagModal = shallowRef(false)
const syncTagInput = shallowRef('')

const title = computed(() =>
  props.repository ? `${props.repository.owner}/${props.repository.repo} - Release 历史` : 'Release 历史'
)

const columns = computed<DataTableColumns<Release>>(() => [
  {
    title: '版本',
    key: 'tag',
    width: 180,
    render: (row) => {
      const tags = []
      if (row.isLatest) tags.push(h(NTag, { size: 'small', type: 'success' }, { default: () => 'Latest' }))
      if (row.isPinned) tags.push(h(NTag, { size: 'small', type: 'warning' }, { default: () => 'Pinned' }))
      return h('div', { style: 'display: flex; gap: 6px; align-items: center' }, [
        h('span', row.tag),
        ...tags
      ])
    }
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
    width: 280,
    render: (row) =>
      h(NSpace, { size: 'small' }, {
        default: () => {
          const buttons = []
          // 查看资产
          buttons.push(
            h(NButton, {
              size: 'small',
              secondary: true,
              onClick: () => toggleAssets(row)
            }, {
              default: () => expandedReleaseId.value === row.id ? '收起' : '资产'
            })
          )
          if (props.canWrite) {
            // 同步指定版本
            buttons.push(
              h(NButton, {
                size: 'small',
                type: 'success',
                secondary: true,
                loading: syncingTag.value === row.tag,
                onClick: () => handleSyncTag(row.tag)
              }, { default: () => '同步' })
            )
            // Pin/Unpin
            if (row.isPinned) {
              buttons.push(
                h(NPopconfirm, {
                  onPositiveClick: () => handleUnpin(row)
                }, {
                  trigger: () => h(NButton, {
                    size: 'small',
                    type: 'warning',
                    secondary: true
                  }, {
                    icon: () => h(PinOff, { size: 14 }),
                    default: () => '取消固定'
                  }),
                  default: () => `取消固定版本 ${row.tag}？取消后可能被保留策略清理。`
                })
              )
            } else {
              buttons.push(
                h(NButton, {
                  size: 'small',
                  type: 'warning',
                  secondary: true,
                  onClick: () => handlePin(row)
                }, {
                  icon: () => h(Pin, { size: 14 }),
                  default: () => '固定'
                })
              )
            }
          }
          return buttons
        }
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

async function handleSyncTag(tag: string) {
  if (!props.repository) return
  syncingTag.value = tag
  try {
    const result = await syncRepositoryByTag(props.repository.id, tag)
    const downloaded = result.downloadResults?.length ?? 0
    const failed = result.failedAssets?.length ?? 0
    if (failed > 0) {
      message.warning(`同步 ${tag} 完成：下载 ${downloaded} 个，失败 ${failed} 个`)
    } else {
      message.success(`同步 ${tag} 完成，下载 ${downloaded} 个资产`)
    }
    // 刷新列表
    const listResult = await listRepositoryReleases(props.repository.id)
    releases.value = listResult.items
    emit('synced')
  } catch (err) {
    message.error(err instanceof Error ? err.message : `同步 ${tag} 失败`)
  } finally {
    syncingTag.value = null
  }
}

async function handlePin(release: Release) {
  try {
    await pinRelease(release.id)
    // shallowRef 需要替换数组引用才能触发渲染更新
    releases.value = releases.value.map(r => r.id === release.id ? { ...r, isPinned: true } : r)
    message.success(`已固定版本 ${release.tag}`)
  } catch (err) {
    message.error(err instanceof Error ? err.message : '固定版本失败')
  }
}

async function handleUnpin(release: Release) {
  try {
    await unpinRelease(release.id)
    releases.value = releases.value.map(r => r.id === release.id ? { ...r, isPinned: false } : r)
    message.success(`已取消固定版本 ${release.tag}`)
  } catch (err) {
    message.error(err instanceof Error ? err.message : '取消固定失败')
  }
}

function openSyncTagModal() {
  syncTagInput.value = ''
  showSyncTagModal.value = true
}

async function submitSyncTag() {
  const tag = syncTagInput.value.trim()
  if (!tag) {
    message.warning('请输入版本号')
    return
  }
  showSyncTagModal.value = false
  await handleSyncTag(tag)
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
  <NDrawer v-model:show="show" :width="760" placement="right">
    <NDrawerContent :title="title" closable>
      <div v-if="canWrite" style="margin-bottom: 12px">
        <NButton size="small" type="success" @click="openSyncTagModal">同步指定版本</NButton>
      </div>
      <NAlert v-if="error" type="error" closable>{{ error }}</NAlert>
      <NSpin :show="loading">
        <NDataTable
          :columns="columns"
          :data="releases"
          :row-key="(row: Release) => row.id"
          :pagination="{ pageSize: 10 }"
          :scroll-x="760"
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

  <!-- 同步指定版本弹窗 -->
  <NModal v-model:show="showSyncTagModal" preset="dialog" title="同步指定版本" positive-text="开始同步" negative-text="取消" @positive-click="submitSyncTag">
    <NInput v-model:value="syncTagInput" placeholder="输入版本号，例如 v1.0.0" @keyup.enter="submitSyncTag" />
  </NModal>
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
