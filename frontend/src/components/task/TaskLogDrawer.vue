<script setup lang="ts">
import { computed } from 'vue'
import { NAlert, NDrawer, NDrawerContent, NEmpty, NSpin, NTag, NTimeline, NTimelineItem } from 'naive-ui'

import type { TaskLogItem } from '@/api/taskLogs'
import type { Task } from '@/types/task'

const show = defineModel<boolean>('show', { required: true })

const props = defineProps<{
  task: Task | null
  logs: TaskLogItem[]
  loading: boolean
  error: string | null
}>()

const title = computed(() => (props.task ? `任务 #${props.task.id} 日志` : '任务日志'))

function logType(level: string) {
  if (level === 'error') {
    return 'error'
  }
  if (level === 'warn' || level === 'warning') {
    return 'warning'
  }
  return 'info'
}

function formatTime(value: string) {
  return new Date(value).toLocaleString()
}
</script>

<template>
  <NDrawer v-model:show="show" :width="560" placement="right">
    <NDrawerContent :title="title" closable>
      <div v-if="task" class="task-summary">
        <NTag size="small">{{ task.type }}</NTag>
        <NTag size="small" :type="task.status === 'failed' ? 'error' : task.status === 'succeeded' ? 'success' : 'default'">
          {{ task.status }}
        </NTag>
      </div>

      <NAlert v-if="error" type="error" class="log-alert">{{ error }}</NAlert>

      <NSpin :show="loading">
        <NEmpty v-if="!loading && logs.length === 0" description="暂无日志" />
        <NTimeline v-else>
          <NTimelineItem
            v-for="log in logs"
            :key="log.id"
            :type="logType(log.level)"
            :title="log.message"
            :content="formatTime(log.timestamp)"
          />
        </NTimeline>
      </NSpin>
    </NDrawerContent>
  </NDrawer>
</template>

<style scoped>
.task-summary {
  display: flex;
  gap: 8px;
  margin-bottom: 16px;
}

.log-alert {
  margin-bottom: 12px;
}
</style>
