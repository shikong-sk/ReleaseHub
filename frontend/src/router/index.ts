import { createRouter, createWebHistory } from 'vue-router'

import DashboardView from '@/views/DashboardView.vue'
import FilesView from '@/views/FilesView.vue'
import RepositoriesView from '@/views/RepositoriesView.vue'
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
    }
  ]
})

export default router
