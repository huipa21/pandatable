import { ref } from 'vue'
import { defineStore } from 'pinia'
import { request, RequestError } from '../api/client'
import type { AuthUser, AuthUserMutation, LoginRequest } from '../api/types'

export const useAuthStore = defineStore('auth', () => {
  const user = ref<AuthUser | null>(null)
  const passwordChanged = ref(false)
  let generation = 0

  function setUser(value: AuthUser | null) {
    generation++
    user.value = value
  }

  async function refresh() {
    const current = ++generation
    try {
      const result = await request<AuthUser>('/auth/me')
      if (generation === current) user.value = result
    } catch (error) {
      if (
        !(error instanceof RequestError) ||
        error.code !== 'ERR_UNAUTHENTICATED'
      ) {
        throw error
      }
      if (generation === current) user.value = null
    }
  }

  async function login(input: LoginRequest) {
    const result = await request<AuthUserMutation>('/auth/login', 'POST', input)
    passwordChanged.value = false
    setUser(result.user)
  }

  async function logout() {
    await request('/auth/logout', 'POST')
    setUser(null)
  }

  return { user, passwordChanged, setUser, refresh, login, logout }
})
