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
  NInputNumber,
  NModal,
  NPopconfirm,
  NSpace,
  NSwitch,
  NTag,
  NTooltip,
  useMessage
} from 'naive-ui'
import { Activity, Plus, RefreshCw } from 'lucide-vue-next'
import type { DataTableColumns } from 'naive-ui'

import { getAppConfig, updateAppConfig, type AppConfigUpdate } from '@/api/settings'
import APIKeyPanel from '@/components/settings/APIKeyPanel.vue'
import { checkTokenHealth, checkTokenRateLimit, type TokenHealthResult } from '@/api/tokens'
import { useTokensStore } from '@/stores/tokens'
import type { TokenItem } from '@/types/token'

const tokensStore = useTokensStore()
const message = useMessage()

const configLoaded = shallowRef(false)
const schedulerEnabled = shallowRef(false)
const schedulerTickSeconds = shallowRef(60)
const schedulerMaxConcurrent = shallowRef(5)
const storageDataDir = shallowRef('')
const githubApiBaseUrl = shallowRef('')
const authEnabled = shallowRef(false)
const configSaving = shallowRef(false)
const configEditing = shallowRef(false)

// 编辑表单的临时值
const editSchedulerEnabled = shallowRef(false)
const editSchedulerTickSeconds = shallowRef(60)
const editSchedulerMaxConcurrent = shallowRef(5)
const editGithubApiBaseUrl = shallowRef('')

const showModal = shallowRef(false)
const formName = shallowRef('')
const formToken = shallowRef('')
const healthResults = shallowRef<Record<number, TokenHealthResult>>({})
const healthLoading = shallowRef<Record<number, boolean>>({})

const tokenColumns: DataTableColumns<TokenItem> = [
  { title: '名称', key: 'name', width: 200 },
  { title: 'Token', key: 'tokenHint', ellipsis: { tooltip: true } },
  {
    title: '状态',
    key: 'health',
    width: 120,
    render: (row) => {
      const result = healthResults.value[row.id]
      if (!result) return h('span', { style: 'color: #8c8c8c' }, '-')
      if (result.valid) {
        const rl = result.rateLimit
        const label = rl ? `${rl.remaining}/${rl.limit}` : '有效'
        return h(NTooltip, {}, {
          trigger: () => h(NTag, { size: 'small', type: 'success' }, { default: () => label }),
          default: () => rl
            ? `剩余 ${rl.remaining}/${rl.limit} 次，已用 ${rl.used} 次，重置于 ${new Date(rl.resetAt * 1000).toLocaleString()}`
            : 'Token 有效'
        })
      }
      return h(NTooltip, {}, {
        trigger: () => h(NTag, { size: 'small', type: 'error' }, { default: () => '无效' }),
        default: () => result.error || 'Token 无效'
      })
    }
  },
  {
    title: '创建时间',
    key: 'createdAt',
    width: 190,
    render: (row) => (row.createdAt ? new Date(row.createdAt).toLocaleString() : '-')
  },
  {
    title: '操作',
    key: 'actions',
    width: 160,
    render: (row) =>
      h('div', { style: 'display: flex; gap: 8px;' }, [
        h(NButton, {
          size: 'small',
          secondary: true,
          loading: healthLoading.value[row.id] || false,
          onClick: () => handleHealthCheck(row.id)
        }, {
          icon: () => h(Activity),
          default: () => '检查'
        }),
        h(NPopconfirm, { onPositiveClick: () => handleDelete(row.id) }, {
          trigger: () => h(NButton, { size: 'small', type: 'error', secondary: true, loading: tokensStore.saving }, { default: () => '删除' }),
          default: () => `删除 Token "${row.name}"？`
        })
      ])
  }
]

onMounted(async () => {
  void tokensStore.refresh()
  try {
    const config = await getAppConfig()
    schedulerEnabled.value = config.schedulerEnabled
    schedulerTickSeconds.value = config.schedulerTickSeconds
    schedulerMaxConcurrent.value = config.schedulerMaxConcurrent
    storageDataDir.value = config.storageDataDir
    githubApiBaseUrl.value = config.githubApiBaseUrl
    authEnabled.value = config.authEnabled
    configLoaded.value = true
    resetEditForm()
  } catch {
    // 配置加载失败不影响页面
  }
})

function resetEditForm() {
  editSchedulerEnabled.value = schedulerEnabled.value
  editSchedulerTickSeconds.value = schedulerTickSeconds.value
  editSchedulerMaxConcurrent.value = schedulerMaxConcurrent.value
  editGithubApiBaseUrl.value = githubApiBaseUrl.value
}

function startEditConfig() {
  resetEditForm()
  configEditing.value = true
}

function cancelEditConfig() {
  configEditing.value = false
}

// 保存配置修改
async function saveConfig() {
  configSaving.value = true
  try {
    const update: AppConfigUpdate = {}
    if (editSchedulerEnabled.value !== schedulerEnabled.value) {
      update.schedulerEnabled = editSchedulerEnabled.value
    }
    if (editSchedulerTickSeconds.value !== schedulerTickSeconds.value) {
      update.schedulerTickSeconds = editSchedulerTickSeconds.value
    }
    if (editSchedulerMaxConcurrent.value !== schedulerMaxConcurrent.value) {
      update.schedulerMaxConcurrent = editSchedulerMaxConcurrent.value
    }
    if (editGithubApiBaseUrl.value !== githubApiBaseUrl.value) {
      update.githubApiBaseUrl = editGithubApiBaseUrl.value
    }

    // 没有变更则直接退出编辑
    if (Object.keys(update).length === 0) {
      configEditing.value = false
      return
    }

    const result = await updateAppConfig(update)
    schedulerEnabled.value = result.schedulerEnabled
    schedulerTickSeconds.value = result.schedulerTickSeconds
    schedulerMaxConcurrent.value = result.schedulerMaxConcurrent
    storageDataDir.value = result.storageDataDir
    githubApiBaseUrl.value = result.githubApiBaseUrl
    authEnabled.value = result.authEnabled
    configEditing.value = false
    message.success('配置已更新')
  } catch (err) {
    message.error(err instanceof Error ? err.message : '更新配置失败')
  } finally {
    configSaving.value = false
  }
}

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

async function handleHealthCheck(id: number) {
  healthLoading.value = { ...healthLoading.value, [id]: true }
  try {
    const result = await checkTokenHealth(id)
    healthResults.value = { ...healthResults.value, [id]: result }
    if (result.valid) {
      message.success(`Token 有效${result.rateLimit ? `，剩余 ${result.rateLimit.remaining}/${result.rateLimit.limit}` : ''}`)
    } else {
      message.warning(`Token 无效: ${result.error || '未知错误'}`)
    }
  } catch (err) {
    message.error(err instanceof Error ? err.message : '健康检查失败')
  } finally {
    healthLoading.value = { ...healthLoading.value, [id]: false }
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
      <p>管理访问凭据、修改全局配置。</p>
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

    <APIKeyPanel />

    <NCard v-if="configLoaded" title="全局配置" :bordered="false">
      <template #header-extra>
        <NSpace>
          <NButton v-if="!configEditing" secondary @click="startEditConfig">
            编辑配置
          </NButton>
          <template v-else>
            <NButton secondary @click="cancelEditConfig">
              取消
            </NButton>
            <NButton type="primary" :loading="configSaving" @click="saveConfig">
              保存
            </NButton>
          </template>
        </NSpace>
      </template>

      <!-- 只读展示模式 -->
      <NDescriptions v-if="!configEditing" :column="2" bordered label-placement="left">
        <NDescriptionsItem label="认证">
          <NTag :type="authEnabled ? 'success' : 'default'">
            {{ authEnabled ? '已启用' : '已禁用' }}
          </NTag>
        </NDescriptionsItem>
        <NDescriptionsItem label="Scheduler">
          <NTag :type="schedulerEnabled ? 'success' : 'default'">
            {{ schedulerEnabled ? '已启用' : '已禁用' }}
          </NTag>
        </NDescriptionsItem>
        <NDescriptionsItem label="扫描间隔">
          {{ schedulerTickSeconds }} 秒
        </NDescriptionsItem>
        <NDescriptionsItem label="最大并发">
          {{ schedulerMaxConcurrent }}
        </NDescriptionsItem>
        <NDescriptionsItem label="存储目录">
          {{ storageDataDir }}
        </NDescriptionsItem>
        <NDescriptionsItem label="GitHub API">
          {{ githubApiBaseUrl }}
        </NDescriptionsItem>
      </NDescriptions>

      <!-- 编辑模式 -->
      <NForm v-else label-placement="left" label-width="120">
        <NFormItem label="Scheduler">
          <NSwitch v-model:value="editSchedulerEnabled" />
        </NFormItem>
        <NFormItem label="扫描间隔">
          <NInputNumber v-model:value="editSchedulerTickSeconds" :min="10" :step="10" />
          <span style="margin-left: 8px; color: #8c8c8c">秒</span>
        </NFormItem>
        <NFormItem label="最大并发">
          <NInputNumber v-model:value="editSchedulerMaxConcurrent" :min="1" :max="50" />
        </NFormItem>
        <NFormItem label="GitHub API">
          <NInput v-model:value="editGithubApiBaseUrl" placeholder="https://api.github.com" />
        </NFormItem>
        <NAlert type="info" style="margin-top: 8px">
          认证开关和存储目录为环境变量配置，需重启服务生效，暂不支持在线修改。
        </NAlert>
      </NForm>
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
  width: 100%;
  min-width: 0;
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
