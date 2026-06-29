<script setup lang="ts">
import { computed, shallowRef, watch } from 'vue'
import { NDrawer, NDrawerContent, NSelect, NSpace } from 'naive-ui'
import type { SelectOption } from 'naive-ui'

import FileTreePanel from '@/components/file/FileTreePanel.vue'
import { getFileTree, getRepositoryFileTree } from '@/api/files'
import type { FileTreeNode } from '@/types/file'
import type { Repository } from '@/types/repository'
import { useStoragesStore } from '@/stores/storages'

const show = defineModel<boolean>('show', { required: true })

const props = defineProps<{
  repository: Repository | null
}>()

const fileTree = shallowRef<FileTreeNode[]>([])
const fileTreeLoading = shallowRef(false)
// 当前选中的存储 ID，null=全部
const activeStorageId = shallowRef<number | null>(null)

const title = computed(() =>
  props.repository ? `${props.repository.owner}/${props.repository.repo} - 存储文件` : '仓库存储文件'
)

const storagesStore = useStoragesStore()

const storageOptions = computed(() => {
  const options: SelectOption[] = [
    { label: '全部存储', value: null as unknown as number }
  ]
  // 优先使用仓库配置的存储列表
  const repoStorages = props.repository?.storageIds ?? []
  for (const sid of repoStorages) {
    const s = storagesStore.items.find(item => item.id === sid)
    options.push({
      label: s ? `${s.name} (${s.type.toUpperCase()})` : `存储 #${sid}`,
      value: sid
    })
  }
  // 如果仓库没有配置 storageIds，回退到全量存储列表
  if (repoStorages.length === 0) {
    for (const s of storagesStore.items) {
      options.push({ label: `${s.name} (${s.type.toUpperCase()})`, value: s.id })
    }
  }
  return options
})

// 打开抽屉时初始化
watch(
 () => [show.value, props.repository?.id] as const,
 ([visible]) => {
   if (visible) {
     activeStorageId.value = null
     void storagesStore.refresh()
     void loadTree()
    }
 },
 { immediate: true }
)

// 切换存储时重新加载树
watch(activeStorageId, () => {
  if (show.value) {
    void loadTree()
  }
})

let loadSeq = 0

async function loadTree() {
  if (!props.repository) return
  const seq = ++loadSeq
  fileTreeLoading.value = true
  try {
    const result = await getRepositoryFileTree(props.repository.id, activeStorageId.value)
    // 防止旧请求覆盖新请求的结果
    if (seq === loadSeq) {
      fileTree.value = result.tree ?? []
    }
  } catch {
    // 加载失败保留现有数据
  } finally {
    if (seq === loadSeq) {
      fileTreeLoading.value = false
    }
  }
}
</script>

<template>
  <NDrawer v-model:show="show" :width="920" placement="right">
    <NDrawerContent :title="title" closable>
      <NSpace v-if="storageOptions.length > 2" align="center" :size="12" style="margin-bottom: 12px">
        <span style="font-size: 13px; color: #667085; white-space: nowrap">存储</span>
        <NSelect
          v-model:value="activeStorageId"
          :options="storageOptions"
          size="small"
          style="min-width: 200px; max-width: 360px"
          
        />
      </NSpace>
      <FileTreePanel
        :tree="fileTree"
        :loading="fileTreeLoading"
        :can-write="true"
        :storage-id="activeStorageId"
        @refresh="loadTree"
      />
    </NDrawerContent>
  </NDrawer>
</template>
