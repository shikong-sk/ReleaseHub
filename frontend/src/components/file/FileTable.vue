<script setup lang="ts">
import { computed, h } from 'vue'
import { Download } from 'lucide-vue-next'
import { NButton, NDataTable } from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'

import { assetFileURL } from '@/api/files'
import type { FileItem } from '@/types/file'

defineProps<{
  files: FileItem[]
  loading: boolean
}>()

const columns = computed<DataTableColumns<FileItem>>(() => [
  {
    title: '文件',
    key: 'name',
    ellipsis: {
      tooltip: true
    }
  },
  {
    title: '仓库',
    key: 'repository',
    width: 220,
    render: (row) => `${row.owner}/${row.repo}`
  },
  {
    title: '版本',
    key: 'tag',
    width: 130
  },
  {
    title: '大小',
    key: 'size',
    width: 120,
    render: (row) => formatBytes(row.size)
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
    title: '下载时间',
    key: 'downloadedAt',
    width: 190,
    render: (row) => formatTime(row.downloadedAt)
  },
  {
    title: '操作',
    key: 'actions',
    width: 110,
    render: (row) =>
      h(
        NButton,
        {
          size: 'small',
          type: 'primary',
          secondary: true,
          tag: 'a',
          href: assetFileURL(row.assetId)
        },
        {
          icon: () => h(Download),
          default: () => '下载'
        }
      )
  }
])

function formatBytes(size: number) {
  if (size < 1024) {
    return `${size} B`
  }
  if (size < 1024 * 1024) {
    return `${(size / 1024).toFixed(1)} KB`
  }
  return `${(size / 1024 / 1024).toFixed(1)} MB`
}

function formatTime(value: string) {
  if (!value) {
    return '-'
  }
  return new Date(value).toLocaleString()
}
</script>

<template>
  <NDataTable
    :columns="columns"
    :data="files"
    :loading="loading"
    :row-key="(row) => row.assetId"
    :pagination="{ pageSize: 12 }"
  />
</template>
