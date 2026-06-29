<script setup lang="ts">
import { computed, h, onMounted, shallowRef } from 'vue'
import {
  NAlert,
  NButton,
  NCard,
  NCheckbox,
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
import { FolderOpen, Plus, RefreshCw, Wrench } from 'lucide-vue-next'
import type { DataTableColumns } from 'naive-ui'

import FileTreeModal from '@/components/file/FileTreeModal.vue'
import { getAppConfig } from '@/api/settings'
import { runReconcile, type ReconcileResult, type ReconcileItem } from '@/api/reconcile'
import { useStoragesStore } from '@/stores/storages'
import type { StorageItem, StorageType } from '@/types/storage'

const storagesStore = useStoragesStore()
const message = useMessage()

const showModal = shallowRef(false)
const editingId = shallowRef<number | null>(null)
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
const defaultDataDir = shallowRef('data/releases')

// 全屏文件树弹窗
const showFileTreeModal = shallowRef(false)
const fileTreeStorageId = shallowRef<number | null>(null)
const fileTreeTitle = shallowRef('文件浏览')

// 存储修复
const showReconcileModal = shallowRef(false)
const reconcileLoading = shallowRef(false)
const reconcileDryRun = shallowRef(true)
const reconcileResult = shallowRef<ReconcileResult | null>(null)

const typeOptions = [
  { label: '本地存储', value: 'local' },
  { label: 'S3 兼容', value: 's3' },
  { label: 'WebDAV', value: 'webdav' }
]

const tableData = computed<StorageItem[]>(() => {
  const hasDefaultLocal = storagesStore.items.some((item) => item.type === 'local' && item.isDefault)
  if (hasDefaultLocal) {
    return storagesStore.items
  }

  return [
    {
      id: 0,
      name: '默认本地存储',
      type: 'local',
      basePath: defaultDataDir.value,
      isDefault: true,
      endpoint: '',
      bucket: '',
      region: '',
      accessKeyHint: '',
      username: '',
      remoteUrl: '',
      builtin: true,
      createdAt: '',
      updatedAt: ''
    },
    ...storagesStore.items
  ]
})

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
  {
    title: '来源',
    key: 'source',
    width: 100,
    render: (row) => row.builtin ? h(NTag, { size: 'small' }, { default: () => '系统内置' }) : '自定义'
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
    width: 280,
    render: (row) =>
      h(NSpace, null, {
        default: () => [
          h(
            NButton,
            {
              size: 'small',
              type: 'info',
              secondary: true,
              disabled: row.builtin,
              onClick: () => handleTest(row.id)
            },
            { default: () => row.builtin ? '无需测试' : '测试连接' }
          ),
          h(
            NButton,
            { size: 'small', secondary: true, disabled: row.builtin, onClick: () => openEditModal(row) },
            { default: () => '编辑' }
          ),
          h(
            NButton,
            { size: 'small', secondary: true, onClick: () => viewStorageFiles(row) },
            { icon: () => h(FolderOpen, { size: 14 }), default: () => '文件' }
          ),
          h(
            NPopconfirm,
            { positiveText: '确定', negativeText: '取消', onPositiveClick: () => handleDelete(row.id) },
            {
              trigger: () => h(NButton, { size: 'small', type: 'error', secondary: true, disabled: row.builtin, loading: storagesStore.saving }, { default: () => '删除' }),
              default: () => `删除存储 "${row.name}"？`
            }
          )
        ]
      })
  }
])

const showS3Fields = computed(() => formType.value === 's3')
const showWebdavFields = computed(() => formType.value === 'webdav')
const showLocalFields = computed(() => formType.value === 'local')

onMounted(() => {
  void storagesStore.refresh()
  void loadConfig()
})

function viewStorageFiles(row: StorageItem) {
  // 内置默认本地存储（id=0）不存在于后端数据库，不传 storageId
  // 其他存储直接传实际 ID
  fileTreeStorageId.value = row.id > 0 ? row.id : null
  fileTreeTitle.value = `${row.name} - 文件浏览`
  showFileTreeModal.value = true
}

async function loadConfig() {
  try {
    const config = await getAppConfig()
    defaultDataDir.value = config.storageDataDir || defaultDataDir.value
  } catch {
    defaultDataDir.value = 'data/releases'
  }
}

function openCreateModal() {
  editingId.value = null
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

function openEditModal(row: StorageItem) {
  editingId.value = row.id
  formName.value = row.name
  formType.value = row.type as StorageType
  formBasePath.value = row.basePath
  formIsDefault.value = row.isDefault
  formEndpoint.value = row.endpoint || ''
  formBucket.value = row.bucket || ''
  formRegion.value = row.region || ''
  formAccessKey.value = ''
  formSecretKey.value = ''
  formUsername.value = row.username || ''
  formPassword.value = ''
  formRemoteUrl.value = row.remoteUrl || ''
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
      basePath: formBasePath.value.trim() || undefined,
      isDefault: formIsDefault.value || undefined,
      endpoint: formEndpoint.value.trim() || undefined,
      bucket: formBucket.value.trim() || undefined,
      region: formRegion.value.trim() || undefined,
      accessKey: formAccessKey.value.trim() || undefined,
      secretKey: formSecretKey.value.trim() || undefined,
      username: formUsername.value.trim() || undefined,
      password: formPassword.value || undefined,
      remoteUrl: formRemoteUrl.value.trim() || undefined
    }
    if (editingId.value) {
      await storagesStore.update(editingId.value, payload)
      message.success('存储已更新')
    } else {
      await storagesStore.create(payload)
      message.success('存储已添加')
    }
    showModal.value = false
  } catch (err) {
    message.error(err instanceof Error ? err.message : '操作失败')
  }
}

async function handleDelete(id: number) {
  if (id === 0) return
  try {
    await storagesStore.remove(id)
    message.success('存储已删除')
  } catch (err) {
    message.error(err instanceof Error ? err.message : '删除存储失败')
  }
}

async function handleTest(id: number) {
  if (id === 0) {
    message.info('默认本地存储来自系统配置，无需测试连接')
    return
  }
  try {
    const result = await storagesStore.testConnection(id)
    message.success(result.message || '连接成功')
  } catch (err) {
    message.error(err instanceof Error ? err.message : '连接测试失败')
  }
}

async function handleReconcile() {
  reconcileLoading.value = true
  reconcileResult.value = null
  try {
    const result = await runReconcile(reconcileDryRun.value)
    reconcileResult.value = result
  } catch (err) {
    message.error(err instanceof Error ? err.message : '存储校验失败')
  } finally {
    reconcileLoading.value = false
  }
}

const reconcileItemColumns = computed<DataTableColumns<ReconcileItem>>(() => [
  { title: '存储', key: 'storageName', width: 120 },
  { title: '路径', key: 'path', ellipsis: { tooltip: true } },
  { title: 'Owner', key: 'owner', width: 100 },
  { title: 'Repo', key: 'repo', width: 120 },
  { title: 'Tag', key: 'tag', width: 100 },
  { title: '文件', key: 'filename', ellipsis: { tooltip: true } },
  { title: '资产ID', key: 'assetId', width: 80 }
])
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
          <NButton secondary @click="showReconcileModal = true; reconcileResult = null">
            <template #icon><Wrench /></template>
            修复存储
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
        :data="tableData"
        :loading="storagesStore.loading"
        :row-key="(row: StorageItem) => row.id"
        :pagination="{ pageSize: 10 }"
        :scroll-x="980"
      />
    </NCard>

    <!-- 全屏文件树弹窗 -->
    <FileTreeModal
      v-model:show="showFileTreeModal"
      :storage-id="fileTreeStorageId"
      :title="fileTreeTitle"
    />

    <!-- 存储修复弹窗 -->
    <NModal v-model:show="showReconcileModal" preset="card" title="存储校验与修复" style="width: 90vw; max-width: 1200px">
      <NSpace vertical :size="16">
        <NAlert type="info" :bordered="false">
          校验存储与数据库的一致性：检测存储中存在但 DB 缺失的文件、DB 中存在但存储缺失的资产、以及状态异常的记录。
        </NAlert>

        <NSpace align="center">
          <NCheckbox v-model:checked="reconcileDryRun">安全预检模式（不实际修改）</NCheckbox>
          <NButton type="primary" :loading="reconcileLoading" @click="handleReconcile">
            开始校验
          </NButton>
        </NSpace>

        <template v-if="reconcileResult">
          <NSpace vertical :size="8">
            <NTag :type="reconcileResult.dryRun ? 'warning' : 'success'" size="small">
              {{ reconcileResult.dryRun ? '预检模式' : '修复模式' }}
            </NTag>
            <span>存储文件: {{ reconcileResult.totalStorageFiles }} | DB 资产: {{ reconcileResult.totalDBAssets }}</span>
          </NSpace>

          <template v-if="reconcileResult.missingInStorage.length > 0">
            <h4 style="margin: 0; color: #e65100">DB 有记录但存储缺失 ({{ reconcileResult.missingInStorage.length }})</h4>
            <NDataTable :columns="reconcileItemColumns" :data="reconcileResult.missingInStorage" :pagination="{ pageSize: 5 }" size="small" />
          </template>

          <template v-if="reconcileResult.missingInDB.length > 0">
            <h4 style="margin: 0; color: #1565c0">存储有文件但 DB 缺失 ({{ reconcileResult.missingInDB.length }})</h4>
            <NDataTable :columns="reconcileItemColumns" :data="reconcileResult.missingInDB" :pagination="{ pageSize: 5 }" size="small" />
          </template>

          <template v-if="reconcileResult.repairedInDB.length > 0">
            <h4 style="margin: 0; color: #2e7d32">已修复 DB 记录 ({{ reconcileResult.repairedInDB.length }})</h4>
            <NDataTable :columns="reconcileItemColumns" :data="reconcileResult.repairedInDB" :pagination="{ pageSize: 5 }" size="small" />
          </template>

          <template v-if="reconcileResult.resetToPending.length > 0">
            <h4 style="margin: 0; color: #f57c00">已重置为待下载 ({{ reconcileResult.resetToPending.length }})</h4>
            <NDataTable :columns="reconcileItemColumns" :data="reconcileResult.resetToPending" :pagination="{ pageSize: 5 }" size="small" />
          </template>

          <template v-if="reconcileResult.orphanReleases.length > 0">
            <h4 style="margin: 0; color: #c62828">孤儿 Release ({{ reconcileResult.orphanReleases.length }})</h4>
            <NDataTable :columns="reconcileItemColumns" :data="reconcileResult.orphanReleases" :pagination="{ pageSize: 5 }" size="small" />
          </template>

          <template v-if="reconcileResult.orphanAssets.length > 0">
            <h4 style="margin: 0; color: #c62828">孤儿 Asset ({{ reconcileResult.orphanAssets.length }})</h4>
            <NDataTable :columns="reconcileItemColumns" :data="reconcileResult.orphanAssets" :pagination="{ pageSize: 5 }" size="small" />
          </template>

          <template v-if="reconcileResult.orphanTasks.length > 0">
            <h4 style="margin: 0; color: #c62828">孤儿 Task ({{ reconcileResult.orphanTasks.length }})</h4>
            <NDataTable :columns="reconcileItemColumns" :data="reconcileResult.orphanTasks" :pagination="{ pageSize: 5 }" size="small" />
          </template>

          <template v-if="reconcileResult.orphanTaskLogs > 0 || reconcileResult.orphanRepoStorages > 0">
            <h4 style="margin: 0; color: #c62828">其他孤儿记录</h4>
            <NSpace>
              <NTag v-if="reconcileResult.orphanTaskLogs > 0" type="error" size="small">TaskLog: {{ reconcileResult.orphanTaskLogs }}</NTag>
              <NTag v-if="reconcileResult.orphanRepoStorages > 0" type="error" size="small">RepositoryStorage: {{ reconcileResult.orphanRepoStorages }}</NTag>
            </NSpace>
          </template>

          <template v-if="reconcileResult.storageScanErrors.length > 0">
            <h4 style="margin: 0; color: #c62828">存储扫描错误</h4>
            <NAlert v-for="(err, i) in reconcileResult.storageScanErrors" :key="i" type="error" :bordered="false" style="margin-bottom: 4px">{{ err }}</NAlert>
          </template>

          <NAlert v-if="reconcileResult.missingInStorage.length === 0 && reconcileResult.missingInDB.length === 0 && reconcileResult.resetToPending.length === 0 && reconcileResult.repairedInDB.length === 0" type="success" :bordered="false">
            所有存储数据一致，无异常 ✅
          </NAlert>
        </template>
      </NSpace>
    </NModal>

    <NModal v-model:show="showModal" preset="dialog" :title="editingId ? '编辑存储' : '添加存储'" :positive-text="editingId ? '保存' : '添加'" negative-text="取消" @positive-click="handleSubmit">
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
  width: 100%;
  min-width: 0;
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
