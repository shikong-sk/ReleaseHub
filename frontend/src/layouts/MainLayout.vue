<script setup lang="ts">
import { RouterView } from 'vue-router'
import { NButton, NLayout, NLayoutContent, NLayoutHeader, NMenu } from 'naive-ui'
import type { MenuOption } from 'naive-ui'
import { computed, h } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import { LogOut } from 'lucide-vue-next'

import { useAuthStore } from '@/stores/auth'

const router = useRouter()
const route = useRoute()
const authStore = useAuthStore()
const isLoginRoute = computed(() => route.name === 'login')
const menuOptions = computed<MenuOption[]>(() => [
  {
    label: () => hRouterLink('/', '控制台'),
    key: 'dashboard'
  },
  {
    label: () => hRouterLink('/repositories', '仓库'),
    key: 'repositories'
  },
  {
    label: () => hRouterLink('/tasks', '任务'),
    key: 'tasks'
  },
  {
    label: () => hRouterLink('/files', '文件'),
    key: 'files'
  },
  {
    label: () => hRouterLink('/storages', '存储'),
    key: 'storages'
  },
  {
    label: () => hRouterLink('/proxies', '代理'),
    key: 'proxies'
  },
  {
    label: () => hRouterLink('/notifications', '通知'),
    key: 'notifications'
  },
  {
    label: () => hRouterLink('/users', '用户'),
    key: 'users'
  },
  {
    label: () => hRouterLink('/settings', '设置'),
    key: 'settings'
  }
])

const selectedMenuKey = computed(() => String(route.name ?? 'dashboard'))

function hRouterLink(to: string, label: string) {
  return h(RouterLink, { to }, { default: () => label })
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
        <NButton quaternary circle title="退出登录" @click="handleLogout">
          <template #icon><LogOut /></template>
        </NButton>
      </div>
    </NLayoutHeader>

    <NLayoutContent class="app-content">
      <RouterView />
    </NLayoutContent>
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
