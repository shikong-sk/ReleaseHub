<script setup lang="ts">
import { computed, h, shallowRef } from 'vue'
import { NButton, NCard, NDataTable, NTag, NTooltip, useMessage } from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import { Upload } from 'lucide-vue-next'

import { uploadAsset } from '@/api/upload'
import type { Asset, CheckReleaseResult } from '@/types/release'

const props = defineProps<{
  result: CheckReleaseResult | null
  downloadingAssetId: number | null
}>()

const emit = defineEmits<{
  download: [asset: Asset]
  retry: [asset: Asset]
  refresh: []
}>()

const message = useMessage()
const uploading = shallowRef(false)

const assets = computed(() => props.result?.assets ?? [])
const title = computed(() => {
  if (!props.result) {
    return 'Release 资产'
  }
  return `${props.result.repository.owner}/${props.result.repository.repo} · ${props.result.release.tag}`
})

const columns = computed<DataTableColumns<Asset>>(() => [
  {
    title: '资产',
    key: 'name',
    ellipsis: {
      tooltip: true
    }
  },
  {
    title: '大小',
    key: 'size',
    width: 120,
    render: (row) => formatBytes(row.size)
  },
  {
    title: '状态',
    key: 'status',
    width: 120,
    render: (row) =>
      row.status === 'failed' && row.errorMessage
        ? h(NTooltip, {}, {
            trigger: () =>
              h(
                NTag,
                { type: 'error' },
                { default: () => '失败' }
              ),
            default: () => row.errorMessage
          })
        : h(
            NTag,
            { type: statusTagType(row.status) },
            { default: () => statusLabel(row.status) }
          )
  },
  {
    title: 'SHA256',
    key: 'sha256',
    ellipsis: {
      tooltip: true
    },
    render: (row) => row.sha256 || '-'
  },
  {
    title: '操作',
    key: 'actions',
    width: 180,
    render: (row) => {
      const buttons = []
      if (row.status === 'failed') {
        buttons.push(
          h(
            NButton,
            {
              size: 'small',
              type: 'warning',
              secondary: true,
              loading: props.downloadingAssetId === row.id,
              onClick: () => emit('retry', row)
            },
            { default: () => '重试' }
          )
        )
      }
      if (row.status === 'verified' || row.status === 'downloaded') {
        buttons.push(
          h(
            NButton,
            {
              size: 'small',
              type: 'primary',
              secondary: true,
              loading: props.downloadingAssetId === row.id,
              onClick: () => emit('download', row)
            },
            { default: () => '重新下载' }
          )
        )
      }
      if (row.status === 'pending') {
        buttons.push(
          h(
            NButton,
            {
              size: 'small',
              type: 'primary',
              secondary: true,
              loading: props.downloadingAssetId === row.id,
              onClick: () => emit('download', row)
            },
            { default: () => '下载' }
          )
        )
      }
      if (row.status === 'skipped') {
        buttons.push(
          h(NTag, { type: 'default', size: 'small' }, { default: () => '已跳过' })
        )
      }
      return h('div', { style: 'display: flex; gap: 8px;' }, buttons)
    }
  }
])

function statusTagType(status: string) {
  if (status === 'verified' || status === 'downloaded') {
    return 'success'
  }
  if (status === 'failed') {
    return 'error'
  }
  if (status === 'pending') {
    return 'warning'
  }
  if (status === 'downloading') {
    return 'info'
  }
  return 'default'
}

function statusLabel(status: string) {
  const map: Record<string, string> = {
    pending: '待下载',
    skipped: '已跳过',
    downloading: '下载中',
    downloaded: '已下载',
    verified: '已验证',
    failed: '失败',
    deleted: '已删除'
  }
  return map[status] ?? status
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

function triggerUpload() {
  const input = document.createElement('input')
  input.type = 'file'
  input.multiple = false
  input.onchange = async () => {
    const file = input.files?.[0]
    if (!file || !props.result) return
    uploading.value = true
    try {
      await uploadAsset(props.result.repository.id, props.result.release.id, file)
      message.success(`${file.name} 上传成功`)
      emit('refresh')
    } catch (err) {
      message.error(err instanceof Error ? err.message : '上传失败')
    } finally {
      uploading.value = false
    }
  }
  input.click()
}
</script>

<template>
  <NCard v-if="result" class="asset-panel" :bordered="false">
    <template #header-extra>
      <NButton size="small" secondary :loading="uploading" @click="triggerUpload">
        <template #icon><Upload /></template>
        上传
      </NButton>
    </template>
    <NDataTable
      :columns="columns"
      :data="assets"
      :row-key="(row) => row.id"
      :pagination="{ pageSize: 8 }"
    />
  </NCard>
</template>

<style scoped>
.asset-panel {
  border-radius: 8px;
}
</style>
