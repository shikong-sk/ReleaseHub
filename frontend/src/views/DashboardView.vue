<script setup lang="ts">
import { computed, onMounted, shallowRef } from 'vue'
import { NGrid, NGi, NCard, NStatistic, NSpin, NAlert } from 'naive-ui'
import { Bar } from 'vue-chartjs'
import {
  Chart as ChartJS,
  CategoryScale,
  LinearScale,
  BarElement,
  Title,
  Tooltip,
  Legend
} from 'chart.js'

import HealthSummary from '@/components/health/HealthSummary.vue'
import { useHealthStore } from '@/stores/health'
import { useRepositoriesStore } from '@/stores/repositories'
import { useTasksStore } from '@/stores/tasks'
import { useFilesStore } from '@/stores/files'
import { getDashboardStats, getTrendStats, type DashboardStats, type TrendStats } from '@/api/stats'

ChartJS.register(CategoryScale, LinearScale, BarElement, Title, Tooltip, Legend)

const healthStore = useHealthStore()
const repositoryStore = useRepositoriesStore()
const tasksStore = useTasksStore()
const filesStore = useFilesStore()

const stats = shallowRef<DashboardStats | null>(null)
const statsLoading = shallowRef(false)
const statsError = shallowRef<string | null>(null)

const trend = shallowRef<TrendStats | null>(null)

const releaseChart = computed(() => {
  const t = trend.value
  if (!t) return null
  return {
    labels: t.releases.map(p => p.date.slice(5)),
    datasets: [{
      label: '新增 Release',
      data: t.releases.map(p => p.count),
      backgroundColor: 'rgba(59, 130, 246, 0.6)',
      borderRadius: 3
    }]
  }
})

const assetChart = computed(() => {
  const t = trend.value
  if (!t) return null
  return {
    labels: t.assets.map(p => p.date.slice(5)),
    datasets: [{
      label: '下载资产',
      data: t.assets.map(p => p.count),
      backgroundColor: 'rgba(16, 185, 129, 0.6)',
      borderRadius: 3
    }]
  }
})

const chartOpts = {
  responsive: true,
  maintainAspectRatio: false,
  plugins: { legend: { display: false } },
  scales: {
    y: { beginAtZero: true, ticks: { precision: 0 } },
    x: { ticks: { maxRotation: 45 } }
  }
}

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
    const [s, t] = await Promise.all([
      getDashboardStats(),
      getTrendStats(30)
    ])
    stats.value = s
    trend.value = t
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

    <!-- 趋势图 -->
    <NGrid v-if="trend" cols="1 l:2" responsive="screen" :x-gap="16" :y-gap="16">
      <NGi>
        <NCard :bordered="false" title="Release 趋势（近 30 天）">
          <div style="height: 220px">
            <Bar v-if="releaseChart" :data="releaseChart" :options="chartOpts" />
          </div>
        </NCard>
      </NGi>
      <NGi>
        <NCard :bordered="false" title="下载趋势（近 30 天）">
          <div style="height: 220px">
            <Bar v-if="assetChart" :data="assetChart" :options="chartOpts" />
          </div>
        </NCard>
      </NGi>
    </NGrid>

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
