<script setup lang="ts">
import { h, onMounted, onUnmounted, shallowRef } from 'vue'
import { NButton, NCard, NDataTable, NInput, NInputNumber, NSelect, NSpace, NSwitch, NTag, useMessage } from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import { RefreshCw } from 'lucide-vue-next'

import { listOperationLogs, type OperationLogItem } from '@/api/operationLogs'
import { getAppConfig, updateAppConfig } from '@/api/settings'

const message = useMessage()
const logs = shallowRef<OperationLogItem[]>([])
const loading = shallowRef(false)
const total = shallowRef(0)
const page = shallowRef(1)
const pageSize = shallowRef(20)

// 筛选条件
const filterStatus = shallowRef<string>('')
const filterKeyword = shallowRef('')

const statusOptions = [
  { label: '全部状态', value: '' },
  { label: '成功', value: 'success' },
  { label: '失败', value: 'failed' }
]

const columns: DataTableColumns<OperationLogItem> = [
  { title: '时间', key: 'createdAt', width: 180 },
  {
    title: '操作者',
    key: 'actor',
    width: 120,
    render: (row) => row.actor || 'system'
  },
  { title: '操作', key: 'action', width: 160 },
  { title: '资源', key: 'resource', width: 160, ellipsis: { tooltip: true } },
  { title: '详情', key: 'detail', ellipsis: { tooltip: true } },
  {
    title: '状态',
    key: 'status',
    width: 90,
    render: (row) =>
      h(
        NTag,
        { size: 'small', type: row.status === 'success' ? 'success' : 'error' },
        { default: () => (row.status === 'success' ? '成功' : '失败') }
      )
  },
  {
    title: '来源 IP',
    key: 'clientIp',
    width: 130,
    render: (row) => row.clientIp || '-'
  }
]

async function loadLogs() {
  loading.value = true
  try {
    const result = await listOperationLogs({
      status: filterStatus.value || undefined,
      keyword: filterKeyword.value.trim() || undefined,
      page: page.value,
      pageSize: pageSize.value
    })
    logs.value = result.items
    total.value = result.total
  } catch (err) {
    message.error(err instanceof Error ? err.message : '加载操作日志失败')
  } finally {
    loading.value = false
  }
}

function applyFilters() {
  page.value = 1
  void loadLogs()
}

function resetFilters() {
  filterStatus.value = ''
  filterKeyword.value = ''
  page.value = 1
  void loadLogs()
}

function handlePageChange(p: number) {
  page.value = p
  void loadLogs()
}

// ===== 操作日志保留策略 =====
const retentionLoaded = shallowRef(false)
const retentionDays = shallowRef(30)
const retentionEditing = shallowRef(false)
const retentionSaving = shallowRef(false)
const editRetentionDays = shallowRef(30)

async function loadRetention() {
  try {
    const config = await getAppConfig()
    retentionDays.value = config.operationLogRetentionDays
    editRetentionDays.value = config.operationLogRetentionDays
    retentionLoaded.value = true
  } catch {
    // 加载失败不阻塞日志页面
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
    const result = await updateAppConfig({ operationLogRetentionDays: editRetentionDays.value })
    retentionDays.value = result.operationLogRetentionDays
    retentionEditing.value = false
    message.success('操作日志保留策略已更新')
  } catch (err) {
    message.error(err instanceof Error ? err.message : '更新失败')
  } finally {
    retentionSaving.value = false
  }
}

// ===== 实时刷新 =====
const autoRefresh = shallowRef(false)
const refreshInterval = shallowRef(3) // 秒
let refreshTimer: ReturnType<typeof setInterval> | undefined

function startAutoRefresh() {
  if (refreshTimer) {
    clearInterval(refreshTimer)
  }
  if (autoRefresh.value) {
    refreshTimer = setInterval(() => {
      void loadLogs()
    }, refreshInterval.value * 1000)
  }
}

function stopAutoRefresh() {
  if (refreshTimer) {
    clearInterval(refreshTimer)
    refreshTimer = undefined
  }
}

function toggleAutoRefresh(val: boolean) {
  autoRefresh.value = val
  if (val) {
    startAutoRefresh()
  } else {
    stopAutoRefresh()
  }
}

onMounted(() => {
  void loadLogs()
  void loadRetention()
})

onUnmounted(() => {
  stopAutoRefresh()
})
</script>

<template>
  <main class="logs-page">
    <section class="logs-heading">
      <div>
        <h1>系统日志</h1>
        <p>查看用户操作、任务事件等系统级审计记录。</p>
      </div>
      <div style="display: flex; align-items: center; gap: 16px;">
        <div style="display: flex; align-items: center; gap: 8px;">
          <NSwitch :value="autoRefresh" @update:value="toggleAutoRefresh" size="small" />
          <span style="color: #667085; font-size: 13px; white-space: nowrap;">实时刷新</span>
        </div>
        <NButton secondary :loading="loading" @click="loadLogs">
          <template #icon><RefreshCw /></template>
          刷新
        </NButton>
      </div>
    </section>

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
        的操作日志将被自动删除（每小时检查一次），避免历史日志撑爆数据库。
      </p>
    </NCard>

    <NCard :bordered="false">
      <div style="display: flex; align-items: center; gap: 12px; flex-wrap: wrap; margin-bottom: 16px;">
        <NSelect
          v-model:value="filterStatus"
          :options="statusOptions"
          placeholder="状态"
          style="width: 150px"
          @update:value="applyFilters"
        />
        <NInput
          v-model:value="filterKeyword"
          placeholder="搜索描述 / 资源"
          clearable
          style="width: 260px"
          @keyup.enter="applyFilters"
          @clear="applyFilters"
        />
        <NButton secondary size="small" @click="resetFilters">重置</NButton>
      </div>

      <NDataTable
        :columns="columns"
        :data="logs"
        :loading="loading"
        :row-key="(row: OperationLogItem) => row.id"
        :pagination="{
          page: page,
          itemCount: total,
          pageSize: pageSize,
          showSizePicker: false,
          prefix: ({ itemCount }) => `共 ${itemCount} 条`
        }"
        :remote="true"
        @update:page="handlePageChange"
      />
    </NCard>
  </main>
</template>

<style scoped>
.logs-page {
  display: flex;
  flex-direction: column;
  gap: 20px;
  width: 100%;
  min-width: 0;
}

.logs-heading {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}

.logs-heading h1 {
  margin: 0;
  color: #101828;
  font-size: 28px;
  font-weight: 700;
  letter-spacing: 0;
}

.logs-heading p {
  margin: 6px 0 0;
  color: #667085;
}
</style>
