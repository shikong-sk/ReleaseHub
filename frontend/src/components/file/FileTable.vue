<script setup lang="ts">
import { computed, h, ref } from 'vue'
import { Download, Trash2 } from 'lucide-vue-next'
import { NButton, NDataTable, NPopconfirm, useMessage } from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'

import { assetFileURL, downloadAssetFile } from '@/api/files'
import type { FileItem } from '@/types/file'

const props = defineProps<{
  files: FileItem[]
  loading: boolean
  canWrite: boolean
}>()

const emit = defineEmits<{
  delete: [file: FileItem]
}>()

const message = useMessage()
const downloadingIds = ref<Set<number>>(new Set())

async function handleDownload(assetId: number, filename: string) {
  downloadingIds.value = new Set(downloadingIds.value).add(assetId)
  try {
    await downloadAssetFile(assetId, filename)
    message.success(`已开始下载 ${filename}`)
  } catch (err) {
    message.error(err instanceof Error ? err.message : '下载失败')
  } finally {
    const next = new Set(downloadingIds.value)
    next.delete(assetId)
    downloadingIds.value = next
  }
}

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
    width: 160,
    render: (row) => {
      const buttons = [
        h(NButton, {
          size: 'small',
          type: 'primary',
          secondary: true,
          loading: downloadingIds.value.has(row.assetId),
          onClick: () => handleDownload(row.assetId, row.name)
        }, {
          icon: () => h(Download),
          default: () => '下载'
        })
      ]
      if (props.canWrite) {
        buttons.push(
          h(NPopconfirm, { positiveText: "确定", negativeText: "取消",
            onPositiveClick: () => emit('delete', row)
          }, {
            trigger: () => h(NButton, {
              size: 'small',
              type: 'error',
              secondary: true
            }, {
              icon: () => h(Trash2),
              default: () => '删除'
            }),
            default: () => `删除 ${row.name}？`
          })
        )
      }
      return h('div', { style: 'display: flex; gap: 8px;' }, buttons)
    }
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
