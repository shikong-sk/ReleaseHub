<script setup lang="ts">
import { NButton, NCard, NDescriptions, NDescriptionsItem, NResult, NSkeleton, NTag } from 'naive-ui'
import { RefreshCw } from 'lucide-vue-next'

import type { HealthStatus } from '@/types/health'

defineProps<{
  status: HealthStatus | null
  databaseStatus: string
  loading: boolean
  error: string | null
}>()

const emit = defineEmits<{
  refresh: []
}>()
</script>

<template>
  <NCard class="health-card" title="本地服务状态" :bordered="false">
    <template #header-extra>
      <NButton secondary :loading="loading" @click="emit('refresh')">
        <template #icon>
          <RefreshCw />
        </template>
        刷新
      </NButton>
    </template>

    <NSkeleton v-if="loading && !status" text :repeat="4" />

    <NResult
      v-else-if="error"
      status="error"
      title="无法连接 API"
      :description="error"
    />

    <NDescriptions v-else-if="status" :column="2" bordered label-placement="left">
      <NDescriptionsItem label="API">
        <NTag :type="status.status === 'ok' ? 'success' : 'warning'">
          {{ status.status }}
        </NTag>
      </NDescriptionsItem>
      <NDescriptionsItem label="数据库">
        <NTag :type="databaseStatus === 'ok' ? 'success' : 'warning'">
          {{ databaseStatus }}
        </NTag>
      </NDescriptionsItem>
      <NDescriptionsItem label="服务">
        {{ status.service }}
      </NDescriptionsItem>
      <NDescriptionsItem label="检查时间">
        {{ new Date(status.checkedAt).toLocaleString() }}
      </NDescriptionsItem>
    </NDescriptions>
  </NCard>
</template>

<style scoped>
.health-card {
  border-radius: 8px;
}
</style>
