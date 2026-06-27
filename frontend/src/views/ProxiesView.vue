<script setup lang="ts">
import { computed, h, onMounted, shallowRef } from 'vue'
import {
  NButton,
  NCard,
  NDataTable,
  NForm,
  NFormItem,
  NInput,
  NInputNumber,
  NModal,
  NPopconfirm,
  NSelect,
  NSpace,
  NTag,
  useMessage
} from 'naive-ui'
import { Plus, RefreshCw } from 'lucide-vue-next'
import type { DataTableColumns } from 'naive-ui'

import { useProxiesStore } from '@/stores/proxies'
import type { ProxyItem, ProxyType } from '@/types/proxy'

const proxiesStore = useProxiesStore()
const message = useMessage()

const showModal = shallowRef(false)
const editingId = shallowRef<number | null>(null)
const formName = shallowRef('')
const formType = shallowRef<ProxyType>('http')
const formHost = shallowRef('')
const formPort = shallowRef(7890)
const formUsername = shallowRef('')
const formPassword = shallowRef('')
const formTestUrl = shallowRef('https://api.github.com/rate_limit')
const manualTestUrl = shallowRef('https://api.github.com/rate_limit')

const typeOptions = [
  { label: 'HTTP', value: 'http' },
  { label: 'HTTPS', value: 'https' },
  { label: 'SOCKS5', value: 'socks5' }
]

const proxyColumns = computed<DataTableColumns<ProxyItem>>(() => [
  { title: '名称', key: 'name', width: 180 },
  {
    title: '类型',
    key: 'type',
    width: 100,
    render: (row) =>
      h(NTag, { type: row.type === 'socks5' ? 'warning' : 'info' }, {
        default: () => row.type.toUpperCase()
      })
  },
  {
    title: '地址',
    key: 'address',
    render: (row) => `${row.host}:${row.port}`
  },
  { title: '用户名', key: 'username', width: 120, render: (row) => row.username || '-' },
  {
    title: '测试地址',
    key: 'testUrl',
    width: 260,
    ellipsis: { tooltip: true },
    render: (row) => row.testUrl || 'https://api.github.com/rate_limit'
  },
  {
    title: '最近测试',
    key: 'lastTestStatus',
    width: 240,
    render: (row) => h('div', { class: 'proxy-test-cell' }, [
      h(NTag, { size: 'small', type: row.lastTestStatus === 'ok' ? 'success' : row.lastTestStatus === 'failed' ? 'error' : 'default' }, {
        default: () => row.lastTestStatus === 'ok' ? '成功' : row.lastTestStatus === 'failed' ? '失败' : '未测试'
      }),
      h('span', row.lastTestedAt ? `${row.lastTestLatencyMs} ms · ${new Date(row.lastTestedAt).toLocaleString()}` : '-'),
      row.lastTestMessage ? h('span', { class: 'proxy-test-message', title: row.lastTestMessage }, row.lastTestMessage) : null
    ])
  },
  {
    title: '操作',
    key: 'actions',
    width: 240,
    render: (row) =>
      h(NSpace, null, {
        default: () => [
          h(
            NButton,
            {
              size: 'small',
              type: 'info',
              secondary: true,
              onClick: () => handleTest(row.id, row.testUrl)
            },
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
              trigger: () => h(NButton, { size: 'small', type: 'error', secondary: true, loading: proxiesStore.saving }, { default: () => '删除' }),
              default: () => `删除代理 "${row.name}"？`
            }
          )
        ]
      })
  }
])

onMounted(() => {
  void proxiesStore.refresh()
})

function openCreateModal() {
  editingId.value = null
  formName.value = ''
  formType.value = 'http'
  formHost.value = ''
  formPort.value = 7890
  formUsername.value = ''
  formPassword.value = ''
  formTestUrl.value = 'https://api.github.com/rate_limit'
  manualTestUrl.value = formTestUrl.value
  showModal.value = true
}

function handleEdit(row: ProxyItem) {
  editingId.value = row.id
  formName.value = row.name
  formType.value = row.type
  formHost.value = row.host
  formPort.value = row.port
  formUsername.value = row.username
  formPassword.value = ''
  formTestUrl.value = row.testUrl || 'https://api.github.com/rate_limit'
  manualTestUrl.value = formTestUrl.value
  showModal.value = true
}

async function handleSubmit() {
  if (!formName.value.trim() || !formHost.value.trim()) {
    message.warning('名称和地址不能为空')
    return
  }
  try {
    const payload = {
      name: formName.value.trim(),
      type: formType.value,
      host: formHost.value.trim(),
      port: formPort.value,
      username: formUsername.value.trim() || undefined,
      password: formPassword.value || undefined,
      testUrl: formTestUrl.value.trim() || undefined
    }
    if (editingId.value) {
      await proxiesStore.update(editingId.value, payload)
      message.success('代理已更新')
    } else {
      await proxiesStore.create(payload)
      message.success('代理已添加')
    }
    showModal.value = false
  } catch (err) {
    message.error(err instanceof Error ? err.message : '操作失败')
  }
}

async function handleDelete(id: number) {
  try {
    await proxiesStore.remove(id)
    message.success('代理已删除')
  } catch (err) {
    message.error(err instanceof Error ? err.message : '删除代理失败')
  }
}

async function handleTest(id: number, testUrl?: string) {
  try {
    const result = await proxiesStore.testConnection(id, testUrl?.trim() || undefined)
    message.success(result.message || '代理连接成功')
  } catch (err) {
    message.error(err instanceof Error ? err.message : '代理测试失败')
  }
}
</script>

<template>
  <main class="proxies-page">
    <section class="proxies-heading">
      <h1>代理</h1>
      <p>管理 HTTP/HTTPS/SOCKS5 代理，用于访问 GitHub API 和下载资源。</p>
    </section>

    <NCard title="代理列表" :bordered="false">
      <template #header-extra>
        <NSpace>
          <NButton secondary :loading="proxiesStore.loading" @click="proxiesStore.refresh">
            <template #icon><RefreshCw /></template>
            刷新
          </NButton>
          <NButton type="primary" @click="openCreateModal">
            <template #icon><Plus /></template>
            添加代理
          </NButton>
        </NSpace>
      </template>

      <NDataTable
        :columns="proxyColumns"
        :data="proxiesStore.items"
        :loading="proxiesStore.loading"
        :row-key="(row: ProxyItem) => row.id"
        :pagination="{ pageSize: 10 }"
        :scroll-x="1280"
      />
    </NCard>

    <NModal v-model:show="showModal" preset="dialog" :title="editingId ? '编辑代理' : '添加代理'" positive-text="保存" negative-text="取消" @positive-click="handleSubmit">
      <NForm label-placement="left" label-width="auto">
        <NFormItem label="名称">
          <NInput v-model:value="formName" placeholder="例如：公司代理" />
        </NFormItem>
        <NFormItem label="类型">
          <NSelect v-model:value="formType" :options="typeOptions" />
        </NFormItem>
        <NFormItem label="主机">
          <NInput v-model:value="formHost" placeholder="proxy.example.com" />
        </NFormItem>
        <NFormItem label="端口">
          <NInputNumber v-model:value="formPort" :min="1" :max="65535" />
        </NFormItem>
        <NFormItem label="用户名">
          <NInput v-model:value="formUsername" placeholder="可选" />
        </NFormItem>
        <NFormItem label="密码">
          <NInput v-model:value="formPassword" type="password" show-password-on="click" placeholder="可选" />
        </NFormItem>
        <NFormItem label="默认测试地址">
          <NInput v-model:value="formTestUrl" placeholder="https://api.github.com/rate_limit" />
        </NFormItem>
        <NFormItem label="本次测试地址">
          <NSpace vertical class="proxy-test-input">
            <NInput v-model:value="manualTestUrl" placeholder="留空则使用默认测试地址" />
            <NButton
              size="small"
              secondary
              :disabled="!editingId"
              @click="editingId && handleTest(editingId, manualTestUrl)"
            >
              测试当前地址
            </NButton>
          </NSpace>
        </NFormItem>
      </NForm>
    </NModal>
  </main>
</template>

<style scoped>
.proxies-page {
  display: flex;
  flex-direction: column;
  gap: 20px;
  width: 100%;
  min-width: 0;
}

.proxies-heading {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.proxies-heading h1 {
  margin: 0;
  color: #101828;
  font-size: 28px;
  font-weight: 700;
}

.proxies-heading p {
  margin: 0;
  color: #667085;
}

:deep(.proxy-test-cell) {
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
}

:deep(.proxy-test-cell span) {
  overflow: hidden;
  color: #667085;
  font-size: 12px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.proxy-test-input {
  width: 100%;
}
</style>
