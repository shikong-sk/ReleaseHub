import { createRouter, createWebHistory } from 'vue-router'

import { getAppConfig } from '@/api/settings'
import { useAuthStore } from '@/stores/auth'
import DashboardView from '@/views/DashboardView.vue'
import FilesView from '@/views/FilesView.vue'
import LoginView from '@/views/LoginView.vue'
import NotificationsView from '@/views/NotificationsView.vue'
import ProxiesView from '@/views/ProxiesView.vue'
import RepositoriesView from '@/views/RepositoriesView.vue'
import SettingsView from '@/views/SettingsView.vue'
import StoragesView from '@/views/StoragesView.vue'
import TasksView from '@/views/TasksView.vue'
import UsersView from '@/views/UsersView.vue'
import LogsView from '@/views/LogsView.vue'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: '/login',
      name: 'login',
      component: LoginView,
      meta: { public: true }
    },
    {
      path: '/',
      name: 'dashboard',
      component: DashboardView
    },
    {
      path: '/repositories',
      name: 'repositories',
      component: RepositoriesView
    },
    {
      path: '/tasks',
      name: 'tasks',
      component: TasksView
    },
    {
      path: '/files',
      name: 'files',
      component: FilesView
    },
    {
      path: '/storages',
      name: 'storages',
      component: StoragesView
    },
    {
      path: '/proxies',
      name: 'proxies',
      component: ProxiesView
    },
    {
      path: '/notifications',
      name: 'notifications',
      component: NotificationsView
    },
    {
      path: '/users',
      name: 'users',
      component: UsersView
    },
    {
      path: '/settings',
      name: 'settings',
      component: SettingsView
    },
    {
      path: '/logs',
      name: 'logs',
      component: LogsView
    }
  ]
})

let authEnabledCache: boolean | null = null

router.beforeEach(async (to) => {
  const authStore = useAuthStore()
  if (authEnabledCache === null) {
    try {
      const config = await getAppConfig()
      authEnabledCache = config.authEnabled
      authStore.setAuthEnabled(config.authEnabled)
    } catch {
      authEnabledCache = false
      authStore.setAuthEnabled(false)
    }
  }

  if (!authEnabledCache) {
    return true
  }

  if (to.meta.public) {
    return authStore.isLoggedIn && to.name === 'login' ? { name: 'dashboard' } : true
  }

  if (!authStore.isLoggedIn) {
    return { name: 'login', query: { redirect: to.fullPath } }
  }

  if (!authStore.user) {
    await authStore.fetchMe()
  }

  if (!authStore.isLoggedIn) {
    return { name: 'login', query: { redirect: to.fullPath } }
  }

  return true
})

export default router
