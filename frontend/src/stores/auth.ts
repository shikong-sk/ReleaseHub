import { defineStore } from 'pinia'
import { computed, shallowRef } from 'vue'

import { getMe, login as apiLogin, changePassword as apiChangePassword, type LoginPayload, type UserInfo } from '@/api/auth'

const TOKEN_KEY = 'releasehub_token'

export const useAuthStore = defineStore('auth', () => {
  const user = shallowRef<UserInfo | null>(null)
  const token = shallowRef<string | null>(localStorage.getItem(TOKEN_KEY))
  const loading = shallowRef(false)
  const bootstrapping = shallowRef(false)
  const error = shallowRef<string | null>(null)

  const isLoggedIn = computed(() => !!token.value)

  async function login(payload: LoginPayload) {
    loading.value = true
    error.value = null
    try {
      const result = await apiLogin(payload)
      token.value = result.token
      user.value = result.user
      localStorage.setItem(TOKEN_KEY, result.token)
      return result
    } catch (err) {
      error.value = err instanceof Error ? err.message : '登录失败'
      throw err
    } finally {
      loading.value = false
    }
  }

  async function fetchMe() {
    if (!token.value) return
    try {
      user.value = await getMe()
    } catch {
      logout()
    }
  }

  async function ensureUser() {
    if (!token.value || user.value || bootstrapping.value) return
    bootstrapping.value = true
    try {
      await fetchMe()
    } finally {
      bootstrapping.value = false
    }
  }

  async function changePassword(oldPassword: string, newPassword: string) {
    await apiChangePassword(oldPassword, newPassword)
  }

  function logout() {
    token.value = null
    user.value = null
    localStorage.removeItem(TOKEN_KEY)
  }

  // 认证开关：关闭时后端将所有请求视为 admin，前端需同步此状态
  const authEnabled = shallowRef(true)

  function setAuthEnabled(value: boolean) {
    authEnabled.value = value
  }

  // 认证关闭时所有操作视为管理员权限，与后端 APIKeyOrAuth 的 role=admin 语义一致
  const isAuthDisabled = computed(() => !authEnabled.value)
  const isAdmin = computed(() => isAuthDisabled.value || user.value?.role === 'admin')
  const isOperator = computed(() => !isAuthDisabled.value && user.value?.role === 'operator')
  const isViewer = computed(() => !isAuthDisabled.value && user.value?.role === 'viewer')
  const canWrite = computed(() => isAdmin.value || isOperator.value)
  const canAdmin = computed(() => isAdmin.value)

  return { user, token, loading, bootstrapping, error, isLoggedIn, isAdmin, isOperator, isViewer, canWrite, canAdmin, authEnabled, setAuthEnabled, login, fetchMe, ensureUser, changePassword, logout }
})
