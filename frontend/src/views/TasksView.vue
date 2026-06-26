<script setup lang="ts">
import { onMounted, shallowRef } from 'vue'
import { NAlert, NButton, NCard, NGrid, NGi, NStatistic } from 'naive-ui'
import { RefreshCw } from 'lucide-vue-next'

import { listTaskLogs, type TaskLogItem } from '@/api/taskLogs'
import TaskLogDrawer from '@/components/task/TaskLogDrawer.vue'
import TaskTable from '@/components/task/TaskTable.vue'
import { useTasksStore } from '@/stores/tasks'
import type { Task } from '@/types/task'

const tasksStore = useTasksStore()
const selectedTask = shallowRef<Task | null>(null)
const taskLogs = shallowRef<TaskLogItem[]>([])
const logsLoading = shallowRef(false)
const logsError = shallowRef<string | null>(null)
const showLogs = shallowRef(false)

onMounted(() => {
  void tasksStore.refresh()
})

async function handleViewLogs(task: Task) {
  selectedTask.value = task
  showLogs.value = true
  logsLoading.value = true
  logsError.value = null
  taskLogs.value = []

  try {
    const result = await listTaskLogs(task.id, 200)
    taskLogs.value = result.items
  } catch (err) {
    logsError.value = err instanceof Error ? err.message : '任务日志加载失败'
  } finally {
    logsLoading.value = false
  }
}
</script>

<template>
  <main class="tasks-page">
    <section class="tasks-heading">
      <div>
        <h1>任务</h1>
        <p>查看检查、下载等后台任务的状态与错误信息。</p>
      </div>
      <NButton secondary :loading="tasksStore.loading" @click="tasksStore.refresh">
        <template #icon>
          <RefreshCw />
        </template>
        刷新
      </NButton>
    </section>

    <NGrid cols="1 s:2 l:4" responsive="screen" :x-gap="16" :y-gap="16">
      <NGi>
        <NCard :bordered="false">
          <NStatistic label="总任务" :value="tasksStore.items.length" />
        </NCard>
      </NGi>
      <NGi>
        <NCard :bordered="false">
          <NStatistic label="运行中" :value="tasksStore.runningCount" />
        </NCard>
      </NGi>
      <NGi>
        <NCard :bordered="false">
          <NStatistic label="失败" :value="tasksStore.failedCount" />
        </NCard>
      </NGi>
    </NGrid>

    <NAlert v-if="tasksStore.error" type="error" closable>
      {{ tasksStore.error }}
    </NAlert>

    <NCard :bordered="false">
      <TaskTable :tasks="tasksStore.items" :loading="tasksStore.loading" @view-logs="handleViewLogs" />
    </NCard>

    <TaskLogDrawer
      v-model:show="showLogs"
      :task="selectedTask"
      :logs="taskLogs"
      :loading="logsLoading"
      :error="logsError"
    />
  </main>
</template>

<style scoped>
.tasks-page {
  display: flex;
  flex-direction: column;
  gap: 20px;
  max-width: 1180px;
  margin: 0 auto;
}

.tasks-heading {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}

.tasks-heading h1 {
  margin: 0;
  color: #101828;
  font-size: 28px;
  font-weight: 700;
  letter-spacing: 0;
}

.tasks-heading p {
  margin: 6px 0 0;
  color: #667085;
}
</style>
