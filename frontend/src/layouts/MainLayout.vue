<script setup lang="ts">
import { RouterView } from 'vue-router'
import { NLayout, NLayoutContent, NLayoutHeader, NMenu } from 'naive-ui'
import type { MenuOption } from 'naive-ui'
import { computed, h } from 'vue'
import { RouterLink, useRoute } from 'vue-router'

const route = useRoute()
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
    label: '设置',
    key: 'settings',
    disabled: true
  }
])

const selectedMenuKey = computed(() => String(route.name ?? 'dashboard'))

function hRouterLink(to: string, label: string) {
  return h(RouterLink, { to }, { default: () => label })
}
</script>

<template>
  <NLayout class="app-shell">
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
  gap: 2px;
  line-height: 1.2;
}

.brand-copy span {
  color: #667085;
  font-size: 12px;
}

.main-menu {
  flex: 1;
}

.app-content {
  padding: 24px;
}

@media (max-width: 760px) {
  .app-header {
    align-items: flex-start;
    flex-direction: column;
    height: auto;
    padding: 16px;
  }

  .brand {
    min-width: 0;
  }

  .app-content {
    padding: 16px;
  }
}
</style>
