<script setup lang="ts">
import { h, onMounted, shallowRef } from 'vue'
import {
  NAlert,
  NButton,
  NCard,
  NDataTable,
  NDescriptions,
  NDescriptionsItem,
  NForm,
  NFormItem,
  NInput,
  NModal,
  NPopconfirm,
  NSpace,
  NTag,
  useMessage
} from 'naive-ui'
import { Plus, RefreshCw } from 'lucide-vue-next'
import type { DataTableColumns } from 'naive-ui'

import { getAppConfig } from '@/api/settings'
import { useTokensStore } from '@/stores/tokens'
import type { TokenItem } from '@/types/token'

const tokensStore = useTokensStore()
const message = useMessage()

const configLoaded = shallowRef(false)
const schedulerEnabled = shallowRef(false)
const schedulerTickSeconds = shallowRef(60)
const storageDataDir = shallowRef('')
const githubApiBaseUrl = shallowRef('')

const showModal = shallowRef(false)
const formName = shallowRef('')
const formToken = shallowRef('')

const tokenColumns: DataTableColumns<TokenItem> = [
  { title: '名称', key: 'name', width: 200 },
  { title: 'Token', key: 'tokenHint', ellipsis: { tooltip: true } },
  {
    title: '创建时间',
    key: 'createdAt',
    width: 190,
    render: (row) => (row.createdAt ? new Date(row.createdAt).toLocaleString() : '-')
  },
  {
    title: '操作',
    key: 'actions',
    width: 100,
    render: (row) =>
      h(NPopconfirm, { onPositiveClick: () => handleDelete(row.id) }, {
        trigger: () => h(NButton, { size: 'small', type: 'error', secondary: true, loading: tokensStore.saving }, { default: () => '删除' }),
        default: () => `删除 Token "${row.name}"？`
      })
  }
]

onMounted(async () => {
  void tokensStore.refresh()
  try {
    const config = await getAppConfig()
    schedulerEnabled.value = config.schedulerEnabled
    schedulerTickSeconds.value = config.schedulerTickSeconds
    storageDataDir.value = config.storageDataDir
    githubApiBaseUrl.value = config.githubApiBaseUrl
    configLoaded.value = true
  } catch {
    // 配置加载失败不影响页面
  }
})

function openCreateModal() {
  formName.value = ''
  formToken.value = ''
  showModal.value = true
}

async function handleCreate() {
  if (!formName.value.trim() || !formToken.value.trim()) {
    message.warning('名称和 Token 不能为空')
    return
  }
  try {
    await tokensStore.create({ name: formName.value.trim(), token: formToken.value.trim() })
    message.success('Token 已添加')
    showModal.value = false
  } catch (err) {
    message.error(err instanceof Error ? err.message : '添加 Token 失败')
  }
}

async function handleDelete(id: number) {
  try {
    await tokensStore.remove(id)
    message.success('Token 已删除')
  } catch (err) {
    message.error(err instanceof Error ? err.message : '删除 Token 失败')
  }
}
</script>

<template>
  <main class="settings-page">
    <section class="settings-heading">
      <h1>设置</h1>
      <p>管理 GitHub Token、查看全局配置。</p>
    </section>

    <NCard title="GitHub Token" :bordered="false">
      <template #header-extra>
        <NSpace>
          <NButton secondary :loading="tokensStore.loading" @click="tokensStore.refresh">
            <template #icon><RefreshCw /></template>
            刷新
          </NButton>
          <NButton type="primary" @click="openCreateModal">
            <template #icon><Plus /></template>
            添加 Token
          </NButton>
        </NSpace>
      </template>

      <NAlert v-if="tokensStore.error" type="error" closable>{{ tokensStore.error }}</NAlert>

      <NDataTable
        :columns="tokenColumns"
        :data="tokensStore.items"
        :loading="tokensStore.loading"
        :row-key="(row: TokenItem) => row.id"
        :pagination="{ pageSize: 10 }"
      />
    </NCard>

    <NCard v-if="configLoaded" title="全局配置" :bordered="false">
      <NDescriptions :column="2" bordered label-placement="left">
        <NDescriptionsItem label="Scheduler">
          <NTag :type="schedulerEnabled ? 'success' : 'default'">
            {{ schedulerEnabled ? '已启用' : '已禁用' }}
          </NTag>
        </NDescriptionsItem>
        <NDescriptionsItem label="扫描间隔">
          {{ schedulerTickSeconds }} 秒
        </NDescriptionsItem>
        <NDescriptionsItem label="存储目录">
          {{ storageDataDir }}
        </NDescriptionsItem>
        <NDescriptionsItem label="GitHub API">
          {{ githubApiBaseUrl }}
        </NDescriptionsItem>
      </NDescriptions>
    </NCard>

    <NModal v-model:show="showModal" preset="dialog" title="添加 GitHub Token" positive-text="添加" negative-text="取消" @positive-click="handleCreate">
      <NForm label-placement="left" label-width="auto">
        <NFormItem label="名称">
          <NInput v-model:value="formName" placeholder="例如：personal-token" />
        </NFormItem>
        <NFormItem label="Token">
          <NInput v-model:value="formToken" type="password" show-password-on="click" placeholder="ghp_xxxx" />
        </NFormItem>
      </NForm>
    </NModal>
  </main>
</template>

<style scoped>
.settings-page {
  display: flex;
  flex-direction: column;
  gap: 20px;
  max-width: 1180px;
  margin: 0 auto;
}

.settings-heading {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.settings-heading h1 {
  margin: 0;
  color: #101828;
  font-size: 28px;
  font-weight: 700;
  letter-spacing: 0;
}

.settings-heading p {
  margin: 0;
  color: #667085;
}
</style>
