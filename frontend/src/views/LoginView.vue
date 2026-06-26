<script setup lang="ts">
import { reactive } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { NButton, NCard, NForm, NFormItem, NInput, useMessage } from 'naive-ui'
import { LogIn } from 'lucide-vue-next'

import { useAuthStore } from '@/stores/auth'

const router = useRouter()
const route = useRoute()
const authStore = useAuthStore()
const message = useMessage()

const form = reactive({
  username: '',
  password: ''
})

async function handleLogin() {
  if (!form.username.trim() || !form.password) {
    message.warning('用户名和密码不能为空')
    return
  }

  try {
    await authStore.login({
      username: form.username.trim(),
      password: form.password
    })
    const redirect = typeof route.query.redirect === 'string' ? route.query.redirect : '/'
    await router.replace(redirect)
  } catch (err) {
    message.error(err instanceof Error ? err.message : '登录失败')
  }
}
</script>

<template>
  <main class="login-page">
    <NCard class="login-card" :bordered="false">
      <div class="login-header">
        <span class="login-mark">RH</span>
        <div class="login-title">
          <strong>ReleaseHub</strong>
          <span>登录管理后台</span>
        </div>
      </div>

      <NForm label-placement="top" @submit.prevent="handleLogin">
        <NFormItem label="用户名">
          <NInput v-model:value="form.username" placeholder="admin" />
        </NFormItem>
        <NFormItem label="密码">
          <NInput v-model:value="form.password" type="password" show-password-on="click" placeholder="请输入密码" />
        </NFormItem>
        <NButton type="primary" block :loading="authStore.loading" @click="handleLogin">
          <template #icon><LogIn /></template>
          登录
        </NButton>
      </NForm>
    </NCard>
  </main>
</template>

<style scoped>
.login-page {
  display: grid;
  min-height: 100vh;
  place-items: center;
  padding: 24px;
  background: #f5f7fb;
}

.login-card {
  width: min(420px, 100%);
  border-radius: 8px;
  box-shadow: 0 18px 48px rgb(16 24 40 / 12%);
}

.login-header {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 24px;
}

.login-mark {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 40px;
  height: 40px;
  border-radius: 8px;
  color: #ffffff;
  font-weight: 700;
  background: #1f6feb;
}

.login-title {
  display: flex;
  flex-direction: column;
}

.login-title strong {
  color: #101828;
  font-size: 18px;
}

.login-title span {
  color: #667085;
  font-size: 13px;
}
</style>
