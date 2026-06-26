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
import { Plus, RefreshCw } from 'lucide-vue-next'
import type { DataTableColumns } from 'naive-ui'

import { useStoragesStore } from '@/stores/storages'
import type { StorageItem, StorageType } from '@/types/storage'

const storagesStore = useStoragesStore()
const message = useMessage()

const showModal = shallowRef(false)
const formName = shallowRef('')
const formType = shallowRef<StorageType>('local')
const formBasePath = shallowRef('')
const formIsDefault = shallowRef(false)
const formEndpoint = shallowRef('')
const formBucket = shallowRef('')
const formRegion = shallowRef('')
const formAccessKey = shallowRef('')
const formSecretKey = shallowRef('')
const formUsername = shallowRef('')
const formPassword = shallowRef('')
const formRemoteUrl = shallowRef('')

const typeOptions = [
  { label: '本地存储', value: 'local' },
  { label: 'S3 兼容', value: 's3' },
  { label: 'WebDAV', value: 'webdav' }
]

const storageColumns = computed<DataTableColumns<StorageItem>>(() => [
  { title: '名称', key: 'name', width: 180 },
  {
    title: '类型',
    key: 'type',
    width: 120,
    render: (row) =>
      h(NTag, { type: row.type === 'local' ? 'success' : row.type === 's3' ? 'info' : 'warning' }, {
        default: () => row.type.toUpperCase()
      })
  },
  { title: '路径/Endpoint', key: 'basePath', ellipsis: { tooltip: true } },
  {
    title: '默认',
    key: 'isDefault',
    width: 80,
    render: (row) => row.isDefault ? h(NTag, { type: 'success', size: 'small' }, { default: () => '默认' }) : '-'
  },
  {
    title: '操作',
    key: 'actions',
    width: 200,
    render: (row) =>
      h(NSpace, null, {
        default: () => [
          h(
            NButton,
            {
              size: 'small',
              type: 'info',
              secondary: true,
              onClick: () => handleTest(row.id)
            },
            { default: () => '测试连接' }
          ),
          h(
            NPopconfirm,
            { onPositiveClick: () => handleDelete(row.id) },
            {
              trigger: () => h(NButton, { size: 'small', type: 'error', secondary: true, loading: storagesStore.saving }, { default: () => '删除' }),
              default: () => `删除存储 "${row.name}"？`
            }
          )
        ]
      })
  }
])

import { computed } from 'vue'

const showS3Fields = computed(() => formType.value === 's3')
const showWebdavFields = computed(() => formType.value === 'webdav')
const showLocalFields = computed(() => formType.value === 'local')

onMounted(() => {
  void storagesStore.refresh()
})

function openCreateModal() {
  formName.value = ''
  formType.value = 'local'
  formBasePath.value = ''
  formIsDefault.value = false
  formEndpoint.value = ''
  formBucket.value = ''
  formRegion.value = ''
  formAccessKey.value = ''
  formSecretKey.value = ''
  formUsername.value = ''
  formPassword.value = ''
  formRemoteUrl.value = ''
  showModal.value = true
}

async function handleCreate() {
  if (!formName.value.trim()) {
    message.warning('名称不能为空')
    return
  }
  try {
    await storagesStore.create({
      name: formName.value.trim(),
      type: formType.value,
      basePath: formBasePath.value.trim() || undefined,
      isDefault: formIsDefault.value || undefined,
      endpoint: formEndpoint.value.trim() || undefined,
      bucket: formBucket.value.trim() || undefined,
      region: formRegion.value.trim() || undefined,
      accessKey: formAccessKey.value.trim() || undefined,
      secretKey: formSecretKey.value.trim() || undefined,
      username: formUsername.value.trim() || undefined,
      password: formPassword.value.trim() || undefined,
      remoteUrl: formRemoteUrl.value.trim() || undefined
    })
    message.success('存储已添加')
    showModal.value = false
  } catch (err) {
    message.error(err instanceof Error ? err.message : '添加存储失败')
  }
}

async function handleDelete(id: number) {
  try {
    await storagesStore.remove(id)
    message.success('存储已删除')
  } catch (err) {
    message.error(err instanceof Error ? err.message : '删除存储失败')
  }
}

async function handleTest(id: number) {
  try {
    const result = await storagesStore.testConnection(id)
    message.success(result.message || '连接成功')
  } catch (err) {
    message.error(err instanceof Error ? err.message : '连接测试失败')
  }
}
</script>

<template>
  <main class="storages-page">
    <section class="storages-heading">
      <h1>存储</h1>
      <p>管理本地、S3 兼容和 WebDAV 存储目标。</p>
    </section>

    <NCard title="存储目标" :bordered="false">
      <template #header-extra>
        <NSpace>
          <NButton secondary :loading="storagesStore.loading" @click="storagesStore.refresh">
            <template #icon><RefreshCw /></template>
            刷新
          </NButton>
          <NButton type="primary" @click="openCreateModal">
            <template #icon><Plus /></template>
            添加存储
          </NButton>
        </NSpace>
      </template>

      <NAlert v-if="storagesStore.error" type="error" closable>{{ storagesStore.error }}</NAlert>

      <NDataTable
        :columns="storageColumns"
        :data="storagesStore.items"
        :loading="storagesStore.loading"
        :row-key="(row: StorageItem) => row.id"
        :pagination="{ pageSize: 10 }"
      />
    </NCard>

    <NModal v-model:show="showModal" preset="dialog" title="添加存储" positive-text="添加" negative-text="取消" @positive-click="handleCreate">
      <NForm label-placement="left" label-width="auto">
        <NFormItem label="名称">
          <NInput v-model:value="formName" placeholder="例如：本地存储" />
        </NFormItem>
        <NFormItem label="类型">
          <NSelect v-model:value="formType" :options="typeOptions" />
        </NFormItem>
        <NFormItem v-if="showLocalFields" label="存储路径">
          <NInput v-model:value="formBasePath" placeholder="/data/releases" />
        </NFormItem>
        <NFormItem v-if="showS3Fields" label="Endpoint">
          <NInput v-model:value="formEndpoint" placeholder="https://s3.amazonaws.com" />
        </NFormItem>
        <NFormItem v-if="showS3Fields" label="Bucket">
          <NInput v-model:value="formBucket" placeholder="my-releases" />
        </NFormItem>
        <NFormItem v-if="showS3Fields" label="Region">
          <NInput v-model:value="formRegion" placeholder="us-east-1" />
        </NFormItem>
        <NFormItem v-if="showS3Fields" label="Access Key">
          <NInput v-model:value="formAccessKey" placeholder="AKIAIOSFODNN7EXAMPLE" />
        </NFormItem>
        <NFormItem v-if="showS3Fields" label="Secret Key">
          <NInput v-model:value="formSecretKey" type="password" show-password-on="click" placeholder="wJalrXUtnFEMI/K7MDENG" />
        </NFormItem>
        <NFormItem v-if="showS3Fields" label="前缀路径">
          <NInput v-model:value="formBasePath" placeholder="releasehub" />
        </NFormItem>
        <NFormItem v-if="showWebdavFields" label="WebDAV URL">
          <NInput v-model:value="formRemoteUrl" placeholder="https://dav.example.com/" />
        </NFormItem>
        <NFormItem v-if="showWebdavFields" label="用户名">
          <NInput v-model:value="formUsername" placeholder="admin" />
        </NFormItem>
        <NFormItem v-if="showWebdavFields" label="密码">
          <NInput v-model:value="formPassword" type="password" show-password-on="click" placeholder="password" />
        </NFormItem>
        <NFormItem v-if="showWebdavFields" label="基础路径">
          <NInput v-model:value="formBasePath" placeholder="/releasehub" />
        </NFormItem>
      </NForm>
    </NModal>
  </main>
</template>

<style scoped>
.storages-page {
  display: flex;
  flex-direction: column;
  gap: 20px;
  max-width: 1180px;
  margin: 0 auto;
}

.storages-heading {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.storages-heading h1 {
  margin: 0;
  color: #101828;
  font-size: 28px;
  font-weight: 700;
  letter-spacing: 0;
}

.storages-heading p {
  margin: 0;
  color: #667085;
}
</style>
