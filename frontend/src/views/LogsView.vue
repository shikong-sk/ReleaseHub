<script setup lang="ts">
import { h, onMounted, shallowRef } from 'vue'
import { NButton, NCard, NDataTable, NInput, NSelect, NTag, useMessage } from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import { RefreshCw } from 'lucide-vue-next'

import { listOperationLogs, type OperationLogItem } from '@/api/operationLogs'

const message = useMessage()
const logs = shallowRef<OperationLogItem[]>([])
const loading = shallowRef(false)
const total = shallowRef(0)
const page = shallowRef(1)
const pageSize = shallowRef(20)

// 筛选条件
const filterStatus = shallowRef<string | null>(null)
const filterKeyword = shallowRef('')

const statusOptions = [
  { label: '全部状态', value: null },
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
      status: filterStatus.value ?? undefined,
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
  filterStatus.value = null
  filterKeyword.value = ''
  page.value = 1
  void loadLogs()
}

function handlePageChange(p: number) {
  page.value = p
  void loadLogs()
}

onMounted(() => {
  void loadLogs()
})
</script>

<template>
  <main class="logs-page">
    <section class="logs-heading">
      <div>
        <h1>系统日志</h1>
        <p>查看用户操作、任务事件等系统级审计记录。</p>
      </div>
      <NButton secondary :loading="loading" @click="loadLogs">
        <template #icon><RefreshCw /></template>
        刷新
      </NButton>
    </section>

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
