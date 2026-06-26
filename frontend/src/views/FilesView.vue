<script setup lang="ts">
import { computed, onMounted, shallowRef } from 'vue'
import { NAlert, NButton, NCard, NGrid, NGi, NInput, NStatistic } from 'naive-ui'
import { RefreshCw } from 'lucide-vue-next'

import FileTable from '@/components/file/FileTable.vue'
import { useFilesStore } from '@/stores/files'

const filesStore = useFilesStore()
const search = shallowRef('')

const filteredFiles = computed(() => {
  const keyword = search.value.trim().toLowerCase()
  if (!keyword) {
    return filesStore.items
  }

  return filesStore.items.filter((item) =>
    `${item.owner}/${item.repo}/${item.tag}/${item.name}`.toLowerCase().includes(keyword)
  )
})

onMounted(() => {
  void filesStore.refresh()
})

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
  <main class="files-page">
    <section class="files-heading">
      <div>
        <h1>文件</h1>
        <p>浏览已同步到本地存储的 Release Assets。</p>
      </div>
      <NButton secondary :loading="filesStore.loading" @click="filesStore.refresh">
        <template #icon>
          <RefreshCw />
        </template>
        刷新
      </NButton>
    </section>

    <NGrid cols="1 s:2 l:4" responsive="screen" :x-gap="16" :y-gap="16">
      <NGi>
        <NCard :bordered="false">
          <NStatistic label="文件数" :value="filesStore.items.length" />
        </NCard>
      </NGi>
      <NGi>
        <NCard :bordered="false">
          <NStatistic label="总大小" :value="formatBytes(filesStore.totalSize)" />
        </NCard>
      </NGi>
    </NGrid>

    <NAlert v-if="filesStore.error" type="error" closable>
      {{ filesStore.error }}
    </NAlert>

    <NCard :bordered="false">
      <NInput v-model:value="search" class="file-search" clearable placeholder="搜索 owner/repo/tag/name" />
      <FileTable class="file-table" :files="filteredFiles" :loading="filesStore.loading" />
    </NCard>
  </main>
</template>

<style scoped>
.files-page {
  display: flex;
  flex-direction: column;
  gap: 20px;
  max-width: 1180px;
  margin: 0 auto;
}

.files-heading {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}

.files-heading h1 {
  margin: 0;
  color: #101828;
  font-size: 28px;
  font-weight: 700;
  letter-spacing: 0;
}

.files-heading p {
  margin: 6px 0 0;
  color: #667085;
}

.file-search {
  max-width: 420px;
}

.file-table {
  margin-top: 16px;
}
</style>
