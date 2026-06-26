<script setup lang="ts">
import { onMounted, shallowRef } from 'vue'
import { NGrid, NGi, NCard, NStatistic } from 'naive-ui'

import HealthSummary from '@/components/health/HealthSummary.vue'
import { useHealthStore } from '@/stores/health'
import { useRepositoriesStore } from '@/stores/repositories'
import { useTasksStore } from '@/stores/tasks'
import { useFilesStore } from '@/stores/files'

const healthStore = useHealthStore()
const repositoryStore = useRepositoriesStore()
const tasksStore = useTasksStore()
const filesStore = useFilesStore()

onMounted(() => {
  void Promise.all([
    healthStore.refresh(),
    repositoryStore.refresh(),
    tasksStore.refresh(),
    filesStore.refresh()
  ])
})
</script>

<template>
  <main class="dashboard">
    <section class="dashboard-heading">
      <h1>控制台</h1>
      <p>ReleaseHub 运行面板，确认服务状态和同步概览。</p>
    </section>

    <NGrid cols="1 s:2 l:4" responsive="screen" :x-gap="16" :y-gap="16">
      <NGi>
        <NCard :bordered="false">
          <NStatistic label="仓库" :value="repositoryStore.totalCount" />
        </NCard>
      </NGi>
      <NGi>
        <NCard :bordered="false">
          <NStatistic label="已启用" :value="repositoryStore.enabledCount" />
        </NCard>
      </NGi>
      <NGi>
        <NCard :bordered="false">
          <NStatistic label="已同步文件" :value="filesStore.items.length" />
        </NCard>
      </NGi>
      <NGi>
        <NCard :bordered="false">
          <NStatistic label="失败任务" :value="tasksStore.failedCount" />
        </NCard>
      </NGi>
    </NGrid>

    <HealthSummary
      :status="healthStore.status"
      :database-status="healthStore.databaseStatus"
      :loading="healthStore.loading"
      :error="healthStore.error"
      @refresh="healthStore.refresh"
    />
  </main>
</template>

<style scoped>
.dashboard {
  display: flex;
  flex-direction: column;
  gap: 20px;
  max-width: 1180px;
  margin: 0 auto;
}

.dashboard-heading {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.dashboard-heading h1 {
  margin: 0;
  color: #101828;
  font-size: 28px;
  font-weight: 700;
  letter-spacing: 0;
}

.dashboard-heading p {
  margin: 0;
  color: #667085;
}
</style>
