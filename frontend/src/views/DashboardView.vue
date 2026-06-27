<script setup lang="ts">
import { onMounted, shallowRef } from 'vue'
import { NGrid, NGi, NCard, NStatistic, NSpin, NAlert } from 'naive-ui'

import HealthSummary from '@/components/health/HealthSummary.vue'
import { useHealthStore } from '@/stores/health'
import { useRepositoriesStore } from '@/stores/repositories'
import { useTasksStore } from '@/stores/tasks'
import { useFilesStore } from '@/stores/files'
import { getDashboardStats, type DashboardStats } from '@/api/stats'

const healthStore = useHealthStore()
const repositoryStore = useRepositoriesStore()
const tasksStore = useTasksStore()
const filesStore = useFilesStore()

const stats = shallowRef<DashboardStats | null>(null)
const statsLoading = shallowRef(false)
const statsError = shallowRef<string | null>(null)

function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i]
}

onMounted(async () => {
  void Promise.all([
    healthStore.refresh(),
    repositoryStore.refresh(),
    tasksStore.refresh(),
    filesStore.refresh(),
    tasksStore.refresh()
  ])
  statsLoading.value = true
  try {
    stats.value = await getDashboardStats()
  } catch (err) {
    statsError.value = err instanceof Error ? err.message : '加载统计失败'
  } finally {
    statsLoading.value = false
  }
})
</script>

<template>
  <main class="dashboard">
    <section class="dashboard-heading">
      <h1>控制台</h1>
      <p>ReleaseHub 运行面板，确认服务状态和同步概览。</p>
    </section>

    <NSpin :show="statsLoading">
      <NGrid cols="1 s:2 l:4" responsive="screen" :x-gap="16" :y-gap="16">
        <NGi>
          <NCard :bordered="false">
            <NStatistic label="仓库" :value="stats?.totalRepositories ?? repositoryStore.totalCount" />
          </NCard>
        </NGi>
        <NGi>
          <NCard :bordered="false">
            <NStatistic label="已启用" :value="stats?.enabledRepositories ?? repositoryStore.enabledCount" />
          </NCard>
        </NGi>
        <NGi>
          <NCard :bordered="false">
            <NStatistic label="Release 总数" :value="stats?.totalReleases ?? '-'" />
          </NCard>
        </NGi>
        <NGi>
          <NCard :bordered="false">
            <NStatistic label="存储用量" :value="stats ? formatBytes(stats.totalStorageBytes) : '-'" />
          </NCard>
        </NGi>
        <NGi>
          <NCard :bordered="false">
            <NStatistic label="已下载资产" :value="stats?.downloadedAssets ?? '-'" />
          </NCard>
        </NGi>
        <NGi>
          <NCard :bordered="false">
            <NStatistic label="失败资产" :value="stats?.failedAssets ?? '-'" />
          </NCard>
        </NGi>
        <NGi>
          <NCard :bordered="false">
            <NStatistic label="待处理任务" :value="stats?.pendingTasks ?? '-'" />
          </NCard>
        </NGi>
        <NGi>
          <NCard :bordered="false">
            <NStatistic label="运行中任务" :value="stats?.runningTasks ?? '-'" />
          </NCard>
        </NGi>
      </NGrid>
    </NSpin>

    <NAlert v-if="statsError" type="error" closable>{{ statsError }}</NAlert>

    <NCard v-if="tasksStore.failedCount > 0" :bordered="false" title="最近失败任务">
      <NAlert type="warning" :show-icon="false">
        {{ tasksStore.failedCount }} 个任务失败。请在任务页面查看详情。
      </NAlert>
    </NCard>

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
  width: 100%;
  min-width: 0;
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
