<script setup lang="ts">
import { onMounted } from 'vue'
import { NGrid, NGi, NCard, NStatistic } from 'naive-ui'

import HealthSummary from '@/components/health/HealthSummary.vue'
import { useHealthStore } from '@/stores/health'

const healthStore = useHealthStore()

onMounted(() => {
  void healthStore.refresh()
})
</script>

<template>
  <main class="dashboard">
    <section class="dashboard-heading">
      <h1>控制台</h1>
      <p>第一版基础运行面板，用于确认 API、数据库和本地部署状态。</p>
    </section>

    <NGrid cols="1 s:2 l:4" responsive="screen" :x-gap="16" :y-gap="16">
      <NGi>
        <NCard :bordered="false">
          <NStatistic label="仓库" value="0" />
        </NCard>
      </NGi>
      <NGi>
        <NCard :bordered="false">
          <NStatistic label="Release" value="0" />
        </NCard>
      </NGi>
      <NGi>
        <NCard :bordered="false">
          <NStatistic label="资产" value="0" />
        </NCard>
      </NGi>
      <NGi>
        <NCard :bordered="false">
          <NStatistic label="任务" value="0" />
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
