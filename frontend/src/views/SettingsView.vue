<script setup lang="ts">
import { h, onMounted, shallowRef } from 'vue'
import {
  NAlert,
  NButton,
  NText,
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
import { Activity, Plus, RefreshCw, RotateCw } from 'lucide-vue-next'
import type { DataTableColumns } from 'naive-ui'

import { getAppConfig, updateAppConfig, type AppConfigUpdate } from '@/api/settings'
import { restartService } from '@/api/system'
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
const syncerMaxConcurrentTasks = shallowRef(2)
const syncerMaxConcurrentDownloads = shallowRef(3)
const downloadMaxSpeedBytes = shallowRef(0)
const aria2RPC = shallowRef('')
const aria2Secret = shallowRef('')
const aria2Dir = shallowRef('')
const configSaving = shallowRef(false)
const configEditing = shallowRef(false)
const restarting = shallowRef(false)

// 编辑表单的临时值
const editSchedulerEnabled = shallowRef(false)
const editSchedulerTickSeconds = shallowRef(60)
const editSchedulerMaxConcurrent = shallowRef(5)
const editGithubApiBaseUrl = shallowRef('')
const editAuthEnabled = shallowRef(false)
const editSyncerMaxConcurrentTasks = shallowRef(2)
const editSyncerMaxConcurrentDownloads = shallowRef(3)
const editDownloadMaxSpeedBytes = shallowRef(0)
const editAria2RPC = shallowRef('')
const editAria2Secret = shallowRef('')
const editAria2Dir = shallowRef('')

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
        h(NPopconfirm, { positiveText: "确定", negativeText: "取消", onPositiveClick: () => handleDelete(row.id) }, {
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
    syncerMaxConcurrentTasks.value = config.syncerMaxConcurrentTasks
    syncerMaxConcurrentDownloads.value = config.syncerMaxConcurrentDownloads
    downloadMaxSpeedBytes.value = config.downloadMaxSpeedBytes
    aria2RPC.value = config.aria2RPC
    aria2Secret.value = config.aria2Secret
    aria2Dir.value = config.aria2Dir
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
  editAuthEnabled.value = authEnabled.value
  editSyncerMaxConcurrentTasks.value = syncerMaxConcurrentTasks.value
  editSyncerMaxConcurrentDownloads.value = syncerMaxConcurrentDownloads.value
  editDownloadMaxSpeedBytes.value = downloadMaxSpeedBytes.value
  editAria2RPC.value = aria2RPC.value
  editAria2Secret.value = aria2Secret.value
  editAria2Dir.value = aria2Dir.value
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
    if (editAuthEnabled.value !== authEnabled.value) {
      update.authEnabled = editAuthEnabled.value
    }
    if (editSyncerMaxConcurrentTasks.value !== syncerMaxConcurrentTasks.value) {
      update.syncerMaxConcurrentTasks = editSyncerMaxConcurrentTasks.value
    }
    if (editSyncerMaxConcurrentDownloads.value !== syncerMaxConcurrentDownloads.value) {
      update.syncerMaxConcurrentDownloads = editSyncerMaxConcurrentDownloads.value
    }
    if (editDownloadMaxSpeedBytes.value !== downloadMaxSpeedBytes.value) {
      update.downloadMaxSpeedBytes = editDownloadMaxSpeedBytes.value ?? 0
    }
    if (editAria2RPC.value !== aria2RPC.value) {
      update.aria2RPC = editAria2RPC.value
    }
    if (editAria2Secret.value !== aria2Secret.value) {
      update.aria2Secret = editAria2Secret.value
    }
    if (editAria2Dir.value !== aria2Dir.value) {
      update.aria2Dir = editAria2Dir.value
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
    syncerMaxConcurrentTasks.value = result.syncerMaxConcurrentTasks
    syncerMaxConcurrentDownloads.value = result.syncerMaxConcurrentDownloads
    downloadMaxSpeedBytes.value = result.downloadMaxSpeedBytes
    aria2RPC.value = result.aria2RPC
    aria2Secret.value = result.aria2Secret
    aria2Dir.value = result.aria2Dir
    configEditing.value = false
    message.success('配置已更新')
  } catch (err) {
    message.error(err instanceof Error ? err.message : '更新配置失败')
  } finally {
    configSaving.value = false
  }
}

async function handleRestart() {
  restarting.value = true
  try {
    await restartService()
    message.info('服务正在重启，请稍后刷新页面')
  } catch (err) {
    message.error(err instanceof Error ? err.message : '重启失败')
  } finally {
    restarting.value = false
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
          <NButton v-if="!configEditing" :loading="restarting" @click="handleRestart">
            <template #icon><RotateCw /></template>
            重启服务
          </NButton>
        </NSpace>
      </template>

      <!-- 只读展示模式 -->
      <NDescriptions v-if="!configEditing" :column="2" bordered label-placement="left">
        <NDescriptionsItem label="认证">
          <NTag :type="authEnabled ? 'success' : 'default'">
            {{ authEnabled ? '已启用' : '已禁用' }}
          </NTag>
        </NDescriptionsItem>
        <NDescriptionsItem>
          <template #label>
            Scheduler <NText depth="3" type="warning" style="font-size: 12px">需重启生效</NText>
          </template>
          <NTag :type="schedulerEnabled ? 'success' : 'default'">
            {{ schedulerEnabled ? '已启用' : '已禁用' }}
          </NTag>
        </NDescriptionsItem>
        <NDescriptionsItem label="扫描间隔">
          {{ schedulerTickSeconds }} 秒/轮
        </NDescriptionsItem>
        <NDescriptionsItem label="调度并发数">
          {{ schedulerMaxConcurrent }} 个仓库/轮
        </NDescriptionsItem>
        <NDescriptionsItem label="存储目录">
          {{ storageDataDir }}
        </NDescriptionsItem>
        <NDescriptionsItem>
          <template #label>
            GitHub API <NText depth="3" type="warning" style="font-size: 12px">需重启生效</NText>
          </template>
          {{ githubApiBaseUrl }}
        </NDescriptionsItem>
        <NDescriptionsItem label="任务并发数">
          {{ syncerMaxConcurrentTasks }} 个任务
        </NDescriptionsItem>
        <NDescriptionsItem label="下载并发数">
          {{ syncerMaxConcurrentDownloads }} 个资产/任务
        </NDescriptionsItem>
        <NDescriptionsItem label="下载限速">
          {{ downloadMaxSpeedBytes > 0 ? `${downloadMaxSpeedBytes} B/s` : '不限速' }}
        </NDescriptionsItem>
        <NDescriptionsItem label="Aria2 RPC">
          {{ aria2RPC || '未配置（走 HTTP）' }}
        </NDescriptionsItem>
        <NDescriptionsItem label="Aria2 Secret">
          {{ aria2Secret ? '已配置' : '未配置' }}
        </NDescriptionsItem>
        <NDescriptionsItem label="Aria2 目录">
          {{ aria2Dir || '默认' }}
        </NDescriptionsItem>
      </NDescriptions>

      <!-- 编辑模式 -->
      <NForm v-else label-placement="left" label-width="120">
        <NFormItem label="认证">
          <NSwitch v-model:value="editAuthEnabled" />
        </NFormItem>
        <NFormItem>
          <template #label>
            Scheduler <NText depth="3" type="warning" style="font-size: 12px">需重启生效</NText>
          </template>
          <NSwitch v-model:value="editSchedulerEnabled" />
        </NFormItem>
        <NFormItem label="扫描间隔">
          <NInputNumber v-model:value="editSchedulerTickSeconds" :min="10" :step="10" />
          <span style="margin-left: 8px; color: #8c8c8c">秒/轮，定时器多久扫描一次到期仓库</span>
        </NFormItem>
        <NFormItem label="调度并发数">
          <NInputNumber v-model:value="editSchedulerMaxConcurrent" :min="1" :max="50" />
          <span style="margin-left: 8px; color: #8c8c8c">每轮定时扫描最多同时投递的仓库数</span>
        </NFormItem>
        <NFormItem>
          <template #label>
            GitHub API <NText depth="3" type="warning" style="font-size: 12px">需重启生效</NText>
          </template>
          <NInput v-model:value="editGithubApiBaseUrl" placeholder="https://api.github.com" />
        </NFormItem>
        <NFormItem label="任务并发数">
          <NInputNumber v-model:value="editSyncerMaxConcurrentTasks" :min="1" :max="16" />
          <span style="margin-left: 8px; color: #8c8c8c">任务队列同时执行的任务数</span>
        </NFormItem>
        <NFormItem label="下载并发数">
          <NInputNumber v-model:value="editSyncerMaxConcurrentDownloads" :min="1" :max="32" />
          <span style="margin-left: 8px; color: #8c8c8c">单个任务内同时下载的资产数</span>
        </NFormItem>
        <NFormItem label="下载限速">
          <NInputNumber v-model:value="editDownloadMaxSpeedBytes" :min="0" :step="1048576" />
          <span style="margin-left: 8px; color: #8c8c8c">单位 B/s，0 = 不限速（仅 HTTP 路径生效，aria2 模式不支持）</span>
        </NFormItem>
        <NFormItem>
          <template #label>
            Aria2 RPC <NText depth="3" type="warning" style="font-size: 12px">选填</NText>
          </template>
          <NInput v-model:value="editAria2RPC" placeholder="http://localhost:6800/jsonrpc" />
          <span style="margin-left: 8px; color: #8c8c8c">aria2 JSON-RPC 端点；留空则用 HTTP 下载</span>
        </NFormItem>
        <NFormItem label="Aria2 Secret">
          <NInput v-model:value="editAria2Secret" type="password" placeholder="留空表示无 secret" />
        </NFormItem>
        <NFormItem label="Aria2 目录">
          <NInput v-model:value="editAria2Dir" placeholder="留空使用 aria2 daemon 默认目录" />
        </NFormItem>
        <NAlert type="info" style="margin-top: 8px">
          扫描间隔是全局定时器的轮询周期，每轮检查哪些仓库到期；单个仓库的同步频率由仓库配置中的同步间隔决定。并发控制分三层：调度并发数限制定时扫描投递速率，任务并发数限制后台任务队列执行速率，下载并发数限制单任务内资产下载速率。存储目录为环境变量配置，需重启服务生效。
        </NAlert>
        <NAlert type="info" style="margin-top: 8px">
          Aria2 模式（RPC 端点非空）下下载由 aria2 daemon 独立进程完成：仓库代理配置不生效（请在 aria2 daemon 端配置网络/代理）、下载限速不生效（由 aria2 daemon --max-download-limit 控制）、且 aria2 完成文件目录需对 ReleaseHub 进程可见（本服务通过共享文件系统直接读取后写入存储）。
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
