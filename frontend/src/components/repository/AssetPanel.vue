<script setup lang="ts">
import { computed, h } from 'vue'
import { NButton, NCard, NDataTable, NTag } from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'

import type { Asset, CheckReleaseResult } from '@/types/release'

const props = defineProps<{
  result: CheckReleaseResult | null
  downloadingAssetId: number | null
}>()

const emit = defineEmits<{
  download: [asset: Asset]
}>()

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
      h(
        NTag,
        {
          type: statusTagType(row.status)
        },
        { default: () => row.status }
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
    width: 140,
    render: (row) =>
      h(
        NButton,
        {
          size: 'small',
          type: 'primary',
          secondary: true,
          disabled: row.status === 'skipped',
          loading: props.downloadingAssetId === row.id,
          onClick: () => emit('download', row)
        },
        { default: () => (row.status === 'verified' ? '重新下载' : '下载') }
      )
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
  return 'default'
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
</script>

<template>
  <NCard v-if="result" class="asset-panel" :title="title" :bordered="false">
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
