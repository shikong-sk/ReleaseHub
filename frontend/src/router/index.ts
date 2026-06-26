import { createRouter, createWebHistory } from 'vue-router'

import DashboardView from '@/views/DashboardView.vue'
import FilesView from '@/views/FilesView.vue'
import NotificationsView from '@/views/NotificationsView.vue'
import ProxiesView from '@/views/ProxiesView.vue'
import RepositoriesView from '@/views/RepositoriesView.vue'
import SettingsView from '@/views/SettingsView.vue'
import StoragesView from '@/views/StoragesView.vue'
import TasksView from '@/views/TasksView.vue'

const router = createRouter({
  history: createWebHistory(),
  routes: [
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
      path: '/settings',
      name: 'settings',
      component: SettingsView
    }
  ]
})

export default router
