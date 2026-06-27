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
  NTag,
  useMessage
} from 'naive-ui'
import { Plus, RefreshCw } from 'lucide-vue-next'
import type { DataTableColumns } from 'naive-ui'
import { computed } from 'vue'

import { listUsers, createUser, updateUser, deleteUser, type UserInfo } from '@/api/auth'

const message = useMessage()
const users = shallowRef<UserInfo[]>([])
const loading = shallowRef(false)
const saving = shallowRef(false)

const showModal = shallowRef(false)
const formUsername = shallowRef('')
const formPassword = shallowRef('')
const formRole = shallowRef('viewer')

const roleOptions = [
  { label: '管理员', value: 'admin' },
  { label: '操作员', value: 'operator' },
  { label: '只读', value: 'viewer' }
]

const roleLabel: Record<string, string> = { admin: '管理员', operator: '操作员', viewer: '只读' }

const userColumns = computed<DataTableColumns<UserInfo>>(() => [
  { title: '用户名', key: 'username', width: 180 },
  {
    title: '角色',
    key: 'role',
    width: 100,
    render: (row) =>
      h(NTag, {
        type: row.role === 'admin' ? 'error' : row.role === 'operator' ? 'warning' : 'default',
        size: 'small'
      }, { default: () => roleLabel[row.role] || row.role })
  },
  {
    title: '状态',
    key: 'enabled',
    width: 80,
    render: (row) =>
      row.enabled
        ? h(NTag, { type: 'success', size: 'small' }, { default: () => '启用' })
        : h(NTag, { type: 'default', size: 'small' }, { default: () => '停用' })
  },
  { title: '最后登录', key: 'lastLoginAt', width: 180, render: (row) => row.lastLoginAt || '-' },
  {
    title: '操作',
    key: 'actions',
    width: 200,
    render: (row) =>
      h(NSpace, null, {
        default: () => [
          h(
            NButton,
            { size: 'small', secondary: true, onClick: () => handleToggleEnabled(row) },
            { default: () => row.enabled ? '停用' : '启用' }
          ),
          h(
            NPopconfirm,
            { onPositiveClick: () => handleDelete(row.id) },
            {
              trigger: () => h(NButton, { size: 'small', type: 'error', secondary: true, loading: saving.value }, { default: () => '删除' }),
              default: () => `删除用户 "${row.username}"？`
            }
          )
        ]
      })
  }
])

onMounted(() => { void refresh() })

async function refresh() {
  loading.value = true
  try {
    const result = await listUsers()
    users.value = result.items
  } catch (err) {
    message.error(err instanceof Error ? err.message : '加载用户列表失败')
  } finally {
    loading.value = false
  }
}

function openCreateModal() {
  formUsername.value = ''
  formPassword.value = ''
  formRole.value = 'viewer'
  showModal.value = true
}

async function handleCreate() {
  if (!formUsername.value.trim() || !formPassword.value) {
    message.warning('用户名和密码不能为空')
    return
  }
  saving.value = true
  try {
    await createUser({
      username: formUsername.value.trim(),
      password: formPassword.value,
      role: formRole.value
    })
    message.success('用户已创建')
    showModal.value = false
    await refresh()
  } catch (err) {
    message.error(err instanceof Error ? err.message : '创建用户失败')
  } finally {
    saving.value = false
  }
}

async function handleToggleEnabled(user: UserInfo) {
  saving.value = true
  try {
    await updateUser(user.id, { enabled: !user.enabled })
    message.success(user.enabled ? '用户已停用' : '用户已启用')
    await refresh()
  } catch (err) {
    message.error(err instanceof Error ? err.message : '操作失败')
  } finally {
    saving.value = false
  }
}

async function handleDelete(id: number) {
  saving.value = true
  try {
    await deleteUser(id)
    message.success('用户已删除')
    await refresh()
  } catch (err) {
    message.error(err instanceof Error ? err.message : '删除用户失败')
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <main class="users-page">
    <section class="users-heading">
      <h1>用户管理</h1>
      <p>管理系统用户、角色和权限。</p>
    </section>

    <NCard title="用户列表" :bordered="false">
      <template #header-extra>
        <NSpace>
          <NButton secondary :loading="loading" @click="refresh">
            <template #icon><RefreshCw /></template>
            刷新
          </NButton>
          <NButton type="primary" @click="openCreateModal">
            <template #icon><Plus /></template>
            添加用户
          </NButton>
        </NSpace>
      </template>

      <NDataTable
        :columns="userColumns"
        :data="users"
        :loading="loading"
        :row-key="(row: UserInfo) => row.id"
        :pagination="{ pageSize: 10 }"
      />
    </NCard>

    <NModal v-model:show="showModal" preset="dialog" title="添加用户" positive-text="创建" negative-text="取消" @positive-click="handleCreate">
      <NForm label-placement="left" label-width="auto">
        <NFormItem label="用户名">
          <NInput v-model:value="formUsername" placeholder="admin" />
        </NFormItem>
        <NFormItem label="密码">
          <NInput v-model:value="formPassword" type="password" show-password-on="click" placeholder="至少 6 位" />
        </NFormItem>
        <NFormItem label="角色">
          <NSelect v-model:value="formRole" :options="roleOptions" />
        </NFormItem>
      </NForm>
    </NModal>
  </main>
</template>

<style scoped>
.users-page {
  display: flex;
  flex-direction: column;
  gap: 20px;
  width: 100%;
  min-width: 0;
}

.users-heading {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.users-heading h1 {
  margin: 0;
  color: #101828;
  font-size: 28px;
  font-weight: 700;
}

.users-heading p {
  margin: 0;
  color: #667085;
}
</style>
