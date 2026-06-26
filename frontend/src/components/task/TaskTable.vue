<script setup lang="ts">
import { computed, h } from 'vue'
import { NButton, NDataTable, NTag } from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import { ScrollText } from 'lucide-vue-next'

import type { Task } from '@/types/task'

defineProps<{
  tasks: Task[]
  loading: boolean
}>()

const emit = defineEmits<{
  viewLogs: [task: Task]
}>()

const columns = computed<DataTableColumns<Task>>(() => [
  {
    title: 'ID',
    key: 'id',
    width: 80
  },
  {
    title: '类型',
    key: 'type',
    width: 160
  },
  {
    title: '状态',
    key: 'status',
    width: 130,
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
    title: 'Repository',
    key: 'repositoryId',
    width: 120,
    render: (row) => row.repositoryId ?? '-'
  },
  {
    title: 'Asset',
    key: 'assetId',
    width: 100,
    render: (row) => row.assetId ?? '-'
  },
  {
    title: '开始时间',
    key: 'startedAt',
    width: 190,
    render: (row) => formatTime(row.startedAt)
  },
  {
    title: '结束时间',
    key: 'finishedAt',
    width: 190,
    render: (row) => formatTime(row.finishedAt)
  },
  {
    title: '错误',
    key: 'errorMessage',
    ellipsis: {
      tooltip: true
    },
    render: (row) => row.errorMessage || '-'
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
          secondary: true,
          onClick: () => emit('viewLogs', row)
        },
        {
          icon: () => h(ScrollText),
          default: () => '日志'
        }
      )
  }
])

function statusTagType(status: string) {
  if (status === 'succeeded') {
    return 'success'
  }
  if (status === 'failed') {
    return 'error'
  }
  if (status === 'running') {
    return 'warning'
  }
  return 'default'
}

function formatTime(value: string | null) {
  if (!value) {
    return '-'
  }
  return new Date(value).toLocaleString()
}
</script>

<template>
  <NDataTable
    :columns="columns"
    :data="tasks"
    :loading="loading"
    :row-key="(row) => row.id"
    :pagination="{ pageSize: 12 }"
  />
</template>
