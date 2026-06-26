import { defineStore } from 'pinia'
import { computed, shallowRef } from 'vue'

import { getMe, login as apiLogin, changePassword as apiChangePassword, type LoginPayload, type UserInfo } from '@/api/auth'

const TOKEN_KEY = 'releasehub_token'

export const useAuthStore = defineStore('auth', () => {
  const user = shallowRef<UserInfo | null>(null)
  const token = shallowRef<string | null>(localStorage.getItem(TOKEN_KEY))
  const loading = shallowRef(false)
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

  async function changePassword(oldPassword: string, newPassword: string) {
    await apiChangePassword(oldPassword, newPassword)
  }

  function logout() {
    token.value = null
    user.value = null
    localStorage.removeItem(TOKEN_KEY)
  }

  // 角色判断
  const isAdmin = computed(() => user.value?.role === 'admin')
  const isOperator = computed(() => user.value?.role === 'operator')
  const isViewer = computed(() => user.value?.role === 'viewer')
  const canWrite = computed(() => isAdmin.value || isOperator.value)
  const canAdmin = computed(() => isAdmin.value)

  return { user, token, loading, error, isLoggedIn, isAdmin, isOperator, isViewer, canWrite, canAdmin, login, fetchMe, changePassword, logout }
})
