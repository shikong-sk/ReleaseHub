<script setup lang="ts">
import { RouterView } from 'vue-router'
import { NButton, NInput, NLayout, NLayoutContent, NLayoutHeader, NMenu, NModal, NForm, NFormItem, useMessage } from 'naive-ui'
import type { MenuOption } from 'naive-ui'
import { computed, h, shallowRef } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import { KeyRound, LogOut } from 'lucide-vue-next'

import { useAuthStore } from '@/stores/auth'

const router = useRouter()
const route = useRoute()
const authStore = useAuthStore()
const isLoginRoute = computed(() => route.name === 'login')
const menuOptions = computed<MenuOption[]>(() => {
  const items: MenuOption[] = [
    { label: () => hRouterLink('/', '控制台'), key: 'dashboard' },
    { label: () => hRouterLink('/repositories', '仓库'), key: 'repositories' },
    { label: () => hRouterLink('/tasks', '任务'), key: 'tasks' },
    { label: () => hRouterLink('/files', '文件'), key: 'files' }
  ]
  // 管理菜单仅 admin 可见
  if (authStore.canAdmin) {
    items.push(
      { label: () => hRouterLink('/storages', '存储'), key: 'storages' },
      { label: () => hRouterLink('/proxies', '代理'), key: 'proxies' },
      { label: () => hRouterLink('/notifications', '通知'), key: 'notifications' },
      { label: () => hRouterLink('/users', '用户'), key: 'users' },
      { label: () => hRouterLink('/settings', '设置'), key: 'settings' }
    )
  }
  return items
})

const selectedMenuKey = computed(() => String(route.name ?? 'dashboard'))

function hRouterLink(to: string, label: string) {
  return h(RouterLink, { to }, { default: () => label })
}

const message = useMessage()
const showPasswordModal = shallowRef(false)
const oldPassword = shallowRef('')
const newPassword = shallowRef('')
const confirmPassword = shallowRef('')
const passwordLoading = shallowRef(false)

async function handleChangePassword() {
  if (!oldPassword.value || !newPassword.value) {
    message.warning('请填写完整')
    return
  }
  if (newPassword.value.length < 6) {
    message.warning('新密码至少 6 位')
    return
  }
  if (newPassword.value !== confirmPassword.value) {
    message.warning('两次输入的新密码不一致')
    return
  }
  passwordLoading.value = true
  try {
    await authStore.changePassword(oldPassword.value, newPassword.value)
    message.success('密码已修改，请重新登录')
    showPasswordModal.value = false
    oldPassword.value = ''
    newPassword.value = ''
    confirmPassword.value = ''
    authStore.logout()
    await router.replace('/login')
  } catch (err) {
    message.error(err instanceof Error ? err.message : '修改密码失败')
  } finally {
    passwordLoading.value = false
  }
}

async function handleLogout() {
  authStore.logout()
  await router.replace('/login')
}
</script>

<template>
  <RouterView v-if="isLoginRoute" />
  <NLayout v-else class="app-shell">
    <NLayoutHeader class="app-header" bordered>
      <div class="brand">
        <span class="brand-mark">RH</span>
        <div class="brand-copy">
          <strong>ReleaseHub</strong>
          <span>GitHub Release Artifact Management</span>
        </div>
      </div>
      <NMenu
        class="main-menu"
        mode="horizontal"
        :options="menuOptions"
        :value="selectedMenuKey"
      />
      <div v-if="authStore.isLoggedIn" class="user-actions">
        <span class="username">{{ authStore.user?.username ?? '已登录' }}</span>
        <NButton quaternary circle title="修改密码" @click="showPasswordModal = true">
          <template #icon><KeyRound /></template>
        </NButton>
        <NButton quaternary circle title="退出登录" @click="handleLogout">
          <template #icon><LogOut /></template>
        </NButton>
      </div>
    </NLayoutHeader>

    <NLayoutContent class="app-content">
      <RouterView />
    </NLayoutContent>
    <NModal v-model:show="showPasswordModal" preset="dialog" title="修改密码" positive-text="确认修改" negative-text="取消" :loading="passwordLoading" @positive-click="handleChangePassword">
      <NForm label-placement="left" label-width="auto">
        <NFormItem label="当前密码">
          <NInput v-model:value="oldPassword" type="password" show-password-on="click" placeholder="输入当前密码" />
        </NFormItem>
        <NFormItem label="新密码">
          <NInput v-model:value="newPassword" type="password" show-password-on="click" placeholder="至少 6 位" />
        </NFormItem>
        <NFormItem label="确认密码">
          <NInput v-model:value="confirmPassword" type="password" show-password-on="click" placeholder="再次输入新密码" />
        </NFormItem>
      </NForm>
    </NModal>
  </NLayout>
</template>

<style scoped>
.app-shell {
  min-height: 100vh;
  background: #f5f7fb;
}

.app-header {
  display: flex;
  align-items: center;
  gap: 28px;
  height: 64px;
  padding: 0 24px;
  background: #ffffff;
}

.brand {
  display: flex;
  align-items: center;
  gap: 12px;
  min-width: 300px;
}

.brand-mark {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 36px;
  height: 36px;
  border-radius: 8px;
  color: #ffffff;
  font-weight: 700;
  background: #1f6feb;
}

.brand-copy {
  display: flex;
  flex-direction: column;
}

.brand-copy strong {
  font-size: 16px;
  color: #101828;
}

.brand-copy span {
  font-size: 12px;
  color: #667085;
}

.app-content {
  padding: 24px;
}

.main-menu {
  flex: 1;
}

.user-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}

.username {
  max-width: 140px;
  overflow: hidden;
  color: #475467;
  font-size: 13px;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
