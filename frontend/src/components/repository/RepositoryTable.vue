<script setup lang="ts">
import { computed, h } from 'vue'
import { FolderOpen, History } from 'lucide-vue-next'
import { NButton, NDataTable, NPopconfirm, NSpace, NSwitch, NTag } from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'

import type { Repository } from '@/types/repository'

const props = defineProps<{
  repositories: Repository[]
  loading: boolean
  saving: boolean
  checkingId: number | null
  checkingAllId: number | null
  syncingId: number | null
  canWrite: boolean
}>()

const emit = defineEmits<{
  edit: [repository: Repository]
  toggle: [repository: Repository]
  remove: [repository: Repository]
  check: [repository: Repository]
  'check-all': [repository: Repository]
  sync: [repository: Repository]
  'sync-tag': [repository: Repository]
  history: [repository: Repository]
  files: [repository: Repository]
}>()

const columns = computed<DataTableColumns<Repository>>(() => [
  {
    title: '仓库',
    key: 'repository',
    render: (row) =>
      h('div', { class: 'repo-cell' }, [
        h('strong', `${row.owner}/${row.repo}`),
        h('span', row.provider)
      ])
  },
  {
    title: '启用',
    key: 'enabled',
    width: 96,
    render: (row) =>
      h(NSwitch, {
        value: row.enabled,
        loading: props.saving,
        onUpdateValue: () => emit('toggle', row)
      })
  },
  {
    title: '最新版本',
    key: 'lastReleaseTag',
    width: 140,
    render: (row) => row.lastReleaseTag || '-'
  },
  {
    title: '状态',
    key: 'lastStatus',
    width: 120,
    render: (row) =>
      h(
        NTag,
        {
          type: statusTagType(row.lastStatus)
        },
        { default: () => row.lastStatus }
      )
  },
  {
    title: '间隔',
    key: 'intervalSeconds',
    width: 120,
    render: (row) => `${Math.round(row.intervalSeconds / 60)} 分钟`
  },
  {
    title: '过滤',
    key: 'filterMode',
    width: 110,
    render: (row) => row.filterMode
  },
  {
    title: '操作',
    key: 'actions',
    width: 480,
    render: (row) =>
      h(NSpace, null, {
        default: () => {
          const buttons = []
          // 查看历史
          buttons.push(
            h(NButton, {
              size: 'small',
              secondary: true,
              onClick: () => emit('history', row)
            }, {
              icon: () => h(History, { size: 14 }),
              default: () => '历史'
            })
          )
          buttons.push(
            h(NButton, {
              size: 'small',
              secondary: true,
              onClick: () => emit('files', row)
            }, {
              icon: () => h(FolderOpen, { size: 14 }),
              default: () => '文件'
            })
          )
          // 检查操作：所有用户可见
          buttons.push(
            h(NButton, {
              size: 'small',
              type: 'primary',
              secondary: true,
              loading: props.checkingId === row.id,
              onClick: () => emit('check', row)
            }, { default: () => '检查最新' })
          )
          buttons.push(
            h(NButton, {
              size: 'small',
              type: 'info',
              secondary: true,
              loading: props.checkingAllId === row.id,
              onClick: () => emit('check-all', row)
            }, { default: () => '全量检查' })
          )
          // 写操作：仅 canWrite 用户可见
          if (props.canWrite) {
            buttons.push(
              h(NButton, {
                size: 'small',
                type: 'success',
                secondary: true,
                loading: props.syncingId === row.id,
                onClick: () => emit('sync', row)
              }, { default: () => '同步最新' })
            )
            buttons.push(
              h(NButton, {
                size: 'small',
                type: 'success',
                secondary: true,
                onClick: () => emit('sync-tag', row)
              }, { default: () => '同步指定版本' })
            )
            buttons.push(
              h(NButton, {
                size: 'small',
                secondary: true,
                onClick: () => emit('edit', row)
              }, { default: () => '编辑' })
            )
            buttons.push(
              h(NPopconfirm, {
                onPositiveClick: () => emit('remove', row)
              }, {
                trigger: () =>
                  h(NButton, {
                    size: 'small',
                    type: 'error',
                    secondary: true,
                    loading: props.saving
                  }, { default: () => '删除' }),
                default: () => `删除 ${row.owner}/${row.repo}？`
              })
            )
          }
          return buttons
        }
      })
  }
])

function statusTagType(status: string) {
  if (status === 'healthy') {
    return 'success'
  }
  if (status === 'failed') {
    return 'error'
  }
  return 'default'
}
</script>

<template>
  <NDataTable
    :columns="columns"
    :data="repositories"
    :loading="loading"
    :row-key="(row) => row.id"
    :pagination="{ pageSize: 10 }"
    :scroll-x="1180"
  />
</template>

<style scoped>
:deep(.repo-cell) {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

:deep(.repo-cell span) {
  color: #667085;
  font-size: 12px;
}
</style>
