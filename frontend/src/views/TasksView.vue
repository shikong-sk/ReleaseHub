<script setup lang="ts">
import { onMounted, onUnmounted, shallowRef } from 'vue'
import { RefreshCw } from 'lucide-vue-next'

import { listTaskLogs, type TaskLogItem } from '@/api/taskLogs'
import { getAppConfig, updateAppConfig } from '@/api/settings'
import { NAlert, NButton, NCard, NGrid, NGi, NStatistic, NInputNumber, NTag, NSpace, useMessage } from 'naive-ui'
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
let refreshTimer: number | undefined

const message = useMessage()
const retentionLoaded = shallowRef(false)
const retentionDays = shallowRef(30)
const retentionEditing = shallowRef(false)
const retentionSaving = shallowRef(false)
const editRetentionDays = shallowRef(30)

onMounted(() => {
  void tasksStore.refresh()
  void loadRetention()
  refreshTimer = window.setInterval(() => {
    void tasksStore.refresh({ silent: true })
  }, 3000)
})

onUnmounted(() => {
  if (refreshTimer) {
    window.clearInterval(refreshTimer)
  }
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

async function loadRetention() {
  try {
    const config = await getAppConfig()
    retentionDays.value = config.taskLogRetentionDays
    editRetentionDays.value = config.taskLogRetentionDays
    retentionLoaded.value = true
  } catch {
    // 加载失败不阻塞任务页面
  }
}

function startEditRetention() {
  editRetentionDays.value = retentionDays.value
  retentionEditing.value = true
}

function cancelEditRetention() {
  retentionEditing.value = false
}

async function saveRetention() {
  if (editRetentionDays.value < 0) {
    message.warning('保留天数不能小于 0')
    return
  }
  retentionSaving.value = true
  try {
    const result = await updateAppConfig({ taskLogRetentionDays: editRetentionDays.value })
    retentionDays.value = result.taskLogRetentionDays
    retentionEditing.value = false
    message.success('日志保留策略已更新')
  } catch (err) {
    message.error(err instanceof Error ? err.message : '更新失败')
  } finally {
    retentionSaving.value = false
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
      <NButton secondary :loading="tasksStore.loading" @click="() => tasksStore.refresh()">
        <template #icon>
          <RefreshCw />
        </template>
        刷新
      </NButton>
    </section>

    <NGrid cols="1 s:2 l:4" responsive="screen" :x-gap="16" :y-gap="16">
      <NGi>
        <NCard :bordered="false">
          <NStatistic label="总任务" :value="tasksStore.total" />
        </NCard>
      </NGi>
      <NGi>
        <NCard :bordered="false">
          <NStatistic label="排队中" :value="tasksStore.pendingCount" />
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

    <NCard v-if="retentionLoaded" title="日志保留策略" :bordered="false">
      <template #header-extra>
        <NButton v-if="!retentionEditing" secondary size="small" @click="startEditRetention">
          编辑
        </NButton>
      </template>

      <div v-if="retentionEditing" style="display: flex; align-items: center; gap: 12px; flex-wrap: wrap;">
        <NInputNumber v-model:value="editRetentionDays" :min="0" :step="1" style="width: 160px" />
        <span style="color: #8c8c8c">天，0 表示不自动清理</span>
        <NSpace>
          <NButton secondary size="small" @click="cancelEditRetention">
            取消
          </NButton>
          <NButton type="primary" size="small" :loading="retentionSaving" @click="saveRetention">
            保存
          </NButton>
        </NSpace>
      </div>
      <p v-else style="margin: 0; color: #667085">
        超过
        <NTag size="small" :type="retentionDays === 0 ? 'warning' : 'info'">
          {{ retentionDays === 0 ? '不清理' : `${retentionDays} 天` }}
        </NTag>
        的任务日志将被自动删除（每小时检查一次），避免历史日志撑爆数据库。
      </p>
    </NCard>

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
  width: 100%;
  min-width: 0;
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
