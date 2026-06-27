<script setup lang="ts">
import { computed, shallowRef, watch } from 'vue'
import {
  NButton,
  NInput,
  NModal,
  NRadioButton,
  NRadioGroup,
  NSpace,
  useMessage
} from 'naive-ui'
import { RefreshCw } from 'lucide-vue-next'

import FileTreePanel from '@/components/file/FileTreePanel.vue'
import FileTable from '@/components/file/FileTable.vue'
import { getFileTree } from '@/api/files'
import type { FileTreeNode } from '@/types/file'
import { useFilesStore } from '@/stores/files'
import { useAuthStore } from '@/stores/auth'
import { deleteAsset } from '@/api/releases'
import type { FileItem } from '@/types/file'

const props = defineProps<{
  show: boolean
  storageId?: number | null
  title?: string
}>()

const emit = defineEmits<{
  'update:show': [value: boolean]
}>()

const filesStore = useFilesStore()
const authStore = useAuthStore()
const message = useMessage()

const viewMode = shallowRef<'tree' | 'table'>('tree')
const fileTree = shallowRef<FileTreeNode[]>([])
const fileTreeLoading = shallowRef(false)
const localSearch = shallowRef('')

const modalTitle = computed(() => props.title ?? '文件浏览')

const filteredFiles = computed(() => {
  const keyword = localSearch.value.trim().toLowerCase()
  if (!keyword) return filesStore.items
  return filesStore.items.filter((item) =>
    `${item.owner}/${item.repo}/${item.tag}/${item.name}`.toLowerCase().includes(keyword)
  )
})

watch(
  () => props.show,
  (visible) => {
    if (visible) {
      void loadFileTree()
      void filesStore.refresh()
    }
  },
  { immediate: true }
)

async function loadFileTree() {
  fileTreeLoading.value = true
  try {
    const result = await getFileTree()
    fileTree.value = result.tree ?? []
  } catch {
    // 加载失败不阻塞
  } finally {
    fileTreeLoading.value = false
  }
}

async function handleDeleteFile(file: FileItem) {
  try {
    await deleteAsset(file.assetId)
    message.success(`${file.name} 已删除`)
    void filesStore.refresh()
    void loadFileTree()
  } catch (err) {
    message.error(err instanceof Error ? err.message : '删除失败')
  }
}
</script>

<template>
  <NModal
    :show="show"
    preset="card"
    :title="modalTitle"
    :style="{
      width: '94vw',
      maxWidth: '1800px',
      height: '90vh'
    }"
    :card-style="{
      display: 'flex',
      flexDirection: 'column',
      height: '100%'
    }"
    :content-style="{
      flex: '1',
      overflow: 'auto',
      minHeight: '0'
    }"
    :mask-closable="false"
    @update:show="emit('update:show', $event)"
  >
    <template #header-extra>
      <NSpace align="center" :size="12">
        <NRadioGroup v-model:value="viewMode" size="small">
          <NRadioButton value="tree">树状</NRadioButton>
          <NRadioButton value="table">列表</NRadioButton>
        </NRadioGroup>
        <NButton secondary size="small" :loading="fileTreeLoading" @click="loadFileTree">
          <template #icon><RefreshCw /></template>
          刷新
        </NButton>
        <NButton size="small" @click="emit('update:show', false)">关闭</NButton>
      </NSpace>
    </template>

    <div class="modal-body" style="height: 100%">
      <template v-if="viewMode === 'tree'">
        <FileTreePanel
          :tree="fileTree"
          :loading="fileTreeLoading"
          :can-write="authStore.canWrite"
          :storage-id="storageId"
          @refresh="loadFileTree"
        />
      </template>
      <template v-else>
        <NInput v-model:value="localSearch" class="file-search" clearable placeholder="搜索 owner/repo/tag/name" />
        <FileTable
          class="file-table"
          :files="filteredFiles"
          :loading="filesStore.loading"
          :can-write="authStore.canWrite"
          @delete="handleDeleteFile"
        />
      </template>
    </div>
  </NModal>
</template>

<style scoped>
.modal-body {
  height: 100%;
}

.file-search {
  max-width: 420px;
  margin-bottom: 12px;
}

.file-table {
  margin-top: 8px;
}
</style>
