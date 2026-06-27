<script setup lang="ts">
import { computed, h, shallowRef, watch } from 'vue'
import { NDrawer, NDrawerContent } from 'naive-ui'

import FileTreePanel from '@/components/file/FileTreePanel.vue'
import { getFileTree, getRepositoryFileTree } from '@/api/files'
import type { FileTreeNode } from '@/types/file'
import type { Repository } from '@/types/repository'

const show = defineModel<boolean>('show', { required: true })

const props = defineProps<{
  repository: Repository | null
}>()

const fileTree = shallowRef<FileTreeNode[]>([])
const fileTreeLoading = shallowRef(false)

const title = computed(() =>
  props.repository ? `${props.repository.owner}/${props.repository.repo} - 存储文件` : '仓库存储文件'
)

watch(
 () => [show.value, props.repository?.id] as const,
 ([visible]) => {
   if (visible) {
      void loadTree()
    }
  },
  { immediate: true }
)

async function loadTree() {
  if (!props.repository) return
  fileTreeLoading.value = true
  fileTree.value = []
  try {
    const result = await getRepositoryFileTree(props.repository.id)
    fileTree.value = result.tree
  } catch {
    // 加载失败保留空树
  } finally {
    fileTreeLoading.value = false
  }
}
</script>

<template>
  <NDrawer v-model:show="show" :width="920" placement="right">
    <NDrawerContent :title="title" closable>
      <FileTreePanel
        :tree="fileTree"
        :loading="fileTreeLoading"
        :can-write="true"
        @refresh="loadTree"
      />
    </NDrawerContent>
  </NDrawer>
</template>
