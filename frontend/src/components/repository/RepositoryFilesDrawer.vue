<script setup lang="ts">
import { computed, h, watch } from 'vue'
import { NButton, NDataTable, NDrawer, NDrawerContent, NTag } from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'

import { assetFileURL } from '@/api/files'
import { useFilesStore } from '@/stores/files'
import type { FileItem } from '@/types/file'
import type { Repository } from '@/types/repository'

const show = defineModel<boolean>('show', { required: true })

const props = defineProps<{
  repository: Repository | null
}>()

const filesStore = useFilesStore()

const title = computed(() =>
  props.repository ? `${props.repository.owner}/${props.repository.repo} - 存储文件` : '仓库存储文件'
)

const files = computed(() => {
  if (!props.repository) return []
  return filesStore.items.filter((item) => item.repositoryId === props.repository?.id)
})

const columns = computed<DataTableColumns<FileItem>>(() => [
  {
    title: '版本',
    key: 'tag',
    width: 150,
    render: (row) => h(NTag, { size: 'small' }, { default: () => row.tag })
  },
  {
    title: '文件名',
    key: 'name',
    width: 260,
    ellipsis: { tooltip: true }
  },
  {
    title: '大小',
    key: 'size',
    width: 110,
    render: (row) => formatBytes(row.size)
  },
  {
    title: '存储位置',
    key: 'storagePath',
    minWidth: 320,
    ellipsis: { tooltip: true },
    render: (row) => row.storagePath || '-'
  },
  {
    title: '下载',
    key: 'actions',
    width: 100,
    render: (row) =>
      h(NButton, {
        size: 'small',
        type: 'primary',
        secondary: true,
        tag: 'a',
        href: assetFileURL(row.assetId)
      }, { default: () => '下载' })
  }
])

watch(
  () => [show.value, props.repository?.id] as const,
  ([visible]) => {
    if (visible) {
      void filesStore.refresh()
    }
  },
  { immediate: true }
)

function formatBytes(size: number) {
  if (size < 1024) return `${size} B`
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`
  return `${(size / 1024 / 1024).toFixed(1)} MB`
}
</script>

<template>
  <NDrawer v-model:show="show" :width="920" placement="right">
    <NDrawerContent :title="title" closable>
      <NDataTable
        :columns="columns"
        :data="files"
        :loading="filesStore.loading"
        :row-key="(row: FileItem) => row.assetId"
        :pagination="{ pageSize: 12 }"
        :scroll-x="900"
      />
    </NDrawerContent>
  </NDrawer>
</template>
