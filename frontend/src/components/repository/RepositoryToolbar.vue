<script setup lang="ts">
import { Plus, RefreshCw } from 'lucide-vue-next'
import { NButton, NInput, NSpace } from 'naive-ui'

defineProps<{
  search: string
  loading: boolean
  canWrite: boolean
}>()

const emit = defineEmits<{
  'update:search': [value: string]
  create: []
  refresh: []
}>()
</script>

<template>
  <div class="repository-toolbar">
    <NInput
      class="repository-search"
      :value="search"
      clearable
      placeholder="搜索 owner/repo"
      @update:value="emit('update:search', $event)"
    />
    <NSpace>
      <NButton secondary :loading="loading" @click="emit('refresh')">
        <template #icon>
          <RefreshCw />
        </template>
        刷新
      </NButton>
      <NButton v-if="canWrite" type="primary" @click="emit('create')">
        <template #icon>
          <Plus />
        </template>
        新增仓库
      </NButton>
    </NSpace>
  </div>
</template>

<style scoped>
.repository-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}

.repository-search {
  max-width: 360px;
}

@media (max-width: 760px) {
  .repository-toolbar {
    align-items: stretch;
    flex-direction: column;
  }

  .repository-search {
    max-width: none;
  }
}
</style>
