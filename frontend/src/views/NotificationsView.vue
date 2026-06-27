<script setup lang="ts">
import { h, onMounted, shallowRef } from 'vue'
import {
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
  NSwitch,
  NTag,
  useMessage
} from 'naive-ui'
import { Plus, RefreshCw } from 'lucide-vue-next'
import type { DataTableColumns } from 'naive-ui'
import { computed } from 'vue'

import { useNotificationsStore } from '@/stores/notifications'
import type { NotificationItem, NotificationType } from '@/types/notification'

const notificationsStore = useNotificationsStore()
const message = useMessage()

const showModal = shallowRef(false)
const editingId = shallowRef<number | null>(null)
const formName = shallowRef('')
const formType = shallowRef<NotificationType>('gotify')
const formServerUrl = shallowRef('')
const formToken = shallowRef('')
const formEnabled = shallowRef(true)
const formEvents = shallowRef('*')

const typeOptions = [
  { label: 'Gotify', value: 'gotify' },
  { label: 'Webhook', value: 'webhook' },
  { label: 'Telegram', value: 'telegram' },
  { label: '邮件 (SMTP)', value: 'email' }
]

const typeLabel: Record<NotificationType, string> = {
  gotify: 'Gotify',
  webhook: 'Webhook',
  telegram: 'Telegram',
  email: '邮件'
}

const typeHint = computed(() => {
  switch (formType.value) {
    case 'gotify':
      return 'ServerURL 填 Gotify 地址，Token 幹应用 App Token'
    case 'webhook':
      return 'ServerURL 填 Webhook URL，Token 可选用于认证'
    case 'telegram':
      return 'ServerURL 格式: botToken:chatID'
    case 'email':
      return 'ServerURL 格式: smtp://host:port，Token 格式: user:pass:from:to'
    default:
      return ''
  }
})

const notificationColumns = computed<DataTableColumns<NotificationItem>>(() => [
  { title: '名称', key: 'name', width: 160 },
  {
    title: '类型',
    key: 'type',
    width: 100,
    render: (row) => h(NTag, { type: 'info', size: 'small' }, { default: () => typeLabel[row.type] || row.type })
  },
  { title: '服务地址', key: 'serverUrl', ellipsis: { tooltip: true } },
  {
    title: '状态',
    key: 'enabled',
    width: 80,
    render: (row) =>
      row.enabled
        ? h(NTag, { type: 'success', size: 'small' }, { default: () => '启用' })
        : h(NTag, { type: 'default', size: 'small' }, { default: () => '停用' })
  },
  { title: '事件', key: 'events', width: 120, ellipsis: { tooltip: true } },
  {
    title: '操作',
    key: 'actions',
    width: 240,
    render: (row) =>
      h(NSpace, null, {
        default: () => [
          h(
            NButton,
            { size: 'small', type: 'info', secondary: true, onClick: () => handleTest(row.id) },
            { default: () => '测试' }
          ),
          h(
            NButton,
            { size: 'small', secondary: true, onClick: () => handleEdit(row) },
            { default: () => '编辑' }
          ),
          h(
            NPopconfirm,
            { onPositiveClick: () => handleDelete(row.id) },
            {
              trigger: () => h(NButton, { size: 'small', type: 'error', secondary: true, loading: notificationsStore.saving }, { default: () => '删除' }),
              default: () => `删除通知 "${row.name}"？`
            }
          )
        ]
      })
  }
])

onMounted(() => {
  void notificationsStore.refresh()
})

function openCreateModal() {
  editingId.value = null
  formName.value = ''
  formType.value = 'gotify'
  formServerUrl.value = ''
  formToken.value = ''
  formEnabled.value = true
  formEvents.value = '*'
  showModal.value = true
}

function handleEdit(row: NotificationItem) {
  editingId.value = row.id
  formName.value = row.name
  formType.value = row.type
  formServerUrl.value = row.serverUrl
  formToken.value = ''
  formEnabled.value = row.enabled
  formEvents.value = row.events
  showModal.value = true
}

async function handleSubmit() {
  if (!formName.value.trim()) {
    message.warning('名称不能为空')
    return
  }
  try {
    const payload = {
      name: formName.value.trim(),
      type: formType.value,
      serverUrl: formServerUrl.value.trim() || undefined,
      token: formToken.value || undefined,
      enabled: formEnabled.value,
      events: formEvents.value.trim() || undefined
    }
    if (editingId.value) {
      await notificationsStore.update(editingId.value, payload)
      message.success('通知已更新')
    } else {
      await notificationsStore.create(payload)
      message.success('通知已添加')
    }
    showModal.value = false
  } catch (err) {
    message.error(err instanceof Error ? err.message : '操作失败')
  }
}

async function handleDelete(id: number) {
  try {
    await notificationsStore.remove(id)
    message.success('通知已删除')
  } catch (err) {
    message.error(err instanceof Error ? err.message : '删除通知失败')
  }
}

async function handleTest(id: number) {
  try {
    const result = await notificationsStore.testSend(id)
    message.success(result.message || '测试通知发送成功')
  } catch (err) {
    message.error(err instanceof Error ? err.message : '测试通知发送失败')
  }
}
</script>

<template>
  <main class="notifications-page">
    <section class="notifications-heading">
      <h1>通知</h1>
      <p>配置 Gotify、Webhook、Telegram 和邮件通知渠道。</p>
    </section>

    <NCard title="通知渠道" :bordered="false">
      <template #header-extra>
        <NSpace>
          <NButton secondary :loading="notificationsStore.loading" @click="notificationsStore.refresh">
            <template #icon><RefreshCw /></template>
            刷新
          </NButton>
          <NButton type="primary" @click="openCreateModal">
            <template #icon><Plus /></template>
            添加通知
          </NButton>
        </NSpace>
      </template>

      <NDataTable
        :columns="notificationColumns"
        :data="notificationsStore.items"
        :loading="notificationsStore.loading"
        :row-key="(row: NotificationItem) => row.id"
        :pagination="{ pageSize: 10 }"
      />
    </NCard>

    <NModal v-model:show="showModal" preset="dialog" :title="editingId ? '编辑通知' : '添加通知'" positive-text="保存" negative-text="取消" @positive-click="handleSubmit">
      <NForm label-placement="left" label-width="auto">
        <NFormItem label="名称">
          <NInput v-model:value="formName" placeholder="例如：Gotify 通知" />
        </NFormItem>
        <NFormItem label="类型">
          <NSelect v-model:value="formType" :options="typeOptions" />
        </NFormItem>
        <NFormItem v-if="formType !== 'telegram'" label="服务地址">
          <NInput v-model:value="formServerUrl" placeholder="https://gotify.example.com" />
        </NFormItem>
        <NFormItem v-else label="BotToken:ChatID">
          <NInput v-model:value="formServerUrl" placeholder="123456:ABC-DEF:chat_id" />
        </NFormItem>
        <NFormItem v-if="typeHint" label="提示">
          <span style="color: #667085; font-size: 13px">{{ typeHint }}</span>
        </NFormItem>
        <NFormItem label="Token/密钥">
          <NInput v-model:value="formToken" type="password" show-password-on="click" placeholder="可选" />
        </NFormItem>
        <NFormItem label="启用">
          <NSwitch v-model:value="formEnabled" />
        </NFormItem>
        <NFormItem label="事件">
          <NInput v-model:value="formEvents" placeholder="* 或 new_release,sync_failed" />
        </NFormItem>
      </NForm>
    </NModal>
  </main>
</template>

<style scoped>
.notifications-page {
  display: flex;
  flex-direction: column;
  gap: 20px;
  width: 100%;
  min-width: 0;
}

.notifications-heading {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.notifications-heading h1 {
  margin: 0;
  color: #101828;
  font-size: 28px;
  font-weight: 700;
}

.notifications-heading p {
  margin: 0;
  color: #667085;
}
</style>
