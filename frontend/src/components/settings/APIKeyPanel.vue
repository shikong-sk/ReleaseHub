<script setup lang="ts">
import { h, onMounted, shallowRef } from 'vue'
import {
  NAlert,
  NButton,
  NCard,
  NDataTable,
  NForm,
  NFormItem,
  NInput,
  NModal,
  NPopconfirm,
  NSelect,
  NSpace,
  NTag,
  useMessage
} from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import { Plus, RefreshCw } from 'lucide-vue-next'

import { useAPIKeysStore } from '@/stores/apikeys'
import type { APIKeyItem } from '@/types/apikey'

const apiKeysStore = useAPIKeysStore()
const message = useMessage()

const showCreateModal = shallowRef(false)
const showCreatedModal = shallowRef(false)
const formName = shallowRef('')
const formScope = shallowRef('*')
const createdKey = shallowRef('')
const scopeOptions = [
  { label: '全部权限 (*)', value: '*' },
  { label: '只读 (read)', value: 'read' },
  { label: '同步与下载 (write)', value: 'write' },
  { label: '仓库写入 (repo:write)', value: 'repo:write' },
  { label: '资产下载 (asset:download)', value: 'asset:download' },
  { label: '管理权限 (admin)', value: 'admin' }
]

const columns: DataTableColumns<APIKeyItem> = [
  { title: '名称', key: 'name', width: 180 },
  { title: 'Key', key: 'keyHint', width: 180 },
  { title: 'Scope', key: 'scope', width: 160 },
  {
    title: '状态',
    key: 'enabled',
    width: 100,
    render: (row) => h(NTag, { type: row.enabled ? 'success' : 'default' }, { default: () => (row.enabled ? '启用' : '禁用') })
  },
  {
    title: '最后使用',
    key: 'lastUsedAt',
    width: 190,
    render: (row) => (row.lastUsedAt ? new Date(row.lastUsedAt).toLocaleString() : '-')
  },
  {
    title: '创建时间',
    key: 'createdAt',
    width: 190,
    render: (row) => new Date(row.createdAt).toLocaleString()
  },
  {
    title: '操作',
    key: 'actions',
    width: 100,
    render: (row) =>
      h(NPopconfirm, { onPositiveClick: () => handleDelete(row.id) }, {
        trigger: () => h(NButton, { size: 'small', type: 'error', secondary: true, loading: apiKeysStore.saving }, { default: () => '删除' }),
        default: () => `删除 API Key "${row.name}"？`
      })
  }
]

onMounted(() => {
  void apiKeysStore.refresh()
})

function openCreateModal() {
  formName.value = ''
  formScope.value = '*'
  showCreateModal.value = true
}

async function handleCreate() {
  if (!formName.value.trim()) {
    message.warning('名称不能为空')
    return
  }

  try {
    const created = await apiKeysStore.create({
      name: formName.value.trim(),
      scope: formScope.value.trim() || '*'
    })
    createdKey.value = created.key
    showCreateModal.value = false
    showCreatedModal.value = true
    message.success('API Key 已创建')
  } catch (err) {
    message.error(err instanceof Error ? err.message : '创建 API Key 失败')
  }
}

async function handleDelete(id: number) {
  try {
    await apiKeysStore.remove(id)
    message.success('API Key 已删除')
  } catch (err) {
    message.error(err instanceof Error ? err.message : '删除 API Key 失败')
  }
}
</script>

<template>
  <NCard title="API Key" :bordered="false">
    <template #header-extra>
      <NSpace>
        <NButton secondary :loading="apiKeysStore.loading" @click="apiKeysStore.refresh">
          <template #icon><RefreshCw /></template>
          刷新
        </NButton>
        <NButton type="primary" @click="openCreateModal">
          <template #icon><Plus /></template>
          添加 Key
        </NButton>
      </NSpace>
    </template>

    <NAlert v-if="apiKeysStore.error" type="error" closable>{{ apiKeysStore.error }}</NAlert>

    <NDataTable
      :columns="columns"
      :data="apiKeysStore.items"
      :loading="apiKeysStore.loading"
      :row-key="(row: APIKeyItem) => row.id"
      :pagination="{ pageSize: 10 }"
    />

    <NModal v-model:show="showCreateModal" preset="dialog" title="添加 API Key" positive-text="添加" negative-text="取消" @positive-click="handleCreate">
      <NForm label-placement="left" label-width="auto">
        <NFormItem label="名称">
          <NInput v-model:value="formName" placeholder="例如：automation" />
        </NFormItem>
        <NFormItem label="Scope">
          <NSelect v-model:value="formScope" filterable tag :options="scopeOptions" placeholder="例如：*" />
        </NFormItem>
      </NForm>
    </NModal>

    <NModal v-model:show="showCreatedModal" preset="dialog" title="API Key 已创建" positive-text="关闭">
      <NAlert type="warning" :show-icon="false">
        完整 Key 仅显示一次。
      </NAlert>
      <NInput class="created-key" :value="createdKey" readonly type="textarea" :autosize="{ minRows: 2, maxRows: 4 }" />
    </NModal>
  </NCard>
</template>

<style scoped>
.created-key {
  margin-top: 12px;
}
</style>
